// Package router 提供 HTTP 路由注册
// 本文件定义认证相关的路由
package router

import (
	"time"

	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由（公开）
// 统一到 /auth 前缀下，对高频接口施加限流保护
func (rt *Router) RegisterAuthRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		// 登录路由：同一 IP 5分钟内最多 10 次
		loginLimiter := middleware.RateLimit(rt.cache, "rate:login:", middleware.ByClientIP, 10, 5*time.Minute)
		authGroup.POST("/login", loginLimiter, rt.handlers.User.Login) // 密码登录

		// 注册路由：同一 IP 5分钟内最多 5 次
		registerLimiter := middleware.RateLimit(rt.cache, "rate:register:", middleware.ByClientIP, 5, 5*time.Minute)
		authGroup.POST("/register", registerLimiter, rt.handlers.User.Register) // 用户注册

		authGroup.POST("/refresh", rt.handlers.Auth.RefreshToken) // 刷新 Access Token
	}
}

// RegisterAuthPrivateRoutes 注册认证相关的私有路由（需要 JWT 认证）
func (rt *Router) RegisterAuthPrivateRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		authGroup.POST("/logout", rt.handlers.User.Logout) // 用户登出
		authGroup.POST("/kick", rt.handlers.User.KickUser) // 管理员踢人
	}
}
