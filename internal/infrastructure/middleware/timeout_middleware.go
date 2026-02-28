package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// RequestTimeout 请求超时中间件
// timeout: 单个请求允许的最大处理时长
// skipPrefixes: 跳过超时控制的路径前缀（如 /ws）
func RequestTimeout(timeout time.Duration, skipPrefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, prefix := range skipPrefixes {
			if prefix != "" && strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusRequestTimeout, gin.H{
				"code": errorx.CodeServerBusy,
				"msg":  "请求超时，请稍后重试",
			})
		}
	}
}
