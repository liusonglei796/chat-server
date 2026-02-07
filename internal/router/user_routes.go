// Package router 提供 HTTP 路由注册
// 本文件定义用户相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户相关路由（需要认证）
// 包括用户信息管理
func (rt *Router) RegisterUserRoutes(rg *gin.RouterGroup) {
	userGroup := rg.Group("/user")
	{
		userGroup.PUT("/info", rt.handlers.User.UpdateUserInfo)           // 更新用户信息
		userGroup.GET("/info", rt.handlers.User.GetUserInfo)              // 获取当前用户详情
		userGroup.GET("/public-info", rt.handlers.User.GetPublicUserInfo) // 获取他人公开信息
	}
}
