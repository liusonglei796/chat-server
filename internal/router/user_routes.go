// Package router 提供 HTTP 路由注册
// 本文件定义用户相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterPublicUserRoutes 注册用户公开路由（无需认证）
// 用于登录、注册等
func RegisterPublicUserRoutes(rg *gin.RouterGroup) {
	rg.POST("/login", handler.LoginHandler)                  // 密码登录
	rg.POST("/register", handler.RegisterHandler)            // 用户注册
	rg.POST("/user/smsLogin", handler.SmsLoginHandler)       // 短信验证码登录
	rg.POST("/user/sendSmsCode", handler.SendSmsCodeHandler) // 发送短信验证码
}

// RegisterUserRoutes 注册用户相关路由（需要认证）
// 包括用户信息管理、管理员功能等
func RegisterUserRoutes(rg *gin.RouterGroup) {
	userGroup := rg.Group("/user")
	{
		userGroup.POST("/wsLogout", handler.WsLogoutHandler)             // WebSocket 登出
		userGroup.POST("/updateUserInfo", handler.UpdateUserInfoHandler) // 更新用户信息
		userGroup.GET("/getUserInfo", handler.GetUserInfoHandler)        // 获取用户详情

	}
}
