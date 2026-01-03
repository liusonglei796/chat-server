package https_server

import (
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/router"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var GE *gin.Engine

// Init 初始化 HTTPS 服务器
func Init() {
	GE = gin.New()
	// 使用自定义的 zap logger 和 recovery 中间件
	GE.Use(logger.GinLogger())
	GE.Use(logger.GinRecovery(true))

	// CORS 配置
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	GE.Use(cors.New(corsConfig))

	// TLS 配置
	// GE.Use(middleware.TlsHandler(config.GetConfig().MainConfig.Host, config.GetConfig().MainConfig.Port))

	// 静态资源
	GE.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	GE.Static("/static/files", config.GetConfig().StaticFilePath)
	// 注册所有路由
	router.RegisterRoutes(GE)
}
