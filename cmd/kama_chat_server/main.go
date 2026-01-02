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
	"kama_chat_server/internal/gateway/websocket"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/infrastructure/logger"
	mq "kama_chat_server/internal/infrastructure/mq"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/service"
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
	dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	myredis.Init()
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)
	zap.L().Info("JWT 初始化成功")

	// 6. 初始化 Service 层 (依赖注入)
	service.InitServices(dao.Repos)
	zap.L().Info("Service 层初始化成功")

	// 7. 初始化 SMS Service
	if err := sms.Init(); err != nil {
		zap.L().Fatal("SMS Service 初始化失败", zap.Error(err))
	}
	zap.L().Info("SMS Service 初始化成功")

	// 6. 初始化 ChatServer
	websocket.Init()
	if conf.KafkaConfig.MessageMode == "kafka" {
		mq.KafkaService.KafkaInit()
		mq.InitKafkaServer()
		// 注入 MessageSender 接口实现 (依赖倒置: mq → websocket)
		// mq.KafkaChatServer 实现了 mq.MessageSender 接口
		mq.SetMessageSender(mq.KafkaChatServer)
		// 注入 MessageWriter 接口实现 (依赖倒置: websocket → mq)
		// mq.KafkaService 实现了 websocket.MessageWriter 接口
		websocket.SetMessageWriter(mq.KafkaService)
		// 注入 ClientManager 接口实现
		websocket.SetClientManager(mq.KafkaChatServer)
	} else {
		// 注入 MessageSender 接口实现 (依赖倒置)
		mq.SetMessageSender(websocket.ChatServer)
		// 注入 ClientManager 接口实现
		websocket.SetClientManager(websocket.ChatServer)
	}
	zap.L().Info("ChatServer 初始化成功")

	// 6. 初始化 HTTPS 服务器
	https_server.Init()
	zap.L().Info("HTTPS 服务器初始化成功")

	// 7. 启动服务
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port
	kafkaConfig := conf.KafkaConfig

	if kafkaConfig.MessageMode == "channel" {
		go websocket.ChatServer.Start()
	} else {
		go mq.KafkaChatServer.Start()
	}

	go func() {
		// Ubuntu22.04云服务器部署
		// 运行 HTTP 服务
		if err := https_server.GE.Run(fmt.Sprintf("%s:%d", host, port)); err != nil {
			zap.L().Fatal("server running fault")
			return
		}
	}()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	<-quit

	if kafkaConfig.MessageMode == "kafka" {
		mq.KafkaService.KafkaClose()
	}

	websocket.ChatServer.Close()

	zap.L().Info("关闭服务器...")

	zap.L().Info("服务器已关闭")
}
