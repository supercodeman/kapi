# P1: 基础设施 + Auth + Biz Mock 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建项目基础设施，实现 JWT 认证和 Mock 业务 CRUD API，为后续子系统提供可运行的服务基座。

**Architecture:** 单体 Go 服务（Gin 框架），分层架构（Router → Handler → Service → DAO → Model）。Docker Compose 编排 MySQL + Redis + Milvus。Auth 中间件统一拦截 HTTP 请求，JWT Token 携带 user_id 实现多租户隔离。

**Tech Stack:** Go 1.23+, Gin, GORM, JWT (golang-jwt), MySQL 8.0, Redis 7, Docker Compose

---

## 文件结构规划

```
kapi/
├── cmd/
│   └── server/
│       └── main.go                    # 服务入口
├── internal/
│   ├── config/
│   │   └── config.go                  # 配置加载（环境变量）
│   ├── model/
│   │   ├── base.go                    # GORM 基础模型 + AutoMigrate
│   │   ├── user.go                    # User 模型
│   │   ├── bill.go                    # Bill 模型
│   │   ├── budget.go                  # Budget 模型
│   │   └── asset.go                   # Asset 模型
│   ├── dao/
│   │   ├── user.go                    # User DAO
│   │   ├── bill.go                    # Bill DAO
│   │   ├── budget.go                  # Budget DAO
│   │   └── asset.go                   # Asset DAO
│   ├── service/
│   │   ├── auth.go                    # Auth Service（注册/登录/JWT）
│   │   ├── bill.go                    # Bill Service
│   │   ├── budget.go                  # Budget Service
│   │   └── asset.go                   # Asset Service
│   ├── handler/
│   │   ├── auth.go                    # Auth Handler（注册/登录）
│   │   ├── bill.go                    # Bill Handler
│   │   ├── budget.go                  # Budget Handler
│   │   └── asset.go                   # Asset Handler
│   ├── middleware/
│   │   └── auth.go                    # JWT Auth 中间件
│   ├── router/
│   │   └── router.go                  # 路由注册
│   └── pkg/
│       ├── response/
│       │   └── response.go            # 统一响应格式
│       └── errcode/
│           └── errcode.go             # 错误码定义
├── docker-compose.yml                 # Docker Compose 编排
├── Dockerfile                         # Go 服务 Dockerfile
├── scripts/
│   └── init.sql                       # 数据库初始化 SQL
├── go.mod
├── go.sum
└── .claude/
    └── CLAUDE.md                      # 项目级 Agent 指令
```

---

## Task 1: 项目初始化 + Docker Compose

### 目标
初始化 Go 模块，创建完整目录结构，编写 Docker Compose 编排文件和多阶段构建 Dockerfile。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `go.mod` |
| Create | `docker-compose.yml` |
| Create | `Dockerfile` |
| Create | `.env.example` |
| Create | `cmd/server/main.go`（占位） |

### Steps

- [ ] **1.1** 初始化 Go 模块

```bash
cd /Users/sangchenglong/go/src/kapi
go mod init github.com/sangchenglong/kapi
```

- [ ] **1.2** 创建目录结构

```bash
cd /Users/sangchenglong/go/src/kapi
mkdir -p cmd/server
mkdir -p internal/{config,model,dao,service,handler,middleware,router}
mkdir -p internal/pkg/{response,errcode}
mkdir -p scripts
mkdir -p .claude
```

- [ ] **1.3** 创建 `docker-compose.yml`

```yaml
# docker-compose.yml
version: "3.8"

services:
  go-server:
    build: .
    ports:
      - "8080:8080"
    env_file:
      - .env
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
      milvus:
        condition: service_healthy
    restart: unless-stopped
    networks:
      - kapi-net

  mysql:
    image: mysql:8.0
    ports:
      - "3306:3306"
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:-kapi_root_123}
      MYSQL_DATABASE: kapi
      MYSQL_CHARSET: utf8mb4
    volumes:
      - mysql_data:/var/lib/mysql
      - ./scripts/init.sql:/docker-entrypoint-initdb.d/init.sql
    command: --character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
    restart: unless-stopped
    networks:
      - kapi-net

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped
    networks:
      - kapi-net

  milvus:
    image: milvusdb/milvus:v2.3-latest
    ports:
      - "19530:19530"
      - "9091:9091"
    volumes:
      - milvus_data:/var/lib/milvus
    environment:
      ETCD_USE_EMBED: "true"
      COMMON_STORAGETYPE: local
    command: ["milvus", "run", "standalone"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9091/healthz"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 60s
    restart: unless-stopped
    networks:
      - kapi-net

volumes:
  mysql_data:
  redis_data:
  milvus_data:

networks:
  kapi-net:
    driver: bridge
```

- [ ] **1.4** 创建 `Dockerfile`（多阶段构建）

```dockerfile
# Dockerfile
# Stage 1: Build
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./cmd/server

# Stage 2: Run
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080

ENTRYPOINT ["./server"]
```

- [ ] **1.5** 创建 `.env.example`

```env
# .env.example
# MySQL
MYSQL_ROOT_PASSWORD=kapi_root_123
MYSQL_DSN=root:kapi_root_123@tcp(mysql:3306)/kapi?charset=utf8mb4&parseTime=True&loc=Local

# Redis
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# Milvus
MILVUS_ADDR=milvus:19530

# JWT
JWT_SECRET=kapi-dev-secret-change-in-production
JWT_EXPIRE_HOURS=168

# Server
SERVER_PORT=8080
GIN_MODE=debug

# LLM (P2 阶段使用)
LLM_PROVIDER=openai
LLM_API_KEY=sk-xxx
LLM_MODEL=gpt-4o
EMBEDDING_PROVIDER=openai
EMBEDDING_API_KEY=sk-xxx
EMBEDDING_MODEL=text-embedding-3-small
```

- [ ] **1.6** 创建占位 `cmd/server/main.go`

```go
// cmd/server/main.go
package main

import "fmt"

func main() {
	fmt.Println("kapi server starting...")
}
```

- [ ] **1.7** 验证

```bash
cd /Users/sangchenglong/go/src/kapi
# 验证 go mod
go mod tidy
# 验证构建
go build ./cmd/server
# 验证 docker-compose 配置语法
docker-compose config
```

预期输出：构建成功，docker-compose config 输出完整配置无报错。

- [ ] **1.8** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git init
git add .
git commit -m "feat: init project structure with Docker Compose (MySQL + Redis + Milvus)"
```

---

## Task 2: 配置加载 + 数据库连接

### 目标
实现从环境变量加载配置，建立 MySQL（GORM）和 Redis 连接，完成 main.go 启动流程。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/config/config.go` |
| Create | `internal/config/config_test.go` |
| Modify | `cmd/server/main.go` |
| Modify | `go.mod`（添加依赖） |

### Steps

- [ ] **2.1** 创建 `internal/config/config_test.go`（TDD：先写测试）

```go
// internal/config/config_test.go
package config

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// 清除可能存在的环境变量
	os.Clearenv()

	cfg := Load()

	if cfg.Server.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Server.Port)
	}
	if cfg.JWT.ExpireHours != 168 {
		t.Errorf("expected default expire hours 168, got %d", cfg.JWT.ExpireHours)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("MYSQL_DSN", "root:pass@tcp(localhost:3306)/testdb")
	os.Setenv("REDIS_ADDR", "localhost:6380")
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("JWT_EXPIRE_HOURS", "24")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("MYSQL_DSN")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_EXPIRE_HOURS")
	}()

	cfg := Load()

	if cfg.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Server.Port)
	}
	if cfg.MySQL.DSN != "root:pass@tcp(localhost:3306)/testdb" {
		t.Errorf("unexpected MySQL DSN: %s", cfg.MySQL.DSN)
	}
	if cfg.Redis.Addr != "localhost:6380" {
		t.Errorf("unexpected Redis addr: %s", cfg.Redis.Addr)
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Errorf("unexpected JWT secret: %s", cfg.JWT.Secret)
	}
	if cfg.JWT.ExpireHours != 24 {
		t.Errorf("expected expire hours 24, got %d", cfg.JWT.ExpireHours)
	}
}
```

- [ ] **2.2** 创建 `internal/config/config.go`

```go
// internal/config/config.go
package config

import (
	"os"
	"strconv"
)

// Config 应用配置
type Config struct {
	Server ServerConfig
	MySQL  MySQLConfig
	Redis  RedisConfig
	Milvus MilvusConfig
	JWT    JWTConfig
}

// ServerConfig 服务配置
type ServerConfig struct {
	Port    string
	GinMode string
}

// MySQLConfig MySQL 配置
type MySQLConfig struct {
	DSN string
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// MilvusConfig Milvus 配置
type MilvusConfig struct {
	Addr string
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret      string
	ExpireHours int
}

// Load 从环境变量加载配置，未设置则使用默认值
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    getEnv("SERVER_PORT", "8080"),
			GinMode: getEnv("GIN_MODE", "debug"),
		},
		MySQL: MySQLConfig{
			DSN: getEnv("MYSQL_DSN", "root:kapi_root_123@tcp(localhost:3306)/kapi?charset=utf8mb4&parseTime=True&loc=Local"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Milvus: MilvusConfig{
			Addr: getEnv("MILVUS_ADDR", "localhost:19530"),
		},
		JWT: JWTConfig{
			Secret:      getEnv("JWT_SECRET", "kapi-dev-secret-change-in-production"),
			ExpireHours: getEnvInt("JWT_EXPIRE_HOURS", 168),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
```

- [ ] **2.3** 更新 `cmd/server/main.go`

```go
// cmd/server/main.go
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

	"github.com/sangchenglong/kapi/internal/config"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.GinMode)

	// 连接 MySQL
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect mysql: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 连接 Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}

	// 初始化路由
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 启动 HTTP 服务
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

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	sqlDB.Close()
	rdb.Close()
	log.Println("server exited")

	// 抑制未使用变量警告（后续 Task 会使用 db 和 rdb）
	_ = db
}
```

- [ ] **2.4** 安装依赖

```bash
cd /Users/sangchenglong/go/src/kapi
go get github.com/gin-gonic/gin@latest
go get gorm.io/gorm@latest
go get gorm.io/driver/mysql@latest
go get github.com/redis/go-redis/v9@latest
go mod tidy
```

- [ ] **2.5** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/config/ -v
```

预期输出：
```
=== RUN   TestLoadConfig_Defaults
--- PASS: TestLoadConfig_Defaults
=== RUN   TestLoadConfig_FromEnv
--- PASS: TestLoadConfig_FromEnv
PASS
```

- [ ] **2.6** 验证构建

```bash
cd /Users/sangchenglong/go/src/kapi
go build ./cmd/server
```

- [ ] **2.7** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add config loading and database connections (MySQL + Redis)"
```

---

## Task 3: 统一响应格式 + 错误码

### 目标
定义统一的 HTTP 响应格式和业务错误码，所有 Handler 使用统一的响应函数。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/pkg/response/response.go` |
| Create | `internal/pkg/response/response_test.go` |
| Create | `internal/pkg/errcode/errcode.go` |
| Create | `internal/pkg/errcode/errcode_test.go` |

### Steps

- [ ] **3.1** 创建 `internal/pkg/errcode/errcode.go`

```go
// internal/pkg/errcode/errcode.go
package errcode

// 通用错误码（10000-19999）
const (
	Success         = 0
	ErrInvalidParam = 10001
	ErrInternal     = 10002
)

// 认证错误码（40000-40999）
const (
	ErrUnauthorized   = 40001
	ErrTokenExpired   = 40002
	ErrTokenInvalid   = 40003
	ErrUserExists     = 40004
	ErrUserNotFound   = 40005
	ErrPasswordWrong  = 40006
	ErrTokenMissing   = 40007
)

// 业务错误码（50000-59999）
const (
	ErrBillNotFound   = 50001
	ErrBudgetNotFound = 50002
	ErrAssetNotFound  = 50003
	ErrAccessDenied   = 50004
)

// 错误码对应的消息
var messages = map[int]string{
	Success:           "success",
	ErrInvalidParam:   "invalid parameters",
	ErrInternal:       "internal server error",
	ErrUnauthorized:   "unauthorized",
	ErrTokenExpired:   "token expired",
	ErrTokenInvalid:   "invalid token",
	ErrUserExists:     "user already exists",
	ErrUserNotFound:   "user not found",
	ErrPasswordWrong:  "incorrect password",
	ErrTokenMissing:   "token missing",
	ErrBillNotFound:   "bill not found",
	ErrBudgetNotFound: "budget not found",
	ErrAssetNotFound:  "asset not found",
	ErrAccessDenied:   "access denied",
}

// Message 获取错误码对应的消息
func Message(code int) string {
	if msg, ok := messages[code]; ok {
		return msg
	}
	return "unknown error"
}
```

- [ ] **3.2** 创建 `internal/pkg/errcode/errcode_test.go`

```go
// internal/pkg/errcode/errcode_test.go
package errcode

import "testing"

func TestMessage(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{Success, "success"},
		{ErrUnauthorized, "unauthorized"},
		{ErrTokenInvalid, "invalid token"},
		{99999, "unknown error"},
	}

	for _, tt := range tests {
		got := Message(tt.code)
		if got != tt.expected {
			t.Errorf("Message(%d) = %q, want %q", tt.code, got, tt.expected)
		}
	}
}
```

- [ ] **3.3** 创建 `internal/pkg/response/response.go`

```go
// internal/pkg/response/response.go
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sangchenglong/kapi/internal/pkg/errcode"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    errcode.Success,
		Message: errcode.Message(errcode.Success),
		Data:    data,
	})
}

// Error 错误响应（自定义错误码）
func Error(c *gin.Context, httpStatus int, code int) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: errcode.Message(code),
		Data:    nil,
	})
}

// ErrorWithMsg 错误响应（自定义消息）
func ErrorWithMsg(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: msg,
		Data:    nil,
	})
}

// BadRequest 参数错误响应
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    errcode.ErrInvalidParam,
		Message: msg,
		Data:    nil,
	})
}

// Unauthorized 未授权响应
func Unauthorized(c *gin.Context, code int) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    code,
		Message: errcode.Message(code),
		Data:    nil,
	})
}

// InternalError 内部错误响应
func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    errcode.ErrInternal,
		Message: msg,
		Data:    nil,
	})
}
```

- [ ] **3.4** 创建 `internal/pkg/response/response_test.go`

```go
// internal/pkg/response/response_test.go
package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sangchenglong/kapi/internal/pkg/errcode"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Success(c, gin.H{"id": 1})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Code != errcode.Success {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected message 'success', got %q", resp.Message)
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	BadRequest(c, "missing field")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Code != errcode.ErrInvalidParam {
		t.Errorf("expected code %d, got %d", errcode.ErrInvalidParam, resp.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Unauthorized(c, errcode.ErrTokenInvalid)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Code != errcode.ErrTokenInvalid {
		t.Errorf("expected code %d, got %d", errcode.ErrTokenInvalid, resp.Code)
	}
}
```

- [ ] **3.5** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/pkg/... -v
```

预期输出：所有测试通过。

- [ ] **3.6** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add unified response format and error codes"
```

---

## Task 4: User 模型 + Auth Service + JWT

### 目标
实现 User 模型、Auth Service（注册/登录/JWT 生成与校验），使用 bcrypt 做密码哈希。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/model/base.go` |
| Create | `internal/model/user.go` |
| Create | `internal/dao/user.go` |
| Create | `internal/dao/user_test.go` |
| Create | `internal/service/auth.go` |
| Create | `internal/service/auth_test.go` |

### Steps

- [ ] **4.1** 创建 `internal/model/base.go`

```go
// internal/model/base.go
package model

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel GORM 基础模型
type BaseModel struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
}

// AutoMigrate 自动迁移所有模型
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Bill{},
		&Budget{},
		&Asset{},
	)
}
```

- [ ] **4.2** 创建 `internal/model/user.go`

```go
// internal/model/user.go
package model

// User 用户模型
type User struct {
	BaseModel
	Username     string `gorm:"type:varchar(64);uniqueIndex;not null" json:"username"`
	PasswordHash string `gorm:"type:varchar(255);not null" json:"-"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
```

- [ ] **4.3** 创建 `internal/dao/user.go`

```go
// internal/dao/user.go
package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

// UserDAO 用户数据访问层
type UserDAO struct {
	db *gorm.DB
}

// NewUserDAO 创建 UserDAO 实例
func NewUserDAO(db *gorm.DB) *UserDAO {
	return &UserDAO{db: db}
}

// Create 创建用户
func (d *UserDAO) Create(ctx context.Context, user *model.User) error {
	return d.db.WithContext(ctx).Create(user).Error
}

// GetByUsername 根据用户名查询用户
func (d *UserDAO) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := d.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID 根据 ID 查询用户
func (d *UserDAO) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
```

- [ ] **4.4** 创建 `internal/service/auth.go`

```go
// internal/service/auth.go
package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

// AuthService 认证服务
type AuthService struct {
	userDAO *dao.UserDAO
	jwtCfg  config.JWTConfig
}

// NewAuthService 创建 AuthService 实例
func NewAuthService(userDAO *dao.UserDAO, jwtCfg config.JWTConfig) *AuthService {
	return &AuthService{
		userDAO: userDAO,
		jwtCfg:  jwtCfg,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse Token 响应
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Register 用户注册
func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (*model.User, error) {
	// 检查用户名是否已存在
	existing, err := s.userDAO.GetByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserExists
	}

	// 生成密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: string(hash),
	}

	if err := s.userDAO.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error) {
	user, err := s.userDAO.GetByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// 校验密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrPasswordWrong
	}

	// 生成 JWT Token
	token, expiresAt, err := s.generateToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateToken 校验 JWT Token，返回 user_id
func (s *AuthService) ValidateToken(tokenString string) (uint64, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return []byte(s.jwtCfg.Secret), nil
	})
	if err != nil {
		return 0, ErrTokenInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, ErrTokenInvalid
	}

	// 提取 user_id
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, ErrTokenInvalid
	}

	return uint64(userIDFloat), nil
}

// generateToken 生成 JWT Token
func (s *AuthService) generateToken(userID uint64) (string, int64, error) {
	expiresAt := time.Now().Add(time.Duration(s.jwtCfg.ExpireHours) * time.Hour)

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtCfg.Secret))
	if err != nil {
		return "", 0, err
	}

	return tokenString, expiresAt.Unix(), nil
}

// 业务错误定义
var (
	ErrUserExists    = errors.New("user already exists")
	ErrUserNotFound  = errors.New("user not found")
	ErrPasswordWrong = errors.New("incorrect password")
	ErrTokenInvalid  = errors.New("invalid token")
)
```

- [ ] **4.5** 创建 `internal/service/auth_test.go`

```go
// internal/service/auth_test.go
package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&model.User{})
	return db
}

func setupAuthService(t *testing.T) *AuthService {
	db := setupTestDB(t)
	userDAO := dao.NewUserDAO(db)
	jwtCfg := config.JWTConfig{
		Secret:      "test-secret-key",
		ExpireHours: 24,
	}
	return NewAuthService(userDAO, jwtCfg)
}

func TestRegister_Success(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})

	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", user.Username)
	}
	if user.ID == 0 {
		t.Error("expected user ID > 0")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})

	_, err := svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password456",
	})

	if err != ErrUserExists {
		t.Errorf("expected ErrUserExists, got %v", err)
	}
}

func TestLogin_Success(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})

	resp, err := svc.Login(ctx, &LoginRequest{
		Username: "testuser",
		Password: "password123",
	})

	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.ExpiresAt == 0 {
		t.Error("expected non-zero expires_at")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})

	_, err := svc.Login(ctx, &LoginRequest{
		Username: "testuser",
		Password: "wrongpassword",
	})

	if err != ErrPasswordWrong {
		t.Errorf("expected ErrPasswordWrong, got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	_, err := svc.Login(ctx, &LoginRequest{
		Username: "nonexistent",
		Password: "password123",
	})

	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestValidateToken_Success(t *testing.T) {
	svc := setupAuthService(t)
	ctx := context.Background()

	svc.Register(ctx, &RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})

	resp, _ := svc.Login(ctx, &LoginRequest{
		Username: "testuser",
		Password: "password123",
	})

	userID, err := svc.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("validate token failed: %v", err)
	}
	if userID == 0 {
		t.Error("expected user_id > 0")
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	svc := setupAuthService(t)

	_, err := svc.ValidateToken("invalid.token.string")
	if err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}
```

- [ ] **4.6** 安装依赖

```bash
cd /Users/sangchenglong/go/src/kapi
go get github.com/golang-jwt/jwt/v5@latest
go get golang.org/x/crypto@latest
go get gorm.io/driver/sqlite@latest  # 仅用于测试
go mod tidy
```

- [ ] **4.7** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/service/ -v -run TestRegister
go test ./internal/service/ -v -run TestLogin
go test ./internal/service/ -v -run TestValidateToken
```

预期输出：所有测试通过。

- [ ] **4.8** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add User model, Auth Service with JWT and bcrypt"
```

---

## Task 5: Auth 中间件 + Auth Handler

### 目标
实现 JWT 认证中间件（从 Authorization header 提取 token，校验后注入 user_id 到 context）和 Auth Handler（注册/登录接口）。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/middleware/auth.go` |
| Create | `internal/middleware/auth_test.go` |
| Create | `internal/handler/auth.go` |
| Create | `internal/handler/auth_test.go` |

### Steps

- [ ] **5.1** 创建 `internal/middleware/auth.go`

```go
// internal/middleware/auth.go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/errcode"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// JWTAuth JWT 认证中间件
func JWTAuth(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization header 提取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, errcode.ErrTokenMissing)
			c.Abort()
			return
		}

		// 校验 Bearer 前缀
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, errcode.ErrTokenInvalid)
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 校验 token
		userID, err := authSvc.ValidateToken(tokenString)
		if err != nil {
			response.Unauthorized(c, errcode.ErrTokenInvalid)
			c.Abort()
			return
		}

		// 注入 user_id 到 context
		c.Set("user_id", userID)
		c.Next()
	}
}

// GetUserID 从 context 获取 user_id 的辅助函数
func GetUserID(c *gin.Context) uint64 {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	id, ok := userID.(uint64)
	if !ok {
		return 0
	}
	return id
}
```

- [ ] **5.2** 创建 `internal/middleware/auth_test.go`

```go
// internal/middleware/auth_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupAuthSvc(t *testing.T) *service.AuthService {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.User{})
	userDAO := dao.NewUserDAO(db)
	jwtCfg := config.JWTConfig{Secret: "test-secret", ExpireHours: 24}
	return service.NewAuthService(userDAO, jwtCfg)
}

func TestJWTAuth_NoHeader(t *testing.T) {
	authSvc := setupAuthSvc(t)

	r := gin.New()
	r.GET("/test", JWTAuth(authSvc), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	authSvc := setupAuthSvc(t)

	r := gin.New()
	r.GET("/test", JWTAuth(authSvc), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	authSvc := setupAuthSvc(t)
	ctx := req_context()

	// 注册并登录获取 token
	authSvc.Register(ctx, &service.RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})
	tokenResp, _ := authSvc.Login(ctx, &service.LoginRequest{
		Username: "testuser",
		Password: "password123",
	})

	r := gin.New()
	r.GET("/test", JWTAuth(authSvc), func(c *gin.Context) {
		userID := GetUserID(c)
		c.JSON(200, gin.H{"user_id": userID})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func req_context() context.Context {
	return context.Background()
}
```

注意：测试文件需要在顶部添加 `import "context"`。

- [ ] **5.3** 创建 `internal/handler/auth.go`

```go
// internal/handler/auth.go
package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/pkg/errcode"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authSvc *service.AuthService
}

// NewAuthHandler 创建 AuthHandler 实例
func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

// Register 用户注册
// POST /api/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "username and password are required (password min 6 chars)")
		return
	}

	user, err := h.authSvc.Register(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			response.Error(c, 409, errcode.ErrUserExists)
			return
		}
		response.InternalError(c, "registration failed")
		return
	}

	response.Success(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
	})
}

// Login 用户登录
// POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "username and password are required")
		return
	}

	tokenResp, err := h.authSvc.Login(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Error(c, 401, errcode.ErrUserNotFound)
			return
		}
		if errors.Is(err, service.ErrPasswordWrong) {
			response.Error(c, 401, errcode.ErrPasswordWrong)
			return
		}
		response.InternalError(c, "login failed")
		return
	}

	response.Success(c, tokenResp)
}
```

- [ ] **5.4** 创建 `internal/handler/auth_test.go`

```go
// internal/handler/auth_test.go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupAuthHandler(t *testing.T) (*AuthHandler, *gin.Engine) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.User{})
	userDAO := dao.NewUserDAO(db)
	jwtCfg := config.JWTConfig{Secret: "test-secret", ExpireHours: 24}
	authSvc := service.NewAuthService(userDAO, jwtCfg)
	handler := NewAuthHandler(authSvc)

	r := gin.New()
	r.POST("/api/auth/register", handler.Register)
	r.POST("/api/auth/login", handler.Login)

	return handler, r
}

func TestRegisterHandler_Success(t *testing.T) {
	_, r := setupAuthHandler(t)

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
}

func TestRegisterHandler_DuplicateUser(t *testing.T) {
	_, r := setupAuthHandler(t)

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})

	// 第一次注册
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 第二次注册（重复）
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestLoginHandler_Success(t *testing.T) {
	_, r := setupAuthHandler(t)

	// 先注册
	regBody, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 登录
	loginBody, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	_, r := setupAuthHandler(t)

	// 先注册
	regBody, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(regBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 错误密码登录
	loginBody, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "wrongpassword",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
```

- [ ] **5.5** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/middleware/ -v
go test ./internal/handler/ -v -run TestRegister
go test ./internal/handler/ -v -run TestLogin
```

预期输出：所有测试通过。

- [ ] **5.6** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add JWT auth middleware and auth handler (register/login)"
```

---

## Task 6: Bill 模型 + DAO + Service + Handler

### 目标
实现账单的完整 CRUD（含软删除），所有查询自动带 `WHERE user_id = ? AND is_deleted = 0`。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/model/bill.go` |
| Create | `internal/dao/bill.go` |
| Create | `internal/dao/bill_test.go` |
| Create | `internal/service/bill.go` |
| Create | `internal/service/bill_test.go` |
| Create | `internal/handler/bill.go` |
| Create | `internal/handler/bill_test.go` |

### Steps

- [ ] **6.1** 创建 `internal/model/bill.go`

```go
// internal/model/bill.go
package model

import "time"

// Bill 账单模型
type Bill struct {
	BaseModel
	UserID    uint64    `gorm:"index:idx_user_date;index:idx_user_category;not null" json:"user_id"`
	Amount    float64   `gorm:"type:decimal(12,2);not null" json:"amount"`
	Category  string    `gorm:"type:varchar(32);index:idx_user_category;not null" json:"category"`
	Merchant  string    `gorm:"type:varchar(128)" json:"merchant"`
	Date      time.Time `gorm:"type:date;index:idx_user_date;not null" json:"date"`
	Note      string    `gorm:"type:varchar(255)" json:"note"`
	IsDeleted bool      `gorm:"not null;default:0" json:"-"`
}

// TableName 指定表名
func (Bill) TableName() string {
	return "bills"
}
```

- [ ] **6.2** 创建 `internal/dao/bill.go`

```go
// internal/dao/bill.go
package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

// BillDAO 账单数据访问层
type BillDAO struct {
	db *gorm.DB
}

// NewBillDAO 创建 BillDAO 实例
func NewBillDAO(db *gorm.DB) *BillDAO {
	return &BillDAO{db: db}
}

// Create 创建账单
func (d *BillDAO) Create(ctx context.Context, bill *model.Bill) error {
	return d.db.WithContext(ctx).Create(bill).Error
}

// Update 更新账单（仅允许更新自己的数据）
func (d *BillDAO) Update(ctx context.Context, userID uint64, bill *model.Bill) error {
	return d.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = ?", bill.ID, userID, false).
		Updates(map[string]interface{}{
			"amount":   bill.Amount,
			"category": bill.Category,
			"merchant": bill.Merchant,
			"date":     bill.Date,
			"note":     bill.Note,
		}).Error
}

// Delete 软删除账单
func (d *BillDAO) Delete(ctx context.Context, userID uint64, id uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.Bill{}).
		Where("id = ? AND user_id = ? AND is_deleted = ?", id, userID, false).
		Update("is_deleted", true).Error
}

// GetByID 根据 ID 查询账单（含多租户隔离）
func (d *BillDAO) GetByID(ctx context.Context, userID uint64, id uint64) (*model.Bill, error) {
	var bill model.Bill
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = ?", id, userID, false).
		First(&bill).Error
	if err != nil {
		return nil, err
	}
	return &bill, nil
}

// BillFilter 账单查询过滤条件
type BillFilter struct {
	Category  string
	StartDate string
	EndDate   string
	Limit     int
	Offset    int
}

// List 查询账单列表（含多租户隔离 + 软删除过滤）
func (d *BillDAO) List(ctx context.Context, userID uint64, filter *BillFilter) ([]model.Bill, int64, error) {
	query := d.db.WithContext(ctx).
		Where("user_id = ? AND is_deleted = ?", userID, false)

	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.StartDate != "" {
		query = query.Where("date >= ?", filter.StartDate)
	}
	if filter.EndDate != "" {
		query = query.Where("date <= ?", filter.EndDate)
	}

	var total int64
	query.Model(&model.Bill{}).Count(&total)

	var bills []model.Bill
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	err := query.Order("date DESC, id DESC").
		Limit(limit).Offset(filter.Offset).
		Find(&bills).Error

	return bills, total, err
}
```

- [ ] **6.3** 创建 `internal/service/bill.go`

```go
// internal/service/bill.go
package service

import (
	"context"
	"time"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

// BillService 账单服务
type BillService struct {
	billDAO *dao.BillDAO
}

// NewBillService 创建 BillService 实例
func NewBillService(billDAO *dao.BillDAO) *BillService {
	return &BillService{billDAO: billDAO}
}

// CreateBillRequest 创建账单请求
type CreateBillRequest struct {
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Category string  `json:"category" binding:"required,max=32"`
	Merchant string  `json:"merchant" binding:"max=128"`
	Date     string  `json:"date" binding:"required"`
	Note     string  `json:"note" binding:"max=255"`
}

// UpdateBillRequest 更新账单请求
type UpdateBillRequest struct {
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Category string  `json:"category" binding:"required,max=32"`
	Merchant string  `json:"merchant" binding:"max=128"`
	Date     string  `json:"date" binding:"required"`
	Note     string  `json:"note" binding:"max=255"`
}

// ListBillsRequest 查询账单列表请求
type ListBillsRequest struct {
	Category  string `form:"category"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Page      int    `form:"page,default=1"`
	PageSize  int    `form:"page_size,default=20"`
}

// Create 创建账单
func (s *BillService) Create(ctx context.Context, userID uint64, req *CreateBillRequest) (*model.Bill, error) {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, ErrInvalidDate
	}

	bill := &model.Bill{
		UserID:   userID,
		Amount:   req.Amount,
		Category: req.Category,
		Merchant: req.Merchant,
		Date:     date,
		Note:     req.Note,
	}

	if err := s.billDAO.Create(ctx, bill); err != nil {
		return nil, err
	}
	return bill, nil
}

// Update 更新账单
func (s *BillService) Update(ctx context.Context, userID uint64, billID uint64, req *UpdateBillRequest) error {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return ErrInvalidDate
	}

	bill := &model.Bill{
		Amount:   req.Amount,
		Category: req.Category,
		Merchant: req.Merchant,
		Date:     date,
		Note:     req.Note,
	}
	bill.ID = billID

	return s.billDAO.Update(ctx, userID, bill)
}

// Delete 删除账单（软删除）
func (s *BillService) Delete(ctx context.Context, userID uint64, billID uint64) error {
	return s.billDAO.Delete(ctx, userID, billID)
}

// GetByID 获取单条账单
func (s *BillService) GetByID(ctx context.Context, userID uint64, billID uint64) (*model.Bill, error) {
	return s.billDAO.GetByID(ctx, userID, billID)
}

// List 查询账单列表
func (s *BillService) List(ctx context.Context, userID uint64, req *ListBillsRequest) ([]model.Bill, int64, error) {
	limit := req.PageSize
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (req.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	filter := &dao.BillFilter{
		Category:  req.Category,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Limit:     limit,
		Offset:    offset,
	}

	return s.billDAO.List(ctx, userID, filter)
}

// 业务错误
var ErrInvalidDate = errors.New("invalid date format, expected YYYY-MM-DD")
```

注意：需要在文件顶部 import 中添加 `"errors"`。

- [ ] **6.4** 创建 `internal/handler/bill.go`

```go
// internal/handler/bill.go
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/pkg/errcode"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// BillHandler 账单处理器
type BillHandler struct {
	billSvc *service.BillService
}

// NewBillHandler 创建 BillHandler 实例
func NewBillHandler(billSvc *service.BillService) *BillHandler {
	return &BillHandler{billSvc: billSvc}
}

// List 查询账单列表
// GET /api/bills
func (h *BillHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.ListBillsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	bills, total, err := h.billSvc.List(c.Request.Context(), userID, &req)
	if err != nil {
		response.InternalError(c, "failed to query bills")
		return
	}

	response.Success(c, gin.H{
		"list":  bills,
		"total": total,
		"page":  req.Page,
	})
}

// Create 创建账单
// POST /api/bills
func (h *BillHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.CreateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid bill data: amount, category, date are required")
		return
	}

	bill, err := h.billSvc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		if err == service.ErrInvalidDate {
			response.BadRequest(c, "invalid date format, expected YYYY-MM-DD")
			return
		}
		response.InternalError(c, "failed to create bill")
		return
	}

	response.Success(c, bill)
}

// Update 更新账单
// PUT /api/bills/:id
func (h *BillHandler) Update(c *gin.Context) {
	userID := middleware.GetUserID(c)

	billID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bill id")
		return
	}

	var req service.UpdateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid bill data")
		return
	}

	if err := h.billSvc.Update(c.Request.Context(), userID, billID, &req); err != nil {
		if err == service.ErrInvalidDate {
			response.BadRequest(c, "invalid date format, expected YYYY-MM-DD")
			return
		}
		response.InternalError(c, "failed to update bill")
		return
	}

	response.Success(c, gin.H{"id": billID})
}

// Delete 删除账单（软删除）
// DELETE /api/bills/:id
func (h *BillHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)

	billID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid bill id")
		return
	}

	if err := h.billSvc.Delete(c.Request.Context(), userID, billID); err != nil {
		response.Error(c, 404, errcode.ErrBillNotFound)
		return
	}

	response.Success(c, gin.H{"id": billID})
}
```

- [ ] **6.5** 创建 `internal/service/bill_test.go`

```go
// internal/service/bill_test.go
package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

func setupBillService(t *testing.T) *BillService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&model.Bill{})
	billDAO := dao.NewBillDAO(db)
	return NewBillService(billDAO)
}

func TestBillService_Create(t *testing.T) {
	svc := setupBillService(t)
	ctx := context.Background()

	bill, err := svc.Create(ctx, 1, &CreateBillRequest{
		Amount:   38.00,
		Category: "餐饮",
		Merchant: "星巴克",
		Date:     "2026-04-28",
		Note:     "拿铁",
	})

	if err != nil {
		t.Fatalf("create bill failed: %v", err)
	}
	if bill.ID == 0 {
		t.Error("expected bill ID > 0")
	}
	if bill.UserID != 1 {
		t.Errorf("expected user_id 1, got %d", bill.UserID)
	}
}

func TestBillService_Create_InvalidDate(t *testing.T) {
	svc := setupBillService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, &CreateBillRequest{
		Amount:   38.00,
		Category: "餐饮",
		Date:     "invalid-date",
	})

	if err != ErrInvalidDate {
		t.Errorf("expected ErrInvalidDate, got %v", err)
	}
}

func TestBillService_List_MultiTenantIsolation(t *testing.T) {
	svc := setupBillService(t)
	ctx := context.Background()

	// 用户 1 创建账单
	svc.Create(ctx, 1, &CreateBillRequest{
		Amount: 100, Category: "餐饮", Date: "2026-04-28",
	})
	// 用户 2 创建账单
	svc.Create(ctx, 2, &CreateBillRequest{
		Amount: 200, Category: "交通", Date: "2026-04-28",
	})

	// 用户 1 只能看到自己的账单
	bills, total, err := svc.List(ctx, 1, &ListBillsRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list bills failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(bills) != 1 {
		t.Errorf("expected 1 bill, got %d", len(bills))
	}
	if bills[0].Amount != 100 {
		t.Errorf("expected amount 100, got %f", bills[0].Amount)
	}
}

func TestBillService_Delete_SoftDelete(t *testing.T) {
	svc := setupBillService(t)
	ctx := context.Background()

	bill, _ := svc.Create(ctx, 1, &CreateBillRequest{
		Amount: 50, Category: "娱乐", Date: "2026-04-28",
	})

	// 软删除
	err := svc.Delete(ctx, 1, bill.ID)
	if err != nil {
		t.Fatalf("delete bill failed: %v", err)
	}

	// 查询列表应该看不到
	bills, total, _ := svc.List(ctx, 1, &ListBillsRequest{Page: 1, PageSize: 20})
	if total != 0 {
		t.Errorf("expected total 0 after delete, got %d", total)
	}
	if len(bills) != 0 {
		t.Errorf("expected 0 bills after delete, got %d", len(bills))
	}
}
```

- [ ] **6.6** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/dao/ -v
go test ./internal/service/ -v -run TestBill
go test ./internal/handler/ -v -run TestBill
```

预期输出：所有测试通过。

- [ ] **6.7** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add Bill CRUD with soft delete and multi-tenant isolation"
```

---

## Task 7: Budget 模型 + DAO + Service + Handler

### 目标
实现预算的 CRUD 操作。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/model/budget.go` |
| Create | `internal/dao/budget.go` |
| Create | `internal/service/budget.go` |
| Create | `internal/service/budget_test.go` |
| Create | `internal/handler/budget.go` |

### Steps

- [ ] **7.1** 创建 `internal/model/budget.go`

```go
// internal/model/budget.go
package model

// Budget 预算模型
type Budget struct {
	BaseModel
	UserID   uint64  `gorm:"index:idx_user_category_period;not null" json:"user_id"`
	Category string  `gorm:"type:varchar(32);index:idx_user_category_period;not null" json:"category"`
	Amount   float64 `gorm:"type:decimal(12,2);not null" json:"amount"`
	Period   string  `gorm:"type:varchar(16);index:idx_user_category_period;not null;default:monthly" json:"period"`
}

// TableName 指定表名
func (Budget) TableName() string {
	return "budgets"
}
```

- [ ] **7.2** 创建 `internal/dao/budget.go`

```go
// internal/dao/budget.go
package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

// BudgetDAO 预算数据访问层
type BudgetDAO struct {
	db *gorm.DB
}

// NewBudgetDAO 创建 BudgetDAO 实例
func NewBudgetDAO(db *gorm.DB) *BudgetDAO {
	return &BudgetDAO{db: db}
}

// Create 创建预算
func (d *BudgetDAO) Create(ctx context.Context, budget *model.Budget) error {
	return d.db.WithContext(ctx).Create(budget).Error
}

// Update 更新预算
func (d *BudgetDAO) Update(ctx context.Context, userID uint64, budget *model.Budget) error {
	return d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", budget.ID, userID).
		Updates(map[string]interface{}{
			"category": budget.Category,
			"amount":   budget.Amount,
			"period":   budget.Period,
		}).Error
}

// GetByID 根据 ID 查询预算
func (d *BudgetDAO) GetByID(ctx context.Context, userID uint64, id uint64) (*model.Budget, error) {
	var budget model.Budget
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&budget).Error
	if err != nil {
		return nil, err
	}
	return &budget, nil
}

// List 查询预算列表
func (d *BudgetDAO) List(ctx context.Context, userID uint64, period string) ([]model.Budget, error) {
	query := d.db.WithContext(ctx).Where("user_id = ?", userID)
	if period != "" {
		query = query.Where("period = ?", period)
	}

	var budgets []model.Budget
	err := query.Order("category ASC").Find(&budgets).Error
	return budgets, err
}
```

- [ ] **7.3** 创建 `internal/service/budget.go`

```go
// internal/service/budget.go
package service

import (
	"context"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

// BudgetService 预算服务
type BudgetService struct {
	budgetDAO *dao.BudgetDAO
}

// NewBudgetService 创建 BudgetService 实例
func NewBudgetService(budgetDAO *dao.BudgetDAO) *BudgetService {
	return &BudgetService{budgetDAO: budgetDAO}
}

// CreateBudgetRequest 创建预算请求
type CreateBudgetRequest struct {
	Category string  `json:"category" binding:"required,max=32"`
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Period   string  `json:"period" binding:"required,oneof=monthly weekly"`
}

// UpdateBudgetRequest 更新预算请求
type UpdateBudgetRequest struct {
	Category string  `json:"category" binding:"required,max=32"`
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	Period   string  `json:"period" binding:"required,oneof=monthly weekly"`
}

// ListBudgetsRequest 查询预算列表请求
type ListBudgetsRequest struct {
	Period string `form:"period"`
}

// Create 创建预算
func (s *BudgetService) Create(ctx context.Context, userID uint64, req *CreateBudgetRequest) (*model.Budget, error) {
	budget := &model.Budget{
		UserID:   userID,
		Category: req.Category,
		Amount:   req.Amount,
		Period:   req.Period,
	}

	if err := s.budgetDAO.Create(ctx, budget); err != nil {
		return nil, err
	}
	return budget, nil
}

// Update 更新预算
func (s *BudgetService) Update(ctx context.Context, userID uint64, budgetID uint64, req *UpdateBudgetRequest) error {
	budget := &model.Budget{
		Category: req.Category,
		Amount:   req.Amount,
		Period:   req.Period,
	}
	budget.ID = budgetID

	return s.budgetDAO.Update(ctx, userID, budget)
}

// GetByID 获取单条预算
func (s *BudgetService) GetByID(ctx context.Context, userID uint64, budgetID uint64) (*model.Budget, error) {
	return s.budgetDAO.GetByID(ctx, userID, budgetID)
}

// List 查询预算列表
func (s *BudgetService) List(ctx context.Context, userID uint64, req *ListBudgetsRequest) ([]model.Budget, error) {
	return s.budgetDAO.List(ctx, userID, req.Period)
}
```

- [ ] **7.4** 创建 `internal/handler/budget.go`

```go
// internal/handler/budget.go
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/pkg/errcode"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// BudgetHandler 预算处理器
type BudgetHandler struct {
	budgetSvc *service.BudgetService
}

// NewBudgetHandler 创建 BudgetHandler 实例
func NewBudgetHandler(budgetSvc *service.BudgetService) *BudgetHandler {
	return &BudgetHandler{budgetSvc: budgetSvc}
}

// List 查询预算列表
// GET /api/budgets
func (h *BudgetHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.ListBudgetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	budgets, err := h.budgetSvc.List(c.Request.Context(), userID, &req)
	if err != nil {
		response.InternalError(c, "failed to query budgets")
		return
	}

	response.Success(c, gin.H{"list": budgets})
}

// Create 创建预算
// POST /api/budgets
func (h *BudgetHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid budget data: category, amount, period(monthly/weekly) are required")
		return
	}

	budget, err := h.budgetSvc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		response.InternalError(c, "failed to create budget")
		return
	}

	response.Success(c, budget)
}

// Update 更新预算
// PUT /api/budgets/:id
func (h *BudgetHandler) Update(c *gin.Context) {
	userID := middleware.GetUserID(c)

	budgetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid budget id")
		return
	}

	var req service.UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid budget data")
		return
	}

	if err := h.budgetSvc.Update(c.Request.Context(), userID, budgetID, &req); err != nil {
		response.Error(c, 404, errcode.ErrBudgetNotFound)
		return
	}

	response.Success(c, gin.H{"id": budgetID})
}
```

- [ ] **7.5** 创建 `internal/service/budget_test.go`

```go
// internal/service/budget_test.go
package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

func setupBudgetService(t *testing.T) *BudgetService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&model.Budget{})
	budgetDAO := dao.NewBudgetDAO(db)
	return NewBudgetService(budgetDAO)
}

func TestBudgetService_Create(t *testing.T) {
	svc := setupBudgetService(t)
	ctx := context.Background()

	budget, err := svc.Create(ctx, 1, &CreateBudgetRequest{
		Category: "餐饮",
		Amount:   3000,
		Period:   "monthly",
	})

	if err != nil {
		t.Fatalf("create budget failed: %v", err)
	}
	if budget.ID == 0 {
		t.Error("expected budget ID > 0")
	}
	if budget.UserID != 1 {
		t.Errorf("expected user_id 1, got %d", budget.UserID)
	}
	if budget.Amount != 3000 {
		t.Errorf("expected amount 3000, got %f", budget.Amount)
	}
}

func TestBudgetService_List_MultiTenantIsolation(t *testing.T) {
	svc := setupBudgetService(t)
	ctx := context.Background()

	// 用户 1 创建预算
	svc.Create(ctx, 1, &CreateBudgetRequest{
		Category: "餐饮", Amount: 3000, Period: "monthly",
	})
	// 用户 2 创建预算
	svc.Create(ctx, 2, &CreateBudgetRequest{
		Category: "交通", Amount: 1000, Period: "monthly",
	})

	// 用户 1 只能看到自己的预算
	budgets, err := svc.List(ctx, 1, &ListBudgetsRequest{})
	if err != nil {
		t.Fatalf("list budgets failed: %v", err)
	}
	if len(budgets) != 1 {
		t.Errorf("expected 1 budget, got %d", len(budgets))
	}
}

func TestBudgetService_Update(t *testing.T) {
	svc := setupBudgetService(t)
	ctx := context.Background()

	budget, _ := svc.Create(ctx, 1, &CreateBudgetRequest{
		Category: "娱乐", Amount: 500, Period: "monthly",
	})

	err := svc.Update(ctx, 1, budget.ID, &UpdateBudgetRequest{
		Category: "娱乐", Amount: 800, Period: "monthly",
	})
	if err != nil {
		t.Fatalf("update budget failed: %v", err)
	}

	updated, _ := svc.GetByID(ctx, 1, budget.ID)
	if updated.Amount != 800 {
		t.Errorf("expected amount 800, got %f", updated.Amount)
	}
}
```

- [ ] **7.6** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/service/ -v -run TestBudget
```

预期输出：所有测试通过。

- [ ] **7.7** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add Budget CRUD with multi-tenant isolation"
```

---

## Task 8: Asset 模型 + DAO + Service + Handler

### 目标
实现资产的 CRUD 操作。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/model/asset.go` |
| Create | `internal/dao/asset.go` |
| Create | `internal/service/asset.go` |
| Create | `internal/service/asset_test.go` |
| Create | `internal/handler/asset.go` |

### Steps

- [ ] **8.1** 创建 `internal/model/asset.go`

```go
// internal/model/asset.go
package model

// Asset 资产模型
type Asset struct {
	BaseModel
	UserID  uint64  `gorm:"index:idx_user_type;not null" json:"user_id"`
	Name    string  `gorm:"type:varchar(64);not null" json:"name"`
	Type    string  `gorm:"type:varchar(16);index:idx_user_type;not null" json:"type"`
	Balance float64 `gorm:"type:decimal(14,2);not null;default:0" json:"balance"`
}

// TableName 指定表名
func (Asset) TableName() string {
	return "assets"
}

// 资产类型常量
const (
	AssetTypeCash       = "cash"
	AssetTypeCredit     = "credit"
	AssetTypeInvestment = "investment"
	AssetTypeDebt       = "debt"
)

// ValidAssetTypes 合法的资产类型白名单
var ValidAssetTypes = map[string]bool{
	AssetTypeCash:       true,
	AssetTypeCredit:     true,
	AssetTypeInvestment: true,
	AssetTypeDebt:       true,
}
```

- [ ] **8.2** 创建 `internal/dao/asset.go`

```go
// internal/dao/asset.go
package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

// AssetDAO 资产数据访问层
type AssetDAO struct {
	db *gorm.DB
}

// NewAssetDAO 创建 AssetDAO 实例
func NewAssetDAO(db *gorm.DB) *AssetDAO {
	return &AssetDAO{db: db}
}

// Create 创建资产
func (d *AssetDAO) Create(ctx context.Context, asset *model.Asset) error {
	return d.db.WithContext(ctx).Create(asset).Error
}

// Update 更新资产
func (d *AssetDAO) Update(ctx context.Context, userID uint64, asset *model.Asset) error {
	return d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", asset.ID, userID).
		Updates(map[string]interface{}{
			"name":    asset.Name,
			"type":    asset.Type,
			"balance": asset.Balance,
		}).Error
}

// GetByID 根据 ID 查询资产
func (d *AssetDAO) GetByID(ctx context.Context, userID uint64, id uint64) (*model.Asset, error) {
	var asset model.Asset
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// List 查询资产列表
func (d *AssetDAO) List(ctx context.Context, userID uint64, assetType string) ([]model.Asset, error) {
	query := d.db.WithContext(ctx).Where("user_id = ?", userID)
	if assetType != "" {
		query = query.Where("type = ?", assetType)
	}

	var assets []model.Asset
	err := query.Order("type ASC, name ASC").Find(&assets).Error
	return assets, err
}
```

- [ ] **8.3** 创建 `internal/service/asset.go`

```go
// internal/service/asset.go
package service

import (
	"context"
	"errors"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

// AssetService 资产服务
type AssetService struct {
	assetDAO *dao.AssetDAO
}

// NewAssetService 创建 AssetService 实例
func NewAssetService(assetDAO *dao.AssetDAO) *AssetService {
	return &AssetService{assetDAO: assetDAO}
}

// CreateAssetRequest 创建资产请求
type CreateAssetRequest struct {
	Name    string  `json:"name" binding:"required,max=64"`
	Type    string  `json:"type" binding:"required,oneof=cash credit investment debt"`
	Balance float64 `json:"balance"`
}

// UpdateAssetRequest 更新资产请求
type UpdateAssetRequest struct {
	Name    string  `json:"name" binding:"required,max=64"`
	Type    string  `json:"type" binding:"required,oneof=cash credit investment debt"`
	Balance float64 `json:"balance"`
}

// ListAssetsRequest 查询资产列表请求
type ListAssetsRequest struct {
	Type string `form:"type"`
}

// Create 创建资产
func (s *AssetService) Create(ctx context.Context, userID uint64, req *CreateAssetRequest) (*model.Asset, error) {
	if !model.ValidAssetTypes[req.Type] {
		return nil, ErrInvalidAssetType
	}

	asset := &model.Asset{
		UserID:  userID,
		Name:    req.Name,
		Type:    req.Type,
		Balance: req.Balance,
	}

	if err := s.assetDAO.Create(ctx, asset); err != nil {
		return nil, err
	}
	return asset, nil
}

// Update 更新资产
func (s *AssetService) Update(ctx context.Context, userID uint64, assetID uint64, req *UpdateAssetRequest) error {
	if !model.ValidAssetTypes[req.Type] {
		return ErrInvalidAssetType
	}

	asset := &model.Asset{
		Name:    req.Name,
		Type:    req.Type,
		Balance: req.Balance,
	}
	asset.ID = assetID

	return s.assetDAO.Update(ctx, userID, asset)
}

// GetByID 获取单条资产
func (s *AssetService) GetByID(ctx context.Context, userID uint64, assetID uint64) (*model.Asset, error) {
	return s.assetDAO.GetByID(ctx, userID, assetID)
}

// List 查询资产列表
func (s *AssetService) List(ctx context.Context, userID uint64, req *ListAssetsRequest) ([]model.Asset, error) {
	if req.Type != "" && !model.ValidAssetTypes[req.Type] {
		return nil, ErrInvalidAssetType
	}
	return s.assetDAO.List(ctx, userID, req.Type)
}

// 业务错误
var ErrInvalidAssetType = errors.New("invalid asset type, must be one of: cash, credit, investment, debt")
```

- [ ] **8.4** 创建 `internal/handler/asset.go`

```go
// internal/handler/asset.go
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/pkg/errcode"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

// AssetHandler 资产处理器
type AssetHandler struct {
	assetSvc *service.AssetService
}

// NewAssetHandler 创建 AssetHandler 实例
func NewAssetHandler(assetSvc *service.AssetService) *AssetHandler {
	return &AssetHandler{assetSvc: assetSvc}
}

// List 查询资产列表
// GET /api/assets
func (h *AssetHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.ListAssetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	assets, err := h.assetSvc.List(c.Request.Context(), userID, &req)
	if err != nil {
		if err == service.ErrInvalidAssetType {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, "failed to query assets")
		return
	}

	response.Success(c, gin.H{"list": assets})
}

// Create 创建资产
// POST /api/assets
func (h *AssetHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req service.CreateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid asset data: name, type(cash/credit/investment/debt) are required")
		return
	}

	asset, err := h.assetSvc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		if err == service.ErrInvalidAssetType {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, "failed to create asset")
		return
	}

	response.Success(c, asset)
}

// Update 更新资产
// PUT /api/assets/:id
func (h *AssetHandler) Update(c *gin.Context) {
	userID := middleware.GetUserID(c)

	assetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid asset id")
		return
	}

	var req service.UpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid asset data")
		return
	}

	if err := h.assetSvc.Update(c.Request.Context(), userID, assetID, &req); err != nil {
		if err == service.ErrInvalidAssetType {
			response.BadRequest(c, err.Error())
			return
		}
		response.Error(c, 404, errcode.ErrAssetNotFound)
		return
	}

	response.Success(c, gin.H{"id": assetID})
}
```

- [ ] **8.5** 创建 `internal/service/asset_test.go`

```go
// internal/service/asset_test.go
package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

func setupAssetService(t *testing.T) *AssetService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&model.Asset{})
	assetDAO := dao.NewAssetDAO(db)
	return NewAssetService(assetDAO)
}

func TestAssetService_Create(t *testing.T) {
	svc := setupAssetService(t)
	ctx := context.Background()

	asset, err := svc.Create(ctx, 1, &CreateAssetRequest{
		Name:    "招商银行储蓄卡",
		Type:    "cash",
		Balance: 12000,
	})

	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}
	if asset.ID == 0 {
		t.Error("expected asset ID > 0")
	}
	if asset.Balance != 12000 {
		t.Errorf("expected balance 12000, got %f", asset.Balance)
	}
}

func TestAssetService_Create_InvalidType(t *testing.T) {
	svc := setupAssetService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, &CreateAssetRequest{
		Name:    "test",
		Type:    "invalid_type",
		Balance: 100,
	})

	if err != ErrInvalidAssetType {
		t.Errorf("expected ErrInvalidAssetType, got %v", err)
	}
}

func TestAssetService_List_MultiTenantIsolation(t *testing.T) {
	svc := setupAssetService(t)
	ctx := context.Background()

	svc.Create(ctx, 1, &CreateAssetRequest{
		Name: "储蓄卡", Type: "cash", Balance: 10000,
	})
	svc.Create(ctx, 2, &CreateAssetRequest{
		Name: "信用卡", Type: "credit", Balance: -5000,
	})

	assets, err := svc.List(ctx, 1, &ListAssetsRequest{})
	if err != nil {
		t.Fatalf("list assets failed: %v", err)
	}
	if len(assets) != 1 {
		t.Errorf("expected 1 asset, got %d", len(assets))
	}
}

func TestAssetService_Update(t *testing.T) {
	svc := setupAssetService(t)
	ctx := context.Background()

	asset, _ := svc.Create(ctx, 1, &CreateAssetRequest{
		Name: "储蓄卡", Type: "cash", Balance: 10000,
	})

	err := svc.Update(ctx, 1, asset.ID, &UpdateAssetRequest{
		Name: "招商储蓄卡", Type: "cash", Balance: 15000,
	})
	if err != nil {
		t.Fatalf("update asset failed: %v", err)
	}

	updated, _ := svc.GetByID(ctx, 1, asset.ID)
	if updated.Balance != 15000 {
		t.Errorf("expected balance 15000, got %f", updated.Balance)
	}
	if updated.Name != "招商储蓄卡" {
		t.Errorf("expected name '招商储蓄卡', got %q", updated.Name)
	}
}
```

- [ ] **8.6** 运行测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/service/ -v -run TestAsset
```

预期输出：所有测试通过。

- [ ] **8.7** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add Asset CRUD with multi-tenant isolation"
```

---

## Task 9: 路由注册 + 集成测试

### 目标
统一注册所有路由，编写集成测试验证完整流程（注册 -> 登录 -> CRUD）和多租户隔离。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `internal/router/router.go` |
| Create | `internal/router/router_test.go` |
| Modify | `cmd/server/main.go` |

### Steps

- [ ] **9.1** 创建 `internal/router/router.go`

```go
// internal/router/router.go
package router

import (
	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/handler"
	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/service"
)

// Setup 注册所有路由
func Setup(
	r *gin.Engine,
	authSvc *service.AuthService,
	billSvc *service.BillService,
	budgetSvc *service.BudgetService,
	assetSvc *service.AssetService,
) {
	// 创建 Handler
	authHandler := handler.NewAuthHandler(authSvc)
	billHandler := handler.NewBillHandler(billSvc)
	budgetHandler := handler.NewBudgetHandler(budgetSvc)
	assetHandler := handler.NewAssetHandler(assetSvc)

	// API 路由组
	api := r.Group("/api")

	// 公开路由（无需认证）
	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
	}

	// 需要认证的路由
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(authSvc))
	{
		// 账单路由
		bills := protected.Group("/bills")
		{
			bills.GET("", billHandler.List)
			bills.POST("", billHandler.Create)
			bills.PUT("/:id", billHandler.Update)
			bills.DELETE("/:id", billHandler.Delete)
		}

		// 预算路由
		budgets := protected.Group("/budgets")
		{
			budgets.GET("", budgetHandler.List)
			budgets.POST("", budgetHandler.Create)
			budgets.PUT("/:id", budgetHandler.Update)
		}

		// 资产路由
		assets := protected.Group("/assets")
		{
			assets.GET("", assetHandler.List)
			assets.POST("", assetHandler.Create)
			assets.PUT("/:id", assetHandler.Update)
		}
	}
}
```

- [ ] **9.2** 更新 `cmd/server/main.go`（完整版）

```go
// cmd/server/main.go
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

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/router"
	"github.com/sangchenglong/kapi/internal/service"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.GinMode)

	// 连接 MySQL
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect mysql: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移
	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("failed to auto migrate: %v", err)
	}

	// 连接 Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}
	_ = rdb // P2 阶段使用

	// 初始化 DAO
	userDAO := dao.NewUserDAO(db)
	billDAO := dao.NewBillDAO(db)
	budgetDAO := dao.NewBudgetDAO(db)
	assetDAO := dao.NewAssetDAO(db)

	// 初始化 Service
	authSvc := service.NewAuthService(userDAO, cfg.JWT)
	billSvc := service.NewBillService(billDAO)
	budgetSvc := service.NewBudgetService(budgetDAO)
	assetSvc := service.NewAssetService(assetDAO)

	// 初始化路由
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 注册业务路由
	router.Setup(r, authSvc, billSvc, budgetSvc, assetSvc)

	// 启动 HTTP 服务
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

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	sqlDB.Close()
	rdb.Close()
	log.Println("server exited")
}
```

- [ ] **9.3** 创建 `internal/router/router_test.go`（集成测试）

```go
// internal/router/router_test.go
package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/config"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/pkg/response"
	"github.com/sangchenglong/kapi/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type testEnv struct {
	router *gin.Engine
}

func setupTestEnv(t *testing.T) *testEnv {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	model.AutoMigrate(db)

	userDAO := dao.NewUserDAO(db)
	billDAO := dao.NewBillDAO(db)
	budgetDAO := dao.NewBudgetDAO(db)
	assetDAO := dao.NewAssetDAO(db)

	jwtCfg := config.JWTConfig{Secret: "integration-test-secret", ExpireHours: 24}
	authSvc := service.NewAuthService(userDAO, jwtCfg)
	billSvc := service.NewBillService(billDAO)
	budgetSvc := service.NewBudgetService(budgetDAO)
	assetSvc := service.NewAssetService(assetDAO)

	r := gin.New()
	r.Use(gin.Recovery())
	Setup(r, authSvc, billSvc, budgetSvc, assetSvc)

	return &testEnv{router: r}
}

func (e *testEnv) request(method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	e.router.ServeHTTP(w, req)
	return w
}

func (e *testEnv) registerAndLogin(t *testing.T, username, password string) string {
	// 注册
	e.request("POST", "/api/auth/register", map[string]string{
		"username": username,
		"password": password,
	}, "")

	// 登录
	w := e.request("POST", "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, "")

	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp.Data.(map[string]interface{})
	return data["token"].(string)
}

// 测试完整流程：注册 -> 登录 -> 创建账单 -> 查询账单
func TestIntegration_FullFlow(t *testing.T) {
	env := setupTestEnv(t)

	// 注册并登录
	token := env.registerAndLogin(t, "alice", "password123")

	// 创建账单
	w := env.request("POST", "/api/bills", map[string]interface{}{
		"amount":   38.5,
		"category": "餐饮",
		"merchant": "星巴克",
		"date":     "2026-04-28",
		"note":     "拿铁",
	}, token)

	if w.Code != http.StatusOK {
		t.Fatalf("create bill failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 查询账单列表
	w = env.request("GET", "/api/bills", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("list bills failed: %d", w.Code)
	}

	var listResp response.Response
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp.Data.(map[string]interface{})
	total := data["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 bill, got %v", total)
	}
}

// 测试多租户隔离：用户 A 看不到用户 B 的数据
func TestIntegration_MultiTenantIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// 用户 A 注册登录
	tokenA := env.registerAndLogin(t, "alice", "password123")
	// 用户 B 注册登录
	tokenB := env.registerAndLogin(t, "bob", "password456")

	// 用户 A 创建账单
	env.request("POST", "/api/bills", map[string]interface{}{
		"amount": 100, "category": "餐饮", "date": "2026-04-28",
	}, tokenA)

	// 用户 B 创建账单
	env.request("POST", "/api/bills", map[string]interface{}{
		"amount": 200, "category": "交通", "date": "2026-04-28",
	}, tokenB)

	// 用户 A 查询只能看到自己的账单
	w := env.request("GET", "/api/bills", nil, tokenA)
	var respA response.Response
	json.Unmarshal(w.Body.Bytes(), &respA)
	dataA := respA.Data.(map[string]interface{})
	listA := dataA["list"].([]interface{})
	if len(listA) != 1 {
		t.Errorf("user A expected 1 bill, got %d", len(listA))
	}
	firstBill := listA[0].(map[string]interface{})
	if firstBill["amount"].(float64) != 100 {
		t.Errorf("user A expected amount 100, got %v", firstBill["amount"])
	}

	// 用户 B 查询只能看到自己的账单
	w = env.request("GET", "/api/bills", nil, tokenB)
	var respB response.Response
	json.Unmarshal(w.Body.Bytes(), &respB)
	dataB := respB.Data.(map[string]interface{})
	listB := dataB["list"].([]interface{})
	if len(listB) != 1 {
		t.Errorf("user B expected 1 bill, got %d", len(listB))
	}
	firstBillB := listB[0].(map[string]interface{})
	if firstBillB["amount"].(float64) != 200 {
		t.Errorf("user B expected amount 200, got %v", firstBillB["amount"])
	}
}

// 测试未认证访问被拒绝
func TestIntegration_UnauthorizedAccess(t *testing.T) {
	env := setupTestEnv(t)

	// 无 token 访问
	w := env.request("GET", "/api/bills", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// 无效 token 访问
	w = env.request("GET", "/api/bills", nil, "invalid-token")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// 测试预算 CRUD
func TestIntegration_BudgetCRUD(t *testing.T) {
	env := setupTestEnv(t)
	token := env.registerAndLogin(t, "alice", "password123")

	// 创建预算
	w := env.request("POST", "/api/budgets", map[string]interface{}{
		"category": "餐饮",
		"amount":   3000,
		"period":   "monthly",
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("create budget failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 查询预算列表
	w = env.request("GET", "/api/budgets", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("list budgets failed: %d", w.Code)
	}

	var listResp response.Response
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp.Data.(map[string]interface{})
	list := data["list"].([]interface{})
	if len(list) != 1 {
		t.Errorf("expected 1 budget, got %d", len(list))
	}
}

// 测试资产 CRUD
func TestIntegration_AssetCRUD(t *testing.T) {
	env := setupTestEnv(t)
	token := env.registerAndLogin(t, "alice", "password123")

	// 创建资产
	w := env.request("POST", "/api/assets", map[string]interface{}{
		"name":    "招商银行储蓄卡",
		"type":    "cash",
		"balance": 12000,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("create asset failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 查询资产列表
	w = env.request("GET", "/api/assets", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("list assets failed: %d", w.Code)
	}

	var listResp response.Response
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp.Data.(map[string]interface{})
	list := data["list"].([]interface{})
	if len(list) != 1 {
		t.Errorf("expected 1 asset, got %d", len(list))
	}
}

// 测试软删除
func TestIntegration_BillSoftDelete(t *testing.T) {
	env := setupTestEnv(t)
	token := env.registerAndLogin(t, "alice", "password123")

	// 创建账单
	w := env.request("POST", "/api/bills", map[string]interface{}{
		"amount": 50, "category": "娱乐", "date": "2026-04-28",
	}, token)

	var createResp response.Response
	json.Unmarshal(w.Body.Bytes(), &createResp)
	billData := createResp.Data.(map[string]interface{})
	billID := int(billData["id"].(float64))

	// 删除账单
	w = env.request("DELETE", "/api/bills/"+itoa(billID), nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("delete bill failed: %d", w.Code)
	}

	// 查询列表应该为空
	w = env.request("GET", "/api/bills", nil, token)
	var listResp response.Response
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp.Data.(map[string]interface{})
	total := data["total"].(float64)
	if total != 0 {
		t.Errorf("expected 0 bills after delete, got %v", total)
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
```

注意：测试文件需要在顶部 import 中添加 `"fmt"`。

- [ ] **9.4** 运行集成测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./internal/router/ -v
```

预期输出：
```
=== RUN   TestIntegration_FullFlow
--- PASS: TestIntegration_FullFlow
=== RUN   TestIntegration_MultiTenantIsolation
--- PASS: TestIntegration_MultiTenantIsolation
=== RUN   TestIntegration_UnauthorizedAccess
--- PASS: TestIntegration_UnauthorizedAccess
=== RUN   TestIntegration_BudgetCRUD
--- PASS: TestIntegration_BudgetCRUD
=== RUN   TestIntegration_AssetCRUD
--- PASS: TestIntegration_AssetCRUD
=== RUN   TestIntegration_BillSoftDelete
--- PASS: TestIntegration_BillSoftDelete
PASS
```

- [ ] **9.5** 运行全量测试

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./... -v
```

- [ ] **9.6** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add router setup and integration tests (multi-tenant isolation verified)"
```

---

## Task 10: Mock 数据 + .claude/CLAUDE.md

### 目标
创建数据库初始化 SQL 脚本和项目级 Agent 指令文件。

### Files

| 操作 | 文件路径 |
|------|----------|
| Create | `scripts/init.sql` |
| Create | `.claude/CLAUDE.md` |

### Steps

- [ ] **10.1** 创建 `scripts/init.sql`

```sql
-- scripts/init.sql
-- 咔皮记账 AI 助手 - 数据库初始化脚本
-- 注意：GORM AutoMigrate 会自动建表，此脚本用于 Docker 首次启动时的额外初始化

-- 确保使用 utf8mb4
SET NAMES utf8mb4;
SET CHARACTER SET utf8mb4;

-- 插入测试用户（密码: password123，bcrypt hash）
-- 注意：此 hash 对应密码 "password123"
INSERT INTO users (username, password_hash, created_at, updated_at) VALUES
('demo_user', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', NOW(), NOW()),
('test_user', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', NOW(), NOW())
ON DUPLICATE KEY UPDATE username=username;

-- 插入 demo_user 的测试账单数据（最近 3 个月）
INSERT INTO bills (user_id, amount, category, merchant, date, note, is_deleted, created_at, updated_at) VALUES
-- 2026年4月
(1, 38.00, '餐饮', '星巴克', '2026-04-28', '拿铁', 0, NOW(), NOW()),
(1, 25.50, '餐饮', '麦当劳', '2026-04-27', '午餐', 0, NOW(), NOW()),
(1, 15.00, '交通', '滴滴出行', '2026-04-27', '打车回家', 0, NOW(), NOW()),
(1, 89.00, '餐饮', '海底捞', '2026-04-26', '聚餐', 0, NOW(), NOW()),
(1, 299.00, '购物', '淘宝', '2026-04-25', '衣服', 0, NOW(), NOW()),
(1, 35.00, '餐饮', '外卖', '2026-04-25', '晚餐外卖', 0, NOW(), NOW()),
(1, 6.00, '交通', '地铁', '2026-04-24', '通勤', 0, NOW(), NOW()),
(1, 42.00, '餐饮', '外卖', '2026-04-23', '午餐外卖', 0, NOW(), NOW()),
(1, 128.00, '娱乐', '电影院', '2026-04-22', '看电影', 0, NOW(), NOW()),
(1, 3500.00, '居住', '房东', '2026-04-01', '月租', 0, NOW(), NOW()),
-- 2026年3月
(1, 3500.00, '居住', '房东', '2026-03-01', '月租', 0, NOW(), NOW()),
(1, 200.00, '餐饮', '外卖', '2026-03-15', '一周外卖', 0, NOW(), NOW()),
(1, 500.00, '购物', '京东', '2026-03-10', '耳机', 0, NOW(), NOW()),
(1, 88.00, '娱乐', '网易云音乐', '2026-03-05', '年费会员', 0, NOW(), NOW()),
(1, 150.00, '交通', '加油站', '2026-03-08', '加油', 0, NOW(), NOW()),
-- 2026年2月
(1, 3500.00, '居住', '房东', '2026-02-01', '月租', 0, NOW(), NOW()),
(1, 1200.00, '餐饮', '各类餐厅', '2026-02-14', '情人节晚餐', 0, NOW(), NOW()),
(1, 66.00, '交通', '滴滴出行', '2026-02-20', '打车', 0, NOW(), NOW()),
(1, 399.00, '购物', '优衣库', '2026-02-18', '春装', 0, NOW(), NOW());

-- 插入 demo_user 的预算数据
INSERT INTO budgets (user_id, category, amount, period, created_at, updated_at) VALUES
(1, '餐饮', 3000.00, 'monthly', NOW(), NOW()),
(1, '交通', 800.00, 'monthly', NOW(), NOW()),
(1, '购物', 2000.00, 'monthly', NOW(), NOW()),
(1, '娱乐', 500.00, 'monthly', NOW(), NOW()),
(1, '居住', 4000.00, 'monthly', NOW(), NOW());

-- 插入 demo_user 的资产数据
INSERT INTO assets (user_id, name, type, balance, created_at, updated_at) VALUES
(1, '招商银行储蓄卡', 'cash', 12000.00, NOW(), NOW()),
(1, '支付宝余额', 'cash', 3500.00, NOW(), NOW()),
(1, '招商信用卡', 'credit', -5200.00, NOW(), NOW()),
(1, '基金定投', 'investment', 30000.00, NOW(), NOW()),
(1, '花呗', 'debt', -2800.00, NOW(), NOW());
```

- [ ] **10.2** 创建 `.claude/CLAUDE.md`

```markdown
# KAPI - 咔皮记账 AI 助手

## 项目概述
咔皮记账 AI 助手后端服务，基于 Go + Gin + GORM 构建。

## 技术栈
- Go 1.23+
- Gin (HTTP 框架)
- GORM (ORM)
- MySQL 8.0 (主存储)
- Redis 7 (缓存/会话)
- Milvus (向量数据库，P2 阶段)
- golang-jwt/jwt/v5 (JWT 认证)
- Docker Compose (编排)

## 项目结构
```
cmd/server/main.go          # 服务入口
internal/config/            # 配置加载
internal/model/             # 数据模型（GORM）
internal/dao/               # 数据访问层
internal/service/           # 业务逻辑层
internal/handler/           # HTTP 处理器
internal/middleware/        # 中间件（JWT Auth）
internal/router/            # 路由注册
internal/pkg/response/      # 统一响应格式
internal/pkg/errcode/       # 错误码定义
```

## 编码规范

### 分层架构
- Router -> Handler -> Service -> DAO -> Model
- Handler 只做参数绑定和响应，不含业务逻辑
- Service 包含业务逻辑，调用 DAO
- DAO 只做数据库操作，第一个参数是 context.Context

### 多租户隔离
- 所有数据查询必须带 `WHERE user_id = ?`
- user_id 从 JWT 中间件注入 context，通过 `middleware.GetUserID(c)` 获取
- DAO 层方法必须接收 userID 参数，不允许跨用户查询

### 响应格式
```json
{"code": 0, "message": "success", "data": {...}}
{"code": 40001, "message": "invalid token", "data": null}
```

### 安全规则
- 排序字段必须使用白名单校验，禁止直接拼接用户输入
- 密码使用 bcrypt 哈希，不存储明文
- JWT Secret 从环境变量加载，不硬编码

## 常用命令

```bash
# 启动依赖服务
docker-compose up -d mysql redis milvus

# 运行服务
go run cmd/server/main.go

# 运行测试
go test ./... -v

# 构建
go build -o bin/server ./cmd/server
```

## 环境变量
参考 `.env.example`

## Git 规范
- feat: 新功能
- fix: 修复
- refactor: 重构
- test: 测试
- docs: 文档
```

- [ ] **10.3** 验证全量测试通过

```bash
cd /Users/sangchenglong/go/src/kapi
go test ./... -v -count=1
```

- [ ] **10.4** 验证构建

```bash
cd /Users/sangchenglong/go/src/kapi
go build -o bin/server ./cmd/server
```

- [ ] **10.5** Commit

```bash
cd /Users/sangchenglong/go/src/kapi
git add .
git commit -m "feat: add init.sql mock data and .claude/CLAUDE.md project instructions"
```

---

## 验收标准

完成所有 Task 后，P1 阶段应满足以下验收条件：

| 验收项 | 验证方式 |
|--------|----------|
| Go 服务可编译 | `go build ./cmd/server` 无报错 |
| 全量单元测试通过 | `go test ./... -v` 全部 PASS |
| Docker Compose 可启动 | `docker-compose up -d` 所有服务 healthy |
| 注册/登录 API 可用 | POST /api/auth/register, POST /api/auth/login 返回正确响应 |
| JWT 认证生效 | 无 token 访问 /api/bills 返回 401 |
| 账单 CRUD 可用 | GET/POST/PUT/DELETE /api/bills 正常工作 |
| 预算 CRUD 可用 | GET/POST/PUT /api/budgets 正常工作 |
| 资产 CRUD 可用 | GET/POST/PUT /api/assets 正常工作 |
| 多租户隔离 | 用户 A 无法看到用户 B 的数据（集成测试验证） |
| 软删除生效 | DELETE 后数据不在列表中出现（集成测试验证） |

---

## 依赖清单

```
github.com/gin-gonic/gin
gorm.io/gorm
gorm.io/driver/mysql
gorm.io/driver/sqlite          # 仅测试
github.com/redis/go-redis/v9
github.com/golang-jwt/jwt/v5
golang.org/x/crypto
```






