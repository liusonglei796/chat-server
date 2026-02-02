package middleware

import (
	"net/http"

	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// AdminAuth 管理员权限校验中间件
// 必须在 JWTAuth 中间件之后使用，依赖 JWT 中间件设置的 is_admin
// 纯 JWT Claims 方案：从 context 读取 is_admin，无需查库
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取 JWT 中间件设置的管理员标识
		isAdmin, exists := c.Get("is_admin")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请先登录",
			})
			return
		}

		// 2. 校验管理员权限
		if !isAdmin.(bool) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": errorx.CodeForbidden,
				"msg":  "无管理员权限",
			})
			return
		}

		c.Next()
	}
}
