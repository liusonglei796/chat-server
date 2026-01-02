package logger

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"kama_chat_server/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Init 初始化 Logger
// 为什么：日志组件需要根据配置（如文件路径、级别）进行初始化，才能正确输出日志
func Init(cfg *config.LogConfig, mode string) (err error) {
	if cfg == nil {
		return fmt.Errorf("logger.Init received nil config")
	}

	// 设置默认值
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

	// 获取日志写入器，支持日志切割
	writeSyncer := getLogWriter(
		cfg.FileName,
		cfg.MaxSize,
		cfg.MaxBackups,
		cfg.MaxAge,
	)
	// 获取日志编码器，决定日志的输出格式（如 JSON）
	encoder := getEncoder()

	var level zapcore.Level
	// 将配置文件中的字符串（如 "info", "debug"）转换成 zap 日志库能识别的内部类型
	if err = level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return
	}
	var core zapcore.Core
	if mode == "dev" || mode == gin.DebugMode {
		// ---------------------------------
		// 开发模式 (dev)，日志输出到控制台和文件
		// ---------------------------------

		// 1. 创建一个用于控制台输出的 Encoder (更易读)
		// 为什么：开发时看 JSON 格式比较累，Console 格式更直观
		consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

		// 2. 创建一个用于文件输出的 Core
		// 为什么：即使是开发模式，也可能需要查看历史日志文件
		fileCore := zapcore.NewCore(encoder, writeSyncer, level)

		// 3. 创建一个用于控制台输出的 Core
		//    - zapcore.Lock(os.Stdout) 表示标准输出
		//    - zapcore.DebugLevel 表示这个 Core 只处理 Debug 及以上级别的日志
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zapcore.DebugLevel)

		// 4. 使用 zapcore.NewTee 合并多个 Core
		//    Tee 的意思是 "三通管"，它会将日志同时分发到所有传入的 Core
		// 为什么：同时满足"看控制台方便调试"和"存文件方便追溯"的需求
		core = zapcore.NewTee(fileCore, consoleCore)
	} else {
		// ---------------------------------
		// 生产模式 (release)，日志只输出到文件
		// ---------------------------------
		// 为什么：生产环境不需要输出到控制台（通常会被忽略或导致性能问题），且必须结构化（JSON）以便日志收集系统（如 ELK）解析
		core = zapcore.NewCore(encoder, writeSyncer, level)
	}
	// 创建 Logger 实例
	// zap.AddCaller() 会在日志中添加调用者的文件名和行号，方便定位代码
	lg := zap.New(core, zap.AddCaller())
	// 替换全局的 Logger，后续在其他包中可以直接使用 zap.L() 调用
	zap.ReplaceGlobals(lg)
	return
}

// getLogWriter 获取日志写入器
// 为什么：使用 lumberjack 库实现日志切割（Log Rotation），防止单个日志文件过大占满磁盘
func getLogWriter(filename string, maxSize int, maxBackups int, maxAge int) zapcore.WriteSyncer {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   filename,   // 日志文件路径
		MaxSize:    maxSize,    // 单个日志文件最大大小（MB）
		MaxBackups: maxBackups, // 保留旧日志文件的最大个数
		MaxAge:     maxAge,     // 保留旧日志文件的最大天数
	}
	return zapcore.AddSync(lumberjackLogger)
}

// getEncoder 获取日志编码器
// 为什么：配置日志的输出格式，这里使用 JSON 格式，适合机器解析
func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoderConfig.TimeKey = "time"                          // 时间字段的 key
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder   // 时间格式，如 2023-01-01T12:00:00.000Z
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 级别大写，如 INFO, ERROR
	return zapcore.NewJSONEncoder(encoderConfig)
}

// GinLogger 是一个中间件构造函数，返回 gin.HandlerFunc 类型
// 为什么：Gin 默认的 Logger 中间件输出格式固定，无法直接对接 zap。我们需要自定义中间件将 Gin 的请求日志通过 zap 输出。
func GinLogger() gin.HandlerFunc {
	// 返回一个匿名函数，这个函数是实际处理请求的逻辑
	// c *gin.Context 是 Gin 的核心上下文，包含了 Request 和 Writer
	return func(c *gin.Context) {

		// 1. 【请求前逻辑】
		// 记录请求进入的时间点，用于后续计算耗时
		start := time.Now()

		// 2. 【核心转折点】
		// c.Next() 表示"放行"。
		// 程序会暂停在这里，去执行后续的中间件和具体的 Controller (业务代码)。
		// 等到 Controller 处理完并返回响应后，程序会回到这里继续往下执行。
		c.Next()

		// 3. 【请求后逻辑】
		// 此时业务逻辑已经执行完毕，响应数据已经准备好发送给客户端

		// 计算总耗时 (当前时间 - 开始时间)
		cost := time.Since(start)

		// 使用 zap 的全局 Logger 记录一条 Info 级别的日志
		// "http request" 是这条日志的 Message (标题)
		zap.L().Info("http request",
			// 记录 HTTP 状态码 (如 200, 404, 500)
			// 注意：因为是在 c.Next() 之后，所以能拿到 Controller 设置的状态码
			zap.Int("status", c.Writer.Status()),

			// 记录 HTTP 请求方法 (GET, POST, PUT 等)
			zap.String("method", c.Request.Method),

			// 记录请求路径 (如 /api/v1/login)
			zap.String("path", c.Request.URL.Path),

			// 记录 URL 查询参数 (如 ?id=1&name=abc)
			zap.String("query", c.Request.URL.RawQuery),

			// 记录客户端 IP，Gin 会自动处理 X-Forwarded-For 等头信息
			zap.String("ClientIP", c.ClientIP()),

			// 记录用户代理 (浏览器信息、Postman 等)
			zap.String("user-agent", c.Request.UserAgent()),

			// 记录请求耗时，Zap 会自动格式化时间 (如 120ms)
			zap.Duration("cost", cost),

			// 记录 Gin 上下文中挂载的错误
			// 如果你在 Controller 里调用了 c.Error(err)，这里会把它记录下来
			// ErrorTypePrivate 通常是内部错误，不会直接返回给前端，但需要记日志
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)
	}
}

// GinRecovery 是一个中间件，用于捕获 panic 并恢复
// 为什么：防止某个请求处理发生 panic 导致整个服务崩溃。同时记录 panic 的堆栈信息到日志中。
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				// 1. 检查是否是 broken pipe（客户端断开连接）
				// 为什么：如果是 broken pipe，说明客户端已经断开了，没必要返回 500 错误，只需记录日志
				var brokenPipe bool
				if err, ok := rec.(error); ok {
					brokenPipe = isBrokenPipeError(err)
				}

				// 2. 获取请求信息（用于日志）
				// 为什么：记录 panic 发生时的请求内容，方便复现和排查
				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				requestStr := string(httpRequest)

				// 3. 统一日志字段作用就是"打包证据"。
				// zap.Any("error", rec): 记录案发原因（比如 "index out of range"）。
				// zap.String("request", ...): 记录案发现场（用户发了什么参数过来）。
				fields := []zap.Field{
					zap.Any("error", rec),
					zap.String("request", requestStr),
				}

				// 4. 处理 broken pipe（只记录，不返回响应）
				if brokenPipe {
					// append(fields, zap.String("path", ...)) 的意思是："在此基础之上，再追加一个 Path 字段"。
					zap.L().Error("broken pipe",
						append(fields, zap.String("path", c.Request.URL.Path))...,
					)
					c.Error(rec.(error)) // 包装为 error 类型
					c.Abort()
					return
				}

				// 5. 其他 panic，根据参数决定是否打印堆栈
				// 为什么：堆栈信息能精确指出代码哪一行出错了
				if stack {
					fields = append(fields, zap.String("stack", string(debug.Stack())))
				}
				zap.L().Error("[Recovery from panic]", fields...)
				// 返回 500 错误给客户端
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// isBrokenPipeError 检查错误链中是否包含 broken pipe
// 为什么：判断是否是网络连接中断导致的错误
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
