// Package router 提供 HTTP 路由注册
// 本文件定义 WebSocket 和聊天室相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterWebSocketRoutes 注册 WebSocket 相关路由（需要认证）
func (rt *Router) RegisterWebSocketRoutes(rg *gin.RouterGroup) {
	// WebSocket 连接入口
	// 客户端通过此路由建立 WebSocket 连接
	// 请求示例: ws://host:port/wss?client_id=U123456789
	rg.GET("/wss", rt.handlers.Ws.WsLoginHandler)
}
