// Package handler 提供 HTTP 请求处理器
// 本文件处理认证相关的 API 请求
package handler

import (
	authpb "kama_chat_server/api/gen/auth"
	"kama_chat_server/internal/common/dto/request/auth"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/jwt"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// AuthHandler 认证请求处理器
type AuthHandler struct {
}

// NewAuthHandler 创建认证处理器实例
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// RefreshToken 刷新 Access Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	ctx := c.Request.Context()

	var req auth.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	// 1. 解析 Refresh Token
	claims, err := jwt.ParseToken(req.RefreshToken)
	if err != nil {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "Refresh Token 已过期或无效，请重新登录"))
		return
	}

	if claims.Subject != "refresh_token" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请使用 Refresh Token"))
		return
	}

	rsp, err := grpc_client.AuthClient.ValidateTokenID(ctx, &authpb.ValidateTokenIDRequest{
		UserId:  claims.UserID,
		TokenId: claims.TokenID,
	})
	if err != nil {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "登录状态已失效，请重新登录"))
		return
	}

	if !rsp.IsValid {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "您的账号已在其他设备登录，请重新登录"))
		return
	}

	adminRsp, err := grpc_client.AuthClient.GetUserIsAdmin(ctx, &authpb.GetUserIsAdminRequest{
		UserId: claims.UserID,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	newAccessToken, err := jwt.GenerateAccessToken(claims.UserID, adminRsp.IsAdmin)
	if err != nil {
		HandleError(c, errorx.ErrServerBusy)
		return
	}

	HandleSuccess(c, gin.H{
		"access_token": newAccessToken,
	})
}
