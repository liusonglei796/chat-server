// Package https_server 提供 HTTP/HTTPS 服务器的初始化和配置
// 负责创建 Gin 引擎实例并配置中间件、静态资源和路由
package https_server

import (
	"kama_chat_server/internal/config"                // 配置管理
	"kama_chat_server/internal/handler"               // Handler 聚合对象
	"kama_chat_server/internal/infrastructure/logger" // 自定义日志中间件
	"kama_chat_server/internal/router"                // 路由注册

	"github.com/gin-contrib/cors" // CORS 跨域中间件
	"github.com/gin-gonic/gin"    // Gin Web 框架
)

// GE 全局 Gin 引擎实例
// 供 main.go 调用 GE.Run() 或 GE.RunTLS() 启动服务
var GE *gin.Engine

// Init 初始化 HTTP/HTTPS 服务器
// handlers: 通过依赖注入传入的 handler 聚合对象
// 配置顺序：
//  1. 创建 Gin 引擎（空白，不含默认中间件）
//  2. 注册日志和恢复中间件
//  3. 配置 CORS 跨域规则
//  4. 映射静态资源目录
//  5. 注册业务路由
func Init(handlers *handler.Handlers) {
	// 创建空白 Gin 引擎（不使用 gin.Default() 以便完全控制中间件）
	GE = gin.New()

	// 注册自定义 Zap 日志中间件，替代 Gin 默认的日志
	// GinLogger: 记录每个请求的详细信息（路径、状态码、耗时等）
	GE.Use(logger.GinLogger())

	// 注册 Panic 恢复中间件，捕获 panic 并记录堆栈
	// 参数 true 表示在日志中包含堆栈信息
	GE.Use(logger.GinRecovery(true))

	// 配置 CORS 跨域规则
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"} // 允许所有来源（生产环境应指定具体域名）
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	GE.Use(cors.New(corsConfig))

	// TLS 重定向中间件（可选，如果由 Nginx 处理 SSL 则注释掉）
	// 功能：将 HTTP 请求自动重定向到 HTTPS
	// GE.Use(middleware.TlsHandler(config.GetConfig().MainConfig.Host, config.GetConfig().MainConfig.Port))

	// 映射静态资源目录
	// /static/avatars -> 头像文件目录
	GE.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	// /static/files -> 普通上传文件目录
	GE.Static("/static/files", config.GetConfig().StaticFilePath)

	// 创建路由管理器并注册所有业务路由
	rt := router.NewRouter(handlers)
	rt.RegisterRoutes(GE)
}
