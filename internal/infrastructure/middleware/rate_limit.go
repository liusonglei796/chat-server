// Package middleware 提供 HTTP 中间件
// 本文件定义基于 Redis 的固定窗口限流中间件
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/pkg/errorx"
)

// RateLimitKeyFunc 限流 key 提取函数类型
// 从请求中提取限流标识（如 IP、手机号等）
type RateLimitKeyFunc func(c *gin.Context) string

// RateLimit 固定窗口限流中间件
// cache: Redis 缓存服务（用于原子计数）
// keyPrefix: 缓存 key 前缀（如 "rate:sms:"）
// keyFunc: 从请求中提取限流标识的函数
// maxRequests: 窗口内允许的最大请求数
// window: 窗口时间长度
func RateLimit(cache myredis.CacheService, keyPrefix string, keyFunc RateLimitKeyFunc, maxRequests int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := keyFunc(c)
		if identifier == "" {
			c.Next()
			return
		}

		key := keyPrefix + identifier
		ctx := context.Background()

		// 原子递增
		count, err := cache.Incr(ctx, key)
		if err != nil {
			// Redis 故障时降级放行，不阻塞业务
			zap.L().Error("限流计数器异常，降级放行", zap.String("key", key), zap.Error(err))
			c.Next()
			return
		}

		// 首次访问时设置窗口过期时间
		if count == 1 {
			if err := cache.Expire(ctx, key, window); err != nil {
				zap.L().Error("设置限流窗口过期失败", zap.String("key", key), zap.Error(err))
			}
		}

		// 超限拒绝
		if count > maxRequests {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": errorx.CodeTooManyRequests,
				"msg":  "请求过于频繁，请稍后再试",
			})
			return
		}

		c.Next()
	}
}

// ByClientIP 按客户端 IP 限流
func ByClientIP(c *gin.Context) string {
	return c.ClientIP()
}

// ByFormPhone 按表单中 telephone 字段限流
func ByFormPhone(c *gin.Context) string {
	return c.Query("telephone")
}

// ByJSONPhone 按 JSON body 中 telephone 字段限流（需预读 body）
// 注意: Gin 的 ShouldBindJSON 会消耗 body，此函数从 PostForm 回退取值
func ByJSONPhone(c *gin.Context) string {
	phone := c.PostForm("telephone")
	if phone == "" {
		phone = c.Query("telephone")
	}
	return phone
}
