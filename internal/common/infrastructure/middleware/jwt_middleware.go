package middleware

import (
	"net/http"
	"strings"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/infrastructure/jwt"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// JWTAuth JWT 认证中间件（不带 Redis 验证，兼容旧版本）
// 验证 Access Token 并将用户信息存入上下文
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请先登录",
			})
			return
		}

		// 2. 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "Token 格式错误，请使用 Bearer Token",
			})
			return
		}

		// 3. 验证 Token
		claims, err := jwt.ParseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "Token 已过期或无效，请重新登录",
			})
			return
		}

		// 4. 验证是否为 Access Token
		if claims.Subject != "access_token" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请使用 Access Token 访问此接口",
			})
			return
		}

		// 5. 将用户信息存入上下文，供后续 Handler 使用
		c.Set("user_id", claims.UserID)
		c.Set("is_admin", claims.IsAdmin) // 管理员标识（从 JWT Claims 读取，无需查库）
		c.Next()
	}
}

// JWTAuthWithCache JWT 认证中间件（带 Redis SSO 验证）
// 验证 Access Token 并将用户信息存入上下文
// SSO: 同时检查 Redis 中 token 是否有效
func JWTAuthWithCache(cache store.CacheService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请先登录",
			})
			return
		}

		// 2. 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "Token 格式错误，请使用 Bearer Token",
			})
			return
		}
		tokenString := parts[1]

		// 3. 验证 Token 签名
		claims, err := jwt.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "Token 已过期或无效，请重新登录",
			})
			return
		}

		// 4. 验证是否为 Access Token
		if claims.Subject != "access_token" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请使用 Access Token 访问此接口",
			})
			return
		}

		// 5. SSO 验证: 检查 Redis 中是否存在该 token
		if cache != nil {
			ssoTokenKey := constants.CacheKeySSOToken + claims.UserID
			storedToken, err := cache.Get(c.Request.Context(), ssoTokenKey)
			if err != nil || storedToken == "" {
				// Token 不在 Redis 中，说明用户已登出或被踢
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code": errorx.CodeUnauthorized,
					"msg":  "登录状态已失效，请重新登录",
				})
				return
			}
			// 验证 token 是否匹配（防止 token 被篡改）
			if storedToken != tokenString {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code": errorx.CodeUnauthorized,
					"msg":  "您的账号已在其他设备登录，请重新登录",
				})
				return
			}
		}

		// 6. 将用户信息存入上下文，供后续 Handler 使用
		c.Set("user_id", claims.UserID)
		c.Set("is_admin", claims.IsAdmin) // 管理员标识（从 JWT Claims 读取，无需查库）
		c.Next()
	}
}
