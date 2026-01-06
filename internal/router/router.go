// Package router 提供 HTTP 路由注册
// 本文件是路由注册的入口，聚合所有子模块的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
// 在 https_server.Init() 中调用
// 按模块分别注册各个路由组
func RegisterRoutes(r *gin.Engine) {
	RegisterAuthRoutes(r)      // 认证路由（Token 刷新）
	RegisterUserRoutes(r)      // 用户路由
	RegisterGroupRoutes(r)     // 群组路由
	RegisterContactRoutes(r)   // 联系人路由
	RegisterSessionRoutes(r)   // 会话路由
	RegisterMessageRoutes(r)   // 消息路由
	RegisterWebSocketRoutes(r) // WebSocket 路由
	RegisterChatRoomRoutes(r)  // 聊天室路由
}
