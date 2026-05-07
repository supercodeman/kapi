package router

import (
	"github.com/gin-gonic/gin"

	"github.com/sangchenglong/kapi/internal/engine"
	"github.com/sangchenglong/kapi/internal/handler"
	"github.com/sangchenglong/kapi/internal/middleware"
	"github.com/sangchenglong/kapi/internal/service"
)

func Setup(r *gin.Engine, authSvc *service.AuthService, authH *handler.AuthHandler, billH *handler.BillHandler, budgetH *handler.BudgetHandler, assetH *handler.AssetHandler, chatH *handler.ChatHandler, skillH *handler.SkillHandler, connMgr *engine.ConnectionManager) {
	api := r.Group("/api")

	auth := api.Group("/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
	}

	protected := api.Group("")
	protected.Use(middleware.Auth(authSvc))
	{
		bills := protected.Group("/bills")
		{
			bills.GET("", billH.List)
			bills.POST("", billH.Create)
			bills.PUT("/:id", billH.Update)
			bills.DELETE("/:id", billH.Delete)
		}

		budgets := protected.Group("/budgets")
		{
			budgets.GET("", budgetH.List)
			budgets.POST("", budgetH.Create)
			budgets.PUT("/:id", budgetH.Update)
		}

		assets := protected.Group("/assets")
		{
			assets.GET("", assetH.List)
			assets.POST("", assetH.Create)
			assets.PUT("/:id", assetH.Update)
		}

		protected.POST("/chat", chatH.Chat)
		protected.POST("/chat/stream", chatH.ChatStream)
		protected.GET("/chat/history", chatH.GetHistory)
	}

	ws := r.Group("/ws")
	ws.Use(middleware.Auth(authSvc))
	{
		ws.GET("/chat", connMgr.HandleWebSocket)
	}

	// Skill 管理 API（运维/业务侧，无需用户认证）
	admin := api.Group("/admin")
	{
		skills := admin.Group("/skills")
		{
			skills.GET("", skillH.List)
			skills.GET("/:name", skillH.Get)
			skills.POST("", skillH.Create)
			skills.PUT("/:name", skillH.Update)
			skills.DELETE("/:name", skillH.Delete)
		}
	}
}
