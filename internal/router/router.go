// Package router 提供 HTTP 路由注册
// 本文件是路由注册的入口，聚合所有子模块的路由
package router

import (
	"kama_chat_server/internal/dao/mysql"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// Router 路由管理器
// 封装所有路由注册逻辑，通过依赖注入接收 handlers 和 userRepo
type Router struct {
	handlers *handler.Handlers
	userRepo mysql.UserRepository // 用于管理员权限校验
}

// NewRouter 创建路由管理器
// handlers: 通过依赖注入传入的 handler 聚合对象
// userRepo: 用户数据访问接口，用于管理员权限校验
func NewRouter(handlers *handler.Handlers, userRepo mysql.UserRepository) *Router {
	return &Router{
		handlers: handlers,
		userRepo: userRepo,
	}
}

// RegisterRoutes 注册所有路由
// 在 https_server.Init() 中调用
// 路由分为两组:
//   - 公开路由: 无需认证，用于登录、注册、Token刷新
//   - 私有路由: 需要 JWT 认证
func (rt *Router) RegisterRoutes(r *gin.Engine) {
	// ==================== 公开路由 (无需认证) ====================
	public := r.Group("")
	{
		rt.RegisterAuthRoutes(public)       // 认证路由（Token 刷新）
		rt.RegisterPublicUserRoutes(public) // 用户公开路由（登录、注册）
	}

	// ==================== 私有路由 (需要认证) ====================
	private := r.Group("")
	private.Use(middleware.JWTAuth())
	{
		rt.RegisterAdminRoutes(private)     // 管理员路由（额外校验管理员权限）
		rt.RegisterUserRoutes(private)      // 用户路由
		rt.RegisterFriendRoutes(private)    // 好友路由
		rt.RegisterGroupRoutes(private)     // 群组路由
		rt.RegisterSessionRoutes(private)   // 会话路由
		rt.RegisterMessageRoutes(private)   // 消息路由
		rt.RegisterWebSocketRoutes(private) // WebSocket 路由
	}
}
