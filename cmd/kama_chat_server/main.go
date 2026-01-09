package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/service"
	"kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/util/jwt"

	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}
	zap.L().Info("日志初始化成功")

	// 3. 初始化数据库
	repos := dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)
	zap.L().Info("JWT 初始化成功")

	// 6. 初始化 Service 层 (依赖注入)
	services := service.NewServices(repos, cacheService)
	zap.L().Info("Service 层初始化成功")

	// 7. 初始化 ChatServer（依赖注入）
	chatServer := chat.NewChatServer(chat.ChatServerConfig{
		Mode:            conf.KafkaConfig.MessageMode,
		MessageRepo:     repos.Message,
		GroupMemberRepo: repos.GroupMember,
		CacheService:    cacheService,
	})
	if conf.KafkaConfig.MessageMode == "kafka" {
		chatServer.InitKafka()
	}
	zap.L().Info("ChatServer 初始化成功")

	// 8. 初始化 Handler 层 (依赖注入，包含 ChatServer 的 broker)
	handlers := handler.NewHandlers(services, chatServer.GetBroker())
	zap.L().Info("Handler 层初始化成功")

	// 9. 初始化 SMS Service (依赖注入缓存服务)
	if err := sms.Init(cacheService); err != nil {
		zap.L().Fatal("SMS Service 初始化失败", zap.Error(err))
	}
	zap.L().Info("SMS Service 初始化成功")

	// 10. 初始化 HTTPS 服务器 (传入 handlers 进行依赖注入)
	engine := https_server.Init(handlers)
	zap.L().Info("HTTPS 服务器初始化成功")

	// 11. 启动服务
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	// 启动聊天服务器
	go chatServer.Start()

	go func() {
		// Ubuntu22.04云服务器部署
		// 运行 HTTP 服务
		if err := engine.Run(fmt.Sprintf("%s:%d", host, port)); err != nil {
			zap.L().Fatal("server running fault")
			return
		}
	}()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	<-quit

	// 关闭聊天服务器（包括 Kafka 客户端）
	chatServer.Close()

	zap.L().Info("关闭服务器...")

	zap.L().Info("服务器已关闭")
}
