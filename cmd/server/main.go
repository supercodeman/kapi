package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/analytics"
	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/engine"
	"github.com/sangchenglong/kapi/internal/handler"
	"github.com/sangchenglong/kapi/internal/memory"
	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/router"
	"github.com/sangchenglong/kapi/internal/service"
	"github.com/sangchenglong/kapi/internal/skill"
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.Server.GinMode)

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect mysql: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("failed to auto migrate: %v", err)
	}
	if err := model.SeedCategories(db); err != nil {
		log.Printf("warning: failed to seed categories: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}

	// DAO
	userDAO := dao.NewUserDAO(db)
	billDAO := dao.NewBillDAO(db)
	budgetDAO := dao.NewBudgetDAO(db)
	assetDAO := dao.NewAssetDAO(db)
	memoryDAO := dao.NewMemoryDAO(db)
	skillDAO := dao.NewSkillDAO(db)
	patternDAO := dao.NewPatternDAO(db)

	// Service
	authSvc := service.NewAuthService(userDAO, cfg.JWT)
	billSvc := service.NewBillService(billDAO)
	budgetSvc := service.NewBudgetService(budgetDAO)
	assetSvc := service.NewAssetService(assetDAO)

	// Memory Manager
	convStore := memory.NewConversationStore(rdb, 50, 7*24*time.Hour)

	// Milvus + Embedding
	var milvusClient *memory.MilvusClient
	var embeddingProvider memory.EmbeddingProvider

	mc, err := memory.NewMilvusClient(cfg.Milvus.Addr)
	if err != nil {
		log.Printf("warning: failed to connect milvus: %v", err)
	} else {
		if err := mc.EnsureCollection(context.Background()); err != nil {
			log.Printf("warning: failed to ensure milvus collection: %v", err)
		} else {
			milvusClient = mc
			log.Printf("Milvus connected: %s", cfg.Milvus.Addr)
		}
	}

	if cfg.Embedding.BaseURL != "" && cfg.Embedding.APIKey != "" && cfg.Embedding.Model != "" {
		embeddingProvider = memory.NewOpenAIEmbeddingProvider(cfg.Embedding.BaseURL, cfg.Embedding.APIKey, cfg.Embedding.Model)
		log.Printf("Embedding provider initialized: %s (%s)", cfg.Embedding.Model, cfg.Embedding.BaseURL)
	}

	memMgr := memory.NewMemoryManager(memoryDAO, milvusClient, embeddingProvider, convStore)

	// 补偿历史记忆的 Embedding
	go memMgr.BackfillEmbeddings(context.Background())

	// Skill Registry
	skillReg := skill.NewRegistry("skills", skillDAO)
	if err := skillReg.Load(context.Background()); err != nil {
		log.Printf("warning: failed to load skills: %v", err)
	}
	skillWatcher := skill.NewWatcher(skillReg, "skills", 10*time.Second)
	skillWatcher.Start(context.Background())

	// Operation Logger
	opLogDAO := dao.NewOperationLogDAO(db)
	opLogSvc := service.NewOpLogService(opLogDAO)

	// AI Engine
	toolExec := engine.NewToolExecutor(billSvc, budgetSvc, assetSvc, opLogSvc)
	toolExec.SetRAGFiller(engine.NewRAGFiller(patternDAO))

	// Analytics Engine（带 Redis 缓存层）
	analyticsBase := analytics.NewEngine(db)
	analyticsEng := analytics.NewCachedEngine(analyticsBase, rdb)

	// Pattern Extractor（L5/L6 消费模式提取）
	patternExtractor := analytics.NewPatternExtractor(billDAO, patternDAO)

	// 注册所有扩展 Tool（Analytics、预算、模式管理等）
	RegisterTools(toolExec, ToolDeps{
		AnalyticsEng: analyticsEng,
		OpLogSvc:     opLogSvc,
		PatternDAO:   patternDAO,
		BillSvc:      billSvc,
		AssetSvc:     assetSvc,
	})

	// LLM Provider
	var llmProvider engine.LLMProvider
	if cfg.LLM.BaseURL != "" && cfg.LLM.APIKey != "" && cfg.LLM.Model != "" {
		llmProvider = engine.NewOpenAICompatProvider(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)
		log.Printf("LLM provider initialized: %s (%s)", cfg.LLM.Model, cfg.LLM.BaseURL)
	} else {
		log.Println("warning: LLM provider not configured, chat will return fallback response")
	}

	// 写操作完成后异步清除分析缓存 + 增量提取消费模式
	toolExec.SetOnWriteComplete(func(ctx context.Context, userID uint64) {
		go analyticsEng.InvalidateUser(ctx, userID)
		go func() {
			bgCtx := context.Background()
			if err := patternExtractor.ExtractConsumptionPatterns(bgCtx, userID); err != nil {
				log.Printf("pattern extraction failed for user %d: %v", userID, err)
			}
			if err := patternExtractor.ExtractSequencePatterns(bgCtx, userID); err != nil {
				log.Printf("sequence extraction failed for user %d: %v", userID, err)
			}
		}()
	})

	aiEngine := engine.NewEngine(llmProvider, skillReg, memMgr, toolExec, engine.DefaultSystemPrompt, cfg.LLM.MaxConcurrent)
	connMgr := engine.NewConnectionManager(aiEngine)

	// Handler
	authH := handler.NewAuthHandler(authSvc)
	billH := handler.NewBillHandler(billSvc)
	budgetH := handler.NewBudgetHandler(budgetSvc)
	assetH := handler.NewAssetHandler(assetSvc)
	chatH := handler.NewChatHandler(aiEngine)

	// Router
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	skillH := handler.NewSkillHandler(skillDAO)

	router.Setup(r, authSvc, authH, billH, budgetH, assetH, chatH, skillH, connMgr)

	// Suggestion API（不修改 router.Setup 签名，单独注册）
	suggestionSvc := service.NewSuggestionService(billDAO, memoryDAO, patternDAO, analyticsEng)
	suggestionH := handler.NewSuggestionHandler(suggestionSvc)
	r.GET("/api/suggestions", middleware.Auth(authSvc), func(c *gin.Context) {
		suggestionH.GetSuggestions(c)
	})

	// Start
	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: r,
	}

	go func() {
		log.Printf("kapi server starting on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	skillWatcher.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	sqlDB.Close()
	rdb.Close()
	log.Println("server exited")
}
