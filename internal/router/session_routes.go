package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterSessionRoutes 注册会话相关路由
func RegisterSessionRoutes(r *gin.Engine) {
	r.POST("/session/openSession", handler.OpenSessionHandler)
	r.GET("/session/getUserSessionList", handler.GetUserSessionListHandler)
	r.GET("/session/getGroupSessionList", handler.GetGroupSessionListHandler)
	r.POST("/session/deleteSession", handler.DeleteSessionHandler)
	r.GET("/session/checkOpenSessionAllowed", handler.CheckOpenSessionAllowedHandler)
}
