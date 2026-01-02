package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine) {
	RegisterAuthRoutes(r)
	RegisterUserRoutes(r)
	RegisterGroupRoutes(r)
	RegisterContactRoutes(r)
	RegisterSessionRoutes(r)
	RegisterMessageRoutes(r)
	RegisterWebSocketRoutes(r)
	RegisterChatRoomRoutes(r)
}
