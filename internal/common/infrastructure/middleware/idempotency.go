// Package middleware 提供 HTTP 中间件
// 本文件定义基于 Redis 的写接口幂等性中间件
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

const (
	// HeaderIdempotencyKey 幂等键请求头
	HeaderIdempotencyKey = "X-Idempotency-Key"
	// HeaderIdempotentReplay 命中幂等回放响应头
	HeaderIdempotentReplay = "X-Idempotent-Replay"
	// inProgressValue 正在执行中的标记
	inProgressValue = "IN_PROGRESS"
	// inProgressTTL 执行中锁的超时兜底时间（防止服务端 panic 导致死锁）
	inProgressTTL = 1 * time.Minute
)

// cachedResponse 缓存的历史响应体结构
type cachedResponse struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

// bodyCaptureWriter 响应体捕获 Writer
type bodyCaptureWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency 幂等中间件
// cache: Redis 缓存服务
// ttl: 幂等记录成功保存的时长（如 24 小时）
func Idempotency(cache store.CacheService, ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅对非幂等的写请求方法生效 (POST, PUT, DELETE, PATCH)
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		idempKey := c.GetHeader(HeaderIdempotencyKey)
		// 所有写请求强制要求提供 X-Idempotency-Key 幂等键
		if idempKey == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code": errorx.CodeInvalidParam,
				"msg":  "写请求必须提供 X-Idempotency-Key 请求头",
			})
			return
		}

		// 将同一个业务幂等键注入 Request Context，供后续 gRPC Client 拦截器全链路透传
		c.Request = c.Request.WithContext(interceptor.ContextWithIdempotencyKey(c.Request.Context(), idempKey))

		// 作用域隔离：优先绑定已认证的 user_id，未登录则绑定客户端 IP
		userScope := c.ClientIP()
		if uid, exists := c.Get("user_id"); exists {
			if uidStr, ok := uid.(string); ok && uidStr != "" {
				userScope = uidStr
			}
		}

		redisKey := fmt.Sprintf("idemp:%s:%s:%s:%s", userScope, method, c.Request.URL.Path, idempKey)
		ctx := c.Request.Context()

		// 1. 尝试原子抢占锁
		acquired, err := cache.SetNX(ctx, redisKey, inProgressValue, inProgressTTL)
		if err != nil {
			// Redis 故障降级放行，不阻塞业务主链路
			zap.L().Warn("幂等中间件 Redis 故障，降级放行", zap.String("key", redisKey), zap.Error(err))
			c.Next()
			return
		}

		if !acquired {
			// 2. 抢占失败：已存在相同请求
			val, getErr := cache.Get(ctx, redisKey)
			if getErr != nil || val == "" {
				// 获取异常时降级放行
				c.Next()
				return
			}

			if val == inProgressValue {
				// 前序相同请求仍在处理中，拦截重复提交
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"code": errorx.CodeTooManyRequests,
					"msg":  "请求正在处理中，请勿重复提交",
				})
				return
			}

			// 前序请求已完成，直接回放历史响应结果
			var cached cachedResponse
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				c.Header(HeaderIdempotentReplay, "true")
				contentType := c.Writer.Header().Get("Content-Type")
				if contentType == "" {
					contentType = "application/json; charset=utf-8"
				}
				c.Data(cached.StatusCode, contentType, []byte(cached.Body))
				c.Abort()
				return
			}

			// 解析缓存失败则降级正常执行
			c.Next()
			return
		}

		// 3. 抢占成功：包装 ResponseWriter 捕获业务执行输出
		writer := &bodyCaptureWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = writer

		c.Next()

		// 4. 后置处理：依据响应状态码决定持久化缓存还是释放锁
		statusCode := c.Writer.Status()
		if statusCode < 500 && statusCode != http.StatusRequestTimeout && statusCode != http.StatusTooManyRequests {
			// 业务确定性处理（成功 200 或业务参数错误 400，重试均应幂等回放）
			cachedData, err := json.Marshal(cachedResponse{
				StatusCode: statusCode,
				Body:       writer.body.String(),
			})
			if err == nil {
				_ = cache.Set(context.Background(), redisKey, string(cachedData), ttl)
			}
		} else {
			// 服务端内部异常 (5xx)、请求超时 (408) 或并发限流 (429)，删除标记允许客户端重试
			_ = cache.Delete(context.Background(), redisKey)
		}
	}
}
