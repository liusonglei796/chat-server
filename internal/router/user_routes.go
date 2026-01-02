package router

import (
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户相关路由
func RegisterUserRoutes(r *gin.Engine) {
	// 公开接口 (无需认证)
	r.POST("/login", handler.LoginHandler)
	r.POST("/register", handler.RegisterHandler)
	r.POST("/user/smsLogin", handler.SmsLoginHandler)
	r.POST("/user/sendSmsCode", handler.SendSmsCodeHandler)

	// 需要认证的接口
	userGroup := r.Group("/user")
	userGroup.Use(middleware.JWTAuth())
	{
		userGroup.POST("/wsLogout", handler.WsLogoutHandler)
		userGroup.POST("/updateUserInfo", handler.UpdateUserInfoHandler)
		userGroup.GET("/getUserInfoList", handler.GetUserInfoListHandler)
		userGroup.GET("/getUserInfo", handler.GetUserInfoHandler)
		userGroup.POST("/ableUsers", handler.AbleUsersHandler)
		userGroup.POST("/disableUsers", handler.DisableUsersHandler)
		userGroup.POST("/deleteUsers", handler.DeleteUsersHandler)
		userGroup.POST("/setAdmin", handler.SetAdminHandler)
	}
}
