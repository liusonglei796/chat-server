// Package router 提供 HTTP 路由注册
// 本文件定义认证相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由（公开）
// 用于 JWT Token 管理
func (rt *Router) RegisterAuthRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		// POST /auth/refresh - 刷新 Access Token
		// 使用 Refresh Token 换取新的 Access Token
		authGroup.POST("/refresh", rt.handlers.Auth.RefreshToken)
	}
}
