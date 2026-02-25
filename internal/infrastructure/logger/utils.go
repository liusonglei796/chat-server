package logger

import (
	"errors"
	"net"
	"os"
	"runtime/debug"
	"strings"
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
