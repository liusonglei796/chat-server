// Package router 提供 HTTP 路由注册
// 本文件定义认证相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由
// 用于 JWT Token 管理
func RegisterAuthRoutes(r *gin.Engine) {
	authGroup := r.Group("/auth")
	{
		// POST /auth/refresh - 刷新 Access Token
		// 使用 Refresh Token 换取新的 Access Token
		authGroup.POST("/refresh", handler.RefreshTokenHandler)
	}
}
