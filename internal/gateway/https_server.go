package gateway

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/logger"
	"kama_chat_server/internal/infra/middleware"
)

// InitHTTPServer 初始化 HTTP/HTTPS 服务器并返回 Gin 引擎实例
func InitHTTPServer(handlers *Handlers, cache cache.CacheService) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.OtelTracing())
	engine.Use(middleware.InjectTraceId())
	engine.Use(logger.GinLogger())
	engine.Use(logger.GinRecovery(true))
	engine.Use(middleware.RequestTimeout(15*time.Second, "/ws"))

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	engine.Use(cors.New(corsConfig))

	engine.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	engine.Static("/static/files", config.GetConfig().StaticFilePath)
	engine.Static("/web", "./web")

	engine.NoRoute(func(c *gin.Context) {
		c.File("./web/index.html")
	})

	rt := NewRouter(handlers, cache)
	rt.RegisterRoutes(engine)

	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return engine
}
