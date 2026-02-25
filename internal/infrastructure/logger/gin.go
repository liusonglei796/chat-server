package logger

import (
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinLogger Gin 日志中间件
// 将 HTTP 请求日志通过 zap 输出
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		cost := time.Since(start)

		zap.L().Info("http request",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("clientIP", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.Duration("cost", cost),
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)
	}
}

// GinRecovery Panic 恢复中间件
// 捕获 panic 并记录日志，防止服务崩溃
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				var brokenPipe bool
				if err, ok := rec.(error); ok {
					brokenPipe = isBrokenPipeError(err)
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				fields := []zap.Field{
					zap.Any("error", rec),
					zap.String("request", string(httpRequest)),
				}

				if brokenPipe {
					zap.L().Error("broken pipe",
						append(fields, zap.String("path", c.Request.URL.Path))...,
					)
					c.Error(rec.(error))
					c.Abort()
					return
				}

				if stack {
					fields = append(fields, zap.String("stack", getStackTrace()))
				}
				zap.L().Error("[Recovery from panic]", fields...)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
