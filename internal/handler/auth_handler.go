package handler

import (
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/jwt"

	"github.com/gin-gonic/gin"
)

// RefreshTokenHandler 刷新 Access Token
// 用 Refresh Token 换取新的 Access Token
// 同时验证 Redis 中的 Token ID 实现单点互踢
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

	// 2. 验证是否为 Refresh Token
	if claims.Subject != "refresh_token" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请使用 Refresh Token"))
		return
	}

	// 3. 从 Redis 获取最新的 Token ID，实现单点互踢
	redisKey := "user_token:" + claims.UserID
	validTokenID, err := myredis.GetKey(redisKey)
	if err != nil || validTokenID == "" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "登录状态已失效，请重新登录"))
		return
	}

	// 4. 比对 Token ID
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
