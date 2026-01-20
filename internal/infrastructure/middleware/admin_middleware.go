package middleware

import (
	"net/http"

	"kama_chat_server/internal/dao/mysql"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// AdminAuth 管理员权限校验中间件
// 必须在 JWTAuth 中间件之后使用，依赖 JWT 中间件设置的 user_id
// userRepo: 用户数据访问接口，用于查询用户管理员状态
func AdminAuth(userRepo mysql.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取 JWT 中间件设置的用户 ID
		userId, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请先登录",
			})
			return
		}

		// 2. 查询用户信息
		user, err := userRepo.FindByUuid(userId.(string))
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code": errorx.CodeForbidden,
					"msg":  "用户不存在",
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code": errorx.CodeServerBusy,
				"msg":  "服务繁忙，请稍后重试",
			})
			return
		}

		// 3. 校验管理员权限
		// IsAdmin: 0=普通用户, 1=管理员
		if user.IsAdmin != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": errorx.CodeForbidden,
				"msg":  "无管理员权限",
			})
			return
		}

		// 4. 将管理员信息存入上下文（可选，供后续使用）
		c.Set("is_admin", true)
		c.Next()
	}
}
