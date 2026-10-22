package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/unrolled/secure"
	"go.uber.org/zap"
)

func TlsHandler(host string, port int) gin.HandlerFunc {
	// 1. 在返回函数之前初始化，避免每次请求都重复创建对象 (性能优化)
	secureMiddleware := secure.New(secure.Options{
		SSLRedirect: true,
		SSLHost:     host + ":" + strconv.Itoa(port),
	})

	return func(c *gin.Context) {
		err := secureMiddleware.Process(c.Writer, c.Request)

		// If there was an error, do not continue.
		if err != nil {
			// 2. 绝对不要在中间件里用 Fatal，否则服务会挂掉！
			// 使用 Error 记录日志，并终止当前请求
			zap.L().Error("TLS redirection failed", zap.Error(err))

			// 终止后续的处理链，不再执行后续的 handler
			c.Abort()
			return
		}

		// 3. 继续处理下一个 handler（如果是重定向，secureMiddleware 内部可能已经处理了响应，这里其实通常不会走到 Next，视具体配置而定）
		c.Next()
	}
}
