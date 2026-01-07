// Package router 提供 HTTP 路由注册
// 本文件是路由注册的入口，聚合所有子模块的路由
package router

import (
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
// 在 https_server.Init() 中调用
// 路由分为两组:
//   - 公开路由: 无需认证，用于登录、注册、Token刷新
//   - 私有路由: 需要 JWT 认证
func RegisterRoutes(r *gin.Engine) {
	// ==================== 公开路由 (无需认证) ====================
	public := r.Group("")
	{
		RegisterAuthRoutes(public)       // 认证路由（Token 刷新）
		RegisterPublicUserRoutes(public) // 用户公开路由（登录、注册）
	}

	// ==================== 私有路由 (需要认证) ====================
	private := r.Group("")
	private.Use(middleware.JWTAuth())
	{
		RegisterAdminRoutes(private)     // 管理员路由
		RegisterUserRoutes(private)      // 用户路由
		RegisterFriendRoutes(private)    // 好友路由
		RegisterGroupRoutes(private)     // 群组路由
		RegisterSessionRoutes(private)   // 会话路由
		RegisterMessageRoutes(private)   // 消息路由
		RegisterWebSocketRoutes(private) // WebSocket 路由
	}
}
