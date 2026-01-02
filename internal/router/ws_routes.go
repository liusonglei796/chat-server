package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterWebSocketRoutes 注册 WebSocket 相关路由
func RegisterWebSocketRoutes(r *gin.Engine) {
	r.GET("/wss", handler.WsLoginHandler)
}

// RegisterChatRoomRoutes 注册聊天室相关路由
func RegisterChatRoomRoutes(r *gin.Engine) {
	r.GET("/chatroom/getCurContactListInChatRoom", handler.GetCurContactListInChatRoomHandler)
}
