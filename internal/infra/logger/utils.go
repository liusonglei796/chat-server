package logger

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// isBrokenPipeError 检查错误链中是否包含 broken pipe
// 用于判断是否是网络连接中断导致的错误
func isBrokenPipeError(err error) bool {
	if err == nil {
		return false
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var syscallErr *os.SyscallError
		if errors.As(opErr.Err, &syscallErr) {
			msg := strings.ToLower(syscallErr.Error())
			return strings.Contains(msg, "broken pipe") ||
				strings.Contains(msg, "connection reset by peer")
		}
	}

	// 兜底检查
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer")
}

// getStackTrace 获取堆栈信息
func getStackTrace() string {
	return string(debug.Stack())
}

// GetTraceId 从上下文中提取 OTel trace ID
// 如果上下文中没有 span 信息，返回空字符串
func GetTraceId(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String()
	}
	return ""
}
