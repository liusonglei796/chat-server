// Package handler 提供 HTTP 请求处理器
// 本文件处理用户相关的 API 请求
package handler

import (
	"fmt"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterHandler 用户注册
// POST /user/register
// 请求体: request.RegisterRequest
// 响应: respond.RegisterRespond (用户信息)
func RegisterHandler(c *gin.Context) {
	// 1. 绑定并验证请求参数
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	fmt.Println(req) // 调试输出，生产环境可删除

	// 2. 调用 Service 层处理业务逻辑
	data, err := service.Svc.User.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}

	// 3. 返回成功响应
	HandleSuccess(c, data)
}

// LoginHandler 用户登录（密码登录）
// POST /user/login
// 请求体: request.LoginRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
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

// SmsLoginHandler 短信验证码登录
// POST /user/smsLogin
// 请求体: request.SmsLoginRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
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

// UpdateUserInfoHandler 修改用户信息
// POST /user/updateUserInfo
// 请求体: request.UpdateUserInfoRequest
// 响应: nil (无返回数据)
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

// GetUserInfoListHandler 获取用户列表
// GET /user/getUserInfoList?ownerId=xxx
// 查询参数: request.GetUserInfoListRequest
// 响应: []respond.GetUserInfoListRespond
func GetUserInfoListHandler(c *gin.Context) {
	var req request.GetUserInfoListRequest
	// 使用 ShouldBindQuery 绑定 URL 查询参数
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

// AbleUsersHandler 启用用户（管理员功能）
// POST /user/ableUsers
// 请求体: request.AbleUsersRequest
// 响应: nil
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

// DisableUsersHandler 禁用用户（管理员功能）
// POST /user/disableUsers
// 请求体: request.AbleUsersRequest
// 响应: nil
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

// GetUserInfoHandler 获取单个用户信息
// GET /user/getUserInfo?uuid=xxx
// 查询参数: request.GetUserInfoRequest
// 响应: respond.GetUserInfoRespond
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

// DeleteUsersHandler 删除用户（管理员功能）
// POST /user/deleteUsers
// 请求体: request.AbleUsersRequest
// 响应: nil
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

// SetAdminHandler 设置管理员权限（管理员功能）
// POST /user/setAdmin
// 请求体: request.AbleUsersRequest (含 IsAdmin 字段)
// 响应: nil
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

// SendSmsCodeHandler 发送短信验证码
// POST /user/sendSmsCode
// 请求体: request.SendSmsCodeRequest
// 响应: nil
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
