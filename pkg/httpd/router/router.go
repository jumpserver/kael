package router

import (
	"github.com/gin-gonic/gin"
	"github.com/jumpserver/kael/pkg/httpd/middlewares"
)

func CreateRouter() *gin.Engine {
	eng := gin.Default()
	eng.Use(middlewares.CORSMiddleware())
	karlGroup := eng.Group("/kael")
	karlGroup.Static("/static/", "ui/dist")
	karlGroup.Static("/assets/", "ui/dist/assets")
	karlGroup.GET("/connect", ConnectApi.ConnectHandler)
	karlGroup.GET("/health/", HealthApi.HealthStatusHandler)
	karlGroup.Use(middlewares.AuthMiddleware()).GET("/chat/", ChatApi.ChatHandler)
	karlGroup.Use(middlewares.AuthMiddleware()).GET("/chat/system/", SystemChatApi.ChatHandler)
	karlGroup.Use(middlewares.AuthMiddleware()).POST("/jms_state/", HandlerApi.JmsStateHandler)
	karlGroup.Use(middlewares.AuthMiddleware()).POST("/interrupt_current_ask/", HandlerApi.InterruptCurrentAskHandler)
	return eng
}
