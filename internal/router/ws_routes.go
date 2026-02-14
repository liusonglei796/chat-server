// Package router 提供 HTTP 路由注册
// 本文件定义 WebSocket 和聊天室相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterWebSocketRoutes 注册 WebSocket 相关路由（需要认证）
func (rt *Router) RegisterWebSocketRoutes(rg *gin.RouterGroup) {
	wsGroup := rg.Group("/ws")
	{
		// WebSocket 连接入口
		// 客户端通过此路由建立 WebSocket 连接
		// 请求示例: ws://host:port/ws?client_id=U123456789
		wsGroup.GET("", rt.handlers.Ws.WsLoginHandler)

		// WebSocket 登出
		wsGroup.POST("/logout", rt.handlers.Ws.WsLogoutHandler)
	}
}
