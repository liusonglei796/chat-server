// Package router 提供 HTTP 路由注册
// 本文件定义用户相关的路由
package router

import (
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户相关路由
// 路由分为两组:
//   - 公开接口: 无需认证，用于登录注册
//   - 认证接口: 需要 JWT 认证，用于用户管理
func RegisterUserRoutes(r *gin.Engine) {
	// ==================== 公开接口 (无需认证) ====================
	r.POST("/login", handler.LoginHandler)                  // 密码登录
	r.POST("/register", handler.RegisterHandler)            // 用户注册
	r.POST("/user/smsLogin", handler.SmsLoginHandler)       // 短信验证码登录
	r.POST("/user/sendSmsCode", handler.SendSmsCodeHandler) // 发送短信验证码

	// ==================== 需要认证的接口 ====================
	userGroup := r.Group("/user")
	userGroup.Use(middleware.JWTAuth()) // 应用 JWT 认证中间件
	{
		userGroup.POST("/wsLogout", handler.WsLogoutHandler)              // WebSocket 登出
		userGroup.POST("/updateUserInfo", handler.UpdateUserInfoHandler)  // 更新用户信息
		userGroup.GET("/getUserInfoList", handler.GetUserInfoListHandler) // 获取用户列表
		userGroup.GET("/getUserInfo", handler.GetUserInfoHandler)         // 获取用户详情
		userGroup.POST("/ableUsers", handler.AbleUsersHandler)            // 批量启用用户（管理员）
		userGroup.POST("/disableUsers", handler.DisableUsersHandler)      // 批量禁用用户（管理员）
		userGroup.POST("/deleteUsers", handler.DeleteUsersHandler)        // 批量删除用户（管理员）
		userGroup.POST("/setAdmin", handler.SetAdminHandler)              // 设置管理员（管理员）
	}
}
