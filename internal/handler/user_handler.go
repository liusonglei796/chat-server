package handler

import (
	"fmt"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// Register 注册
func RegisterHandler(c *gin.Context) {
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	fmt.Println(req)
	data, err := service.Svc.User.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// Login 登录
func LoginHandler(c *gin.Context) {
	var req request.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.Login(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// SmsLogin 验证码登录
func SmsLoginHandler(c *gin.Context) {
	var req request.SmsLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.SmsLogin(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// UpdateUserInfo 修改用户信息
func UpdateUserInfoHandler(c *gin.Context) {
	var req request.UpdateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.UpdateUserInfo(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetUserInfoList 获取用户列表
func GetUserInfoListHandler(c *gin.Context) {
	var req request.GetUserInfoListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.GetUserInfoList(req.OwnerId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// AbleUsers 启用用户
func AbleUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.AbleUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DisableUsers 禁用用户
func DisableUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.DisableUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetUserInfo 获取用户信息
func GetUserInfoHandler(c *gin.Context) {
	var req request.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.GetUserInfo(req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteUsers 删除用户
func DeleteUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.DeleteUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SetAdmin 设置管理员
func SetAdminHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.SetAdmin(req.UuidList, req.IsAdmin); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SendSmsCode 发送短信验证码
func SendSmsCodeHandler(c *gin.Context) {
	var req request.SendSmsCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.SendSmsCode(req.Telephone); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
