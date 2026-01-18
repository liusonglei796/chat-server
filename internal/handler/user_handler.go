// Package handler 提供 HTTP 请求处理器
// 本文件处理用户相关的 API 请求
package handler

import (
	"fmt"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户请求处理器
// 通过构造函数注入 UserService，遵循依赖倒置原则
type UserHandler struct {
	userSvc service.UserService
}

// NewUserHandler 创建用户处理器实例
// userSvc: 用户服务接口
func NewUserHandler(userSvc service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// Register 用户注册
// POST /user/register
// 请求体: request.RegisterRequest
// 响应: respond.RegisterRespond (用户信息)
func (h *UserHandler) Register(c *gin.Context) {
	// 1. 绑定并验证请求参数
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	fmt.Println(req) // 调试输出，生产环境可删除

	// 2. 调用 Service 层处理业务逻辑
	data, err := h.userSvc.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}

	// 3. 返回成功响应
	HandleSuccess(c, data)
}

// Login 用户登录（密码登录）
// POST /user/login
// 请求体: request.LoginRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
func (h *UserHandler) Login(c *gin.Context) {
	var req request.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.Login(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// SmsLogin 短信验证码登录
// POST /user/smsLogin
// 请求体: request.SmsLoginRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
func (h *UserHandler) SmsLogin(c *gin.Context) {
	var req request.SmsLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.SmsLogin(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// UpdateUserInfo 修改用户信息
// POST /user/updateUserInfo
// 请求体: request.UpdateUserInfoRequest
// 响应: nil (无返回数据)
// 安全: 从JWT上下文获取当前用户ID，只能修改自己的信息
func (h *UserHandler) UpdateUserInfo(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.UpdateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.userSvc.UpdateUserInfo(userId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetUserListPaged 分页获取用户列表（管理员功能）
// GET /admin/user/list?page=1&pageSize=10&keyword=xxx&status=0
// 查询参数: request.GetUserListPagedRequest
// 响应: respond.PagedUserListRespond
func (h *UserHandler) GetUserListPaged(c *gin.Context) {
	var req request.GetUserListPagedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.GetUserListPaged(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// BatchUpdateUserStatus 批量更新用户状态（管理员功能）
// POST /admin/user/batchStatus
// 请求体: request.BatchUpdateUserStatusRequest
// Action: enable(启用), disable(禁用), delete(删除)
// 响应: nil
func (h *UserHandler) BatchUpdateUserStatus(c *gin.Context) {
	var req request.BatchUpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.userSvc.BatchUpdateUserStatus(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetUserInfo 获取当前用户完整信息（仅限查自己）
// GET /user/getUserInfo
// 安全: 从JWT上下文获取当前用户ID
// 响应: respond.GetUserInfoRespond
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := h.userSvc.GetUserInfo(userId.(string), userId.(string))
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetPublicUserInfo 获取他人公开信息
// GET /user/getPublicUserInfo?uuid=xxx
// 查询参数: request.GetUserInfoRequest
// 响应: respond.PublicUserInfoRespond
func (h *UserHandler) GetPublicUserInfo(c *gin.Context) {
	var req request.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.GetPublicUserInfo(req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// SetAdmin 设置管理员权限（管理员功能）
// POST /user/setAdmin
// 请求体: request.AbleUsersRequest (含 IsAdmin 字段)
// 响应: nil
func (h *UserHandler) SetAdmin(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.userSvc.SetAdmin(req.UuidList, req.IsAdmin); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SendSmsCode 发送短信验证码
// POST /user/sendSmsCode
// 请求体: request.SendSmsCodeRequest
// 响应: nil
func (h *UserHandler) SendSmsCode(c *gin.Context) {
	var req request.SendSmsCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.userSvc.SendSmsCode(req.Telephone); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
