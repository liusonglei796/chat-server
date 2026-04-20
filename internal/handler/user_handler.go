// Package handler 提供 HTTP 请求处理器
// 本文件处理用户相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/auth"
	"kama_chat_server/internal/dto/request/user"
	usersvc "kama_chat_server/internal/service/user"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户请求处理器
// 通过构造函数注入 UserService，遵循依赖倒置原则
type UserHandler struct {
	userSvc *usersvc.UserService
}

// NewUserHandler 创建用户处理器实例
// userSvc: 用户服务
func NewUserHandler(userSvc *usersvc.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// Login 用户登录（密码登录）
// POST /auth/login
// 请求体: auth.LoginRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
// SSO: 登录时会将 token 存入 Redis，实现单点登录
func (h *UserHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()

	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.Login(ctx, req, c.ClientIP())
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// Register 用户注册
// POST /auth/register
// 请求体: auth.RegisterRequest
// 响应: respond.LoginRespond (用户信息 + JWT Token)
func (h *UserHandler) Register(c *gin.Context) {
	ctx := c.Request.Context()

	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.Register(ctx, req, c.ClientIP())
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// Logout 用户登出
// POST /auth/logout
// 功能: 从 Redis 删除用户 token，实现 SSO 登出
func (h *UserHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	if err := h.userSvc.Logout(ctx, userId.(string)); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// KickUser 管理员踢人下线
// POST /auth/kick
// 功能: 管理员强制指定用户下线
func (h *UserHandler) KickUser(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		UserId string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.userSvc.KickUser(ctx, req.UserId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateUserInfo 修改用户信息
// POST /user/updateUserInfo
// 请求体: user.UpdateUserInfoRequest
// 响应: nil (无返回数据)
// 安全: 从JWT上下文获取当前用户ID，只能修改自己的信息
func (h *UserHandler) UpdateUserInfo(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req user.UpdateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.userSvc.UpdateUserInfo(ctx, userId.(string), req); err != nil {
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
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := h.userSvc.GetUserInfo(ctx, userId.(string), userId.(string))
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetPublicUserInfo 获取他人公开信息
// GET /user/getPublicUserInfo?uuid=xxx
// 查询参数: user.GetUserInfoRequest
// 响应: respond.PublicUserInfoRespond
func (h *UserHandler) GetPublicUserInfo(c *gin.Context) {
	ctx := c.Request.Context()

	var req user.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.GetPublicUserInfo(ctx, req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
