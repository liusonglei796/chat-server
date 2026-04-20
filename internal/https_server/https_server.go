// Package https_server 提供 HTTP/HTTPS 服务器的初始化和配置
// 负责创建 Gin 引擎实例并配置中间件、静态资源和路由
package https_server

import (
	"kama_chat_server/internal/config" // 配置管理
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/handler"                   // Handler 聚合对象
	"kama_chat_server/internal/infrastructure/logger"     // 自定义日志中间件
	"kama_chat_server/internal/infrastructure/middleware" // 中间件
	"kama_chat_server/internal/router"                    // 路由注册
	"time"

	"github.com/gin-contrib/cors" // CORS 跨域中间件
	"github.com/gin-gonic/gin"    // Gin Web 框架
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Init 初始化 HTTP/HTTPS 服务器并返回 Gin 引擎实例
// handlers: 通过依赖注入传入的 handler 聚合对象（已包含预构造的中间件）
// adminChecker: 可选的管理员权限实时校验回调（查库验证权限未被撤销）
// cache: Redis 缓存服务，供限流等中间件使用
// 配置顺序：
//  1. 创建 Gin 引擎（空白，不含默认中间件）
//  2. 注册日志和恢复中间件
//  3. 配置 CORS 跨域规则
//  4. 映射静态资源目录
//  5. 注册业务路由
//
// 返回: 配置完成的 Gin 引擎实例
func Init(handlers *handler.Handlers, adminChecker middleware.AdminAuthChecker, cache repository.CacheService) *gin.Engine {
	// 创建空白 Gin 引擎（不使用 gin.Default() 以便完全控制中间件）
	engine := gin.New()
	// 注册 OpenTelemetry 追踪中间件（必须在最前面，确保所有请求都被追踪）
	// 使用 otelgin 官方中间件自动创建 span、提取/传播 trace context
	engine.Use(middleware.OtelTracing())
	// 注入 traceId 到响应头 X-Trace-Id，方便客户端关联日志
	engine.Use(middleware.InjectTraceId())
	// 注册自定义 Zap 日志中间件，替代 Gin 默认的日志
	// GinLogger: 记录每个请求的详细信息（路径、状态码、耗时等）
	engine.Use(logger.GinLogger())
	// 注册 Panic 恢复中间件，捕获 panic 并记录堆栈
	// 参数 true 表示在日志中包含堆栈信息
	engine.Use(logger.GinRecovery(true))
	// 全局请求超时控制（默认 15 秒）
	// 跳过 /ws 前缀，避免影响 WebSocket 长连接
	engine.Use(middleware.RequestTimeout(15*time.Second, "/ws"))
	// 配置 CORS 跨域规则
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"} // 允许所有来源（生产环境应指定具体域名）
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	engine.Use(cors.New(corsConfig))
	// TLS 重定向中间件（可选，如果由 Nginx 处理 SSL 则注释掉）
	// 功能：将 HTTP 请求自动重定向到 HTTPS
	// engine.Use(middleware.TlsHandler(config.GetConfig().MainConfig.Host, config.GetConfig().MainConfig.Port))
	// 映射静态资源目录
	// /static/avatars -> 头像文件目录
	engine.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	// /static/files -> 普通上传文件目录
	engine.Static("/static/files", config.GetConfig().StaticFilePath)
	// /web -> 前端页面（SPA 应用）
	engine.Static("/web", "./web")
	// 根路径返回前端页面
	engine.NoRoute(func(c *gin.Context) {
		// API 路由返回 404
		if len(c.Request.URL.Path) > 1 && c.Request.URL.Path != "/" {
			c.File("./web/index.html")
		} else {
			c.File("./web/index.html")
		}
	})
	// 创建路由管理器并注册所有业务路由
	rt := router.NewRouter(handlers, adminChecker, cache)
	rt.RegisterRoutes(engine)

	// 注册 Prometheus /metrics 端点
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return engine
}
