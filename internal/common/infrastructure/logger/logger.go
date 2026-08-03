package logger

import (
	"os"
	"sync"

	"kama_chat_server/internal/common/config"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// loggerOnce 确保日志只初始化一次
var loggerOnce sync.Once

// Init 初始化 Logger（单例模式）
// 根据配置创建 zap logger，支持开发/生产模式
func Init(logCfg *config.LogConfig, mode string) error {
	var initErr error
	loggerOnce.Do(func() {
		if logCfg == nil {
			initErr = errorx.New(errorx.CodeInvalidParam, "logger.Init received nil config")
			return
		}

		// 设置默认值
		setDefaultIfEmpty(logCfg)

		// 获取日志写入器和 JSON 编码器
		writeSyncer := getLogWriter(logCfg.FileName, logCfg.MaxSize, logCfg.MaxBackups, logCfg.MaxAge)
		jsonEncoder := getJSONEncoderForFile()

		var level zapcore.Level
		if err := level.UnmarshalText([]byte(logCfg.Level)); err != nil {
			initErr = errorx.Wrap(err, errorx.CodeInvalidParam, "无效的日志级别")
			return
		}

		var core zapcore.Core
		if mode == "dev" || mode == gin.DebugMode {
			// 开发模式：同时输出到控制台和文件
			consoleEncoder := getConsoleEncoderForTerminal()
			core = newDualCore(jsonEncoder, consoleEncoder, writeSyncer, level)
		} else {
			// 生产模式：只输出到文件（JSON 格式）
			core = zapcore.NewCore(jsonEncoder, writeSyncer, level)
		}

		// 创建并替换全局 Logger
		lg := zap.New(core, zap.AddCaller())
		zap.ReplaceGlobals(lg)
	})
	return initErr
}

// setDefaultIfEmpty 设置配置默认值
func setDefaultIfEmpty(logCfg *config.LogConfig) {
	if logCfg.FileName == "" {
		logCfg.FileName = logCfg.LogPath + "/app.log"
	}
	if logCfg.MaxSize == 0 {
		logCfg.MaxSize = 100
	}
	if logCfg.MaxBackups == 0 {
		logCfg.MaxBackups = 5
	}
	if logCfg.MaxAge == 0 {
		logCfg.MaxAge = 30
	}
	if logCfg.Level == "" {
		logCfg.Level = "info"
	}
}

// newDualCore 创建双输出 Core（开发模式）
// 同时输出到控制台（彩色易读）和文件（JSON格式便于分析）
// 参数说明：
//   - jsonEncoder: JSON 编码器，用于文件日志
//   - consoleEncoder: Console 编码器，用于控制台输出
//   - writeSyncer: 日志写入目标（文件）
//   - level: 日志级别
func newDualCore(jsonEncoder, consoleEncoder zapcore.Encoder, writeSyncer zapcore.WriteSyncer, level zapcore.Level) zapcore.Core {
	// 文件日志：使用 JSON encoder，按配置的级别输出
	fileCore := zapcore.NewCore(jsonEncoder, writeSyncer, level)

	// 控制台日志：使用 console encoder，始终输出 DebugLevel（更详细）
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zapcore.DebugLevel)

	return zapcore.NewTee(fileCore, consoleCore)
}
