package middleware

import (
	"net/http"
	"strings"

	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/jwt"

	"github.com/gin-gonic/gin"
)

// JWTAuth JWT 认证中间件
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
		c.Next()
	}
}
