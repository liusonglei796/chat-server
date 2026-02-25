package logger

import (
	"fmt"
	"os"

	"kama_chat_server/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化 Logger
// 根据配置创建 zap logger，支持开发/生产模式
func Init(cfg *config.LogConfig, mode string) error {
	if cfg == nil {
		return fmt.Errorf("logger.Init received nil config")
	}

	// 设置默认值
	setDefaultIfEmpty(cfg)

	// 获取日志写入器和编码器
	writeSyncer := getLogWriter(cfg.FileName, cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)
	encoder := getJSONEncoder()

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return err
	}

	var core zapcore.Core
	if mode == "dev" || mode == gin.DebugMode {
		// 开发模式：同时输出到控制台和文件
		core = newDevCore(encoder, writeSyncer, level)
	} else {
		// 生产模式：只输出到文件（JSON 格式）
		core = zapcore.NewCore(encoder, writeSyncer, level)
	}

	// 创建并替换全局 Logger
	lg := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(lg)

	return nil
}

// setDefaultIfEmpty 设置配置默认值
func setDefaultIfEmpty(cfg *config.LogConfig) {
	if cfg.FileName == "" {
		cfg.FileName = cfg.LogPath + "/app.log"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 5
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 30
	}
	if cfg.Level == "" {
		cfg.Level = "info"
	}
}

// newDevCore 创建开发模式的 Core
// 同时输出到控制台（易读）和文件（便于追溯）
func newDevCore(encoder zapcore.Encoder, writeSyncer zapcore.WriteSyncer, level zapcore.Level) zapcore.Core {
	consoleEncoder := getConsoleEncoder()
	fileCore := zapcore.NewCore(encoder, writeSyncer, level)
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zapcore.DebugLevel)
	return zapcore.NewTee(fileCore, consoleCore)
}
