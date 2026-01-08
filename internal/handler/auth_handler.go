// Package handler 提供 HTTP 请求处理器
// 本文件处理认证相关的 API 请求
package handler

import (
	"context"

	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/jwt"

	"github.com/gin-gonic/gin"
)

// RefreshTokenHandler 刷新 Access Token
// POST /auth/refresh
// 请求体: request.RefreshTokenRequest
// 响应: { access_token: string }
//
// 功能:
//   - 验证 Refresh Token 是否有效
//   - 验证 Token ID 是否与 Redis 中存储的一致（单点互踢）
//   - 生成新的 Access Token
//
// 单点互踢机制:
//   - 用户登录时会在 Redis 中存储 Token ID
//   - 如果用户在其他设备登录，会覆盖旧的 Token ID
//   - 使用旧 Token ID 刷新时会被拒绝
func RefreshTokenHandler(c *gin.Context) {
	var req request.RefreshTokenRequest
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

	// 2. 验证是否为 Refresh Token（防止使用 Access Token 刷新）
	if claims.Subject != "refresh_token" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请使用 Refresh Token"))
		return
	}

	// 3. 从 Redis 获取最新的 Token ID，实现单点互踢
	redisKey := "user_token:" + claims.UserID
	validTokenID, err := myredis.GetKey(context.Background(), redisKey)
	if err != nil || validTokenID == "" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "登录状态已失效，请重新登录"))
		return
	}

	// 4. 比对 Token ID（如果不一致，说明用户在其他设备登录过）
	if claims.TokenID != validTokenID {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "您的账号已在其他设备登录，请重新登录"))
		return
	}

	// 5. 生成新的 Access Token
	newAccessToken, err := jwt.GenerateAccessToken(claims.UserID)
	if err != nil {
		HandleError(c, errorx.ErrServerBusy)
		return
	}

	HandleSuccess(c, gin.H{
		"access_token": newAccessToken,
	})
}
