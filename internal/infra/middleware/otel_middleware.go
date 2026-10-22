package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
)

// OtelTracing 返回 otelgin 官方中间件
// 为每个 HTTP 请求自动创建 span，提取/传播 trace context
func OtelTracing() gin.HandlerFunc {
	return otelgin.Middleware("kama_chat_server")
}

// InjectTraceId 将 traceId 注入到 HTTP 响应头 X-Trace-Id
// 需放在 OtelTracing() 之后注册
func InjectTraceId() gin.HandlerFunc {
	return func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		if span.SpanContext().IsValid() {
			c.Header("X-Trace-Id", span.SpanContext().TraceID().String())
		}
		c.Next()
	}
}

// GetSpanFromContext 从 gin.Context 中提取当前 span
// 供下游 handler 使用
func GetSpanFromContext(c *gin.Context) trace.Span {
	return trace.SpanFromContext(c.Request.Context())
}
