package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetChatRouter(router *gin.Engine) {
	chatRouter := router.Group("/")
	chatRouter.Use(middleware.CORS())
	chatRouter.Use(middleware.RouteTag("chat"))
	{
		chatRouter.GET("/ws/receive", controller.ChatReceive)
	}

	apiChatRouter := router.Group("/api/chat")
	apiChatRouter.Use(middleware.CORS())
	apiChatRouter.Use(middleware.RouteTag("chat"))
	apiChatRouter.Use(middleware.BodyStorageCleanup())
	apiChatRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiChatRouter.POST("/send", controller.ChatSend)
		apiChatRouter.GET("/heartbeat", controller.ChatHeartbeat)
		apiChatRouter.POST("/heartbeat", controller.ChatHeartbeat)
	}
}
