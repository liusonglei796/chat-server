// Package router 提供 HTTP 路由注册
// 本文件定义 WebSocket 和聊天室相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterWebSocketRoutes 注册 WebSocket 相关路由
func RegisterWebSocketRoutes(r *gin.Engine) {
	// WebSocket 连接入口
	// 客户端通过此路由建立 WebSocket 连接
	// 请求示例: ws://host:port/wss?client_id=U123456789
	r.GET("/wss", handler.WsLoginHandler)
}

// RegisterChatRoomRoutes 注册聊天室相关路由
func RegisterChatRoomRoutes(r *gin.Engine) {
	// 获取当前聊天室联系人列表
	r.GET("/chatroom/getCurContactListInChatRoom", handler.GetCurContactListInChatRoomHandler)
}
