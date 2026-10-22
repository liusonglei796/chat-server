package logger

import (
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// getLogWriter 获取日志写入器
// 使用 lumberjack 库实现日志切割，防止单个日志文件过大
func getLogWriter(filename string, maxSize int, maxBackups int, maxAge int) zapcore.WriteSyncer {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
	}
	return zapcore.AddSync(lumberjackLogger)
}
