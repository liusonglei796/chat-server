// Package handler 提供 HTTP 请求处理器
// 本文件处理用户相关的 API 请求
package handler

import (
	authpb "kama_chat_server/api/gen/auth"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/dto/request/auth"
	"kama_chat_server/internal/common/dto/request/user"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户请求处理器
type UserHandler struct {
}

// NewUserHandler 创建用户处理器实例
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// Login 用户登录
func (h *UserHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()

	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	rsp, err := grpc_client.AuthClient.Login(ctx, &authpb.LoginRequest{
		Username: req.Username,
		Password: req.Password,
		ClientIp: c.ClientIP(),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	ctx := c.Request.Context()

	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	rsp, err := grpc_client.AuthClient.Register(ctx, &authpb.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		ClientIp: c.ClientIP(),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// Logout 用户登出
func (h *UserHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	_, err := grpc_client.AuthClient.Logout(ctx, &authpb.LogoutRequest{
		UserId: userId.(string),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// KickUser 管理员踢人下线
func (h *UserHandler) KickUser(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		UserId string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	_, err := grpc_client.UserClient.KickUser(ctx, &userpb.KickUserRequest{
		UserId: req.UserId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateUserInfo 修改用户信息
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

	_, err := grpc_client.UserClient.UpdateUserInfo(ctx, &userpb.UpdateUserInfoRequest{
		UserId:    userId.(string),
		Email:     req.Email,
		Nickname:  req.Nickname,
		Birthday:  req.Birthday,
		Signature: req.Signature,
		Avatar:    req.Avatar,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetUserInfo 获取当前用户完整信息
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := grpc_client.UserClient.GetUserInfo(ctx, &userpb.GetUserInfoRequest{
		RequesterId: userId.(string),
		TargetId:    userId.(string),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetPublicUserInfo 获取他人公开信息
func (h *UserHandler) GetPublicUserInfo(c *gin.Context) {
	ctx := c.Request.Context()

	var req user.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := grpc_client.UserClient.GetPublicUserInfo(ctx, &userpb.GetPublicUserInfoRequest{
		TargetId: req.Uuid,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
