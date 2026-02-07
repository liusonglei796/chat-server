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
		authGroup.POST("/login", loginLimiter, rt.handlers.User.Login)        // 密码登录
		authGroup.POST("/sms-login", loginLimiter, rt.handlers.User.SmsLogin) // 短信验证码登录

		// 短信验证码：同一手机号 60秒内最多 1 次
		smsLimiter := middleware.RateLimit(rt.cache, "rate:sms:", middleware.ByFormPhone, 1, 60*time.Second)
		authGroup.POST("/sms-code", smsLimiter, rt.handlers.User.SendSmsCode) // 发送短信验证码

		authGroup.POST("/register", rt.handlers.User.Register)    // 用户注册
		authGroup.POST("/refresh", rt.handlers.Auth.RefreshToken) // 刷新 Access Token
	}
}
