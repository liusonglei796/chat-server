// Package router 提供 HTTP 路由注册
// 本文件定义用户相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterPublicUserRoutes 注册用户公开路由（无需认证）
// 用于登录、注册等
func (rt *Router) RegisterPublicUserRoutes(rg *gin.RouterGroup) {
	rg.POST("/login", rt.handlers.User.Login)                  // 密码登录
	rg.POST("/register", rt.handlers.User.Register)            // 用户注册
	rg.POST("/user/smsLogin", rt.handlers.User.SmsLogin)       // 短信验证码登录
	rg.POST("/user/sendSmsCode", rt.handlers.User.SendSmsCode) // 发送短信验证码
}

// RegisterUserRoutes 注册用户相关路由（需要认证）
// 包括用户信息管理、管理员功能等
func (rt *Router) RegisterUserRoutes(rg *gin.RouterGroup) {
	userGroup := rg.Group("/user")
	{
		userGroup.POST("/wsLogout", handler.WsLogoutHandler)               // WebSocket 登出
		userGroup.POST("/updateUserInfo", rt.handlers.User.UpdateUserInfo) // 更新用户信息
		userGroup.GET("/getUserInfo", rt.handlers.User.GetUserInfo)        // 获取用户详情

	}
}
