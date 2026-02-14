package middleware

import (
	"net/http"

	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// AdminAuthChecker 管理员权限校验回调函数类型
// 传入用户ID，返回是否为管理员
type AdminAuthChecker func(userId string) (bool, error)

// AdminAuth 管理员权限校验中间件
// 必须在 JWTAuth 中间件之后使用
// 双重校验：先读 JWT Claims（快速拒绝），再通过回调实时查库（防止权限撤销后仍可访问）
// checker 为 nil 时退化为纯 JWT 校验
func AdminAuth(checker ...AdminAuthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 第一层：从 JWT Claims 快速拒绝非管理员（无需查库）
		isAdmin, exists := c.Get("is_admin")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": errorx.CodeUnauthorized,
				"msg":  "请先登录",
			})
			return
		}
		if !isAdmin.(bool) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": errorx.CodeForbidden,
				"msg":  "无管理员权限",
			})
			return
		}

		// 2. 第二层：实时校验（防止 JWT 签发后权限被撤销）
		if len(checker) > 0 && checker[0] != nil {
			userId, _ := c.Get("user_id")
			uid, ok := userId.(string)
			if !ok || uid == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code": errorx.CodeUnauthorized,
					"msg":  "用户身份无效",
				})
				return
			}

			realAdmin, err := checker[0](uid)
			if err != nil {
				// 查询失败时降级为 JWT 校验（已通过第一层），不阻塞请求
				// 记录日志但不返回错误，保证可用性
			} else if !realAdmin {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code": errorx.CodeForbidden,
					"msg":  "管理员权限已被撤销",
				})
				return
			}
		}

		c.Next()
	}
}
