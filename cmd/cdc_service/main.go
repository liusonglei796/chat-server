package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"go.uber.org/zap"

	"kama_chat_server/internal/common/config"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/logger"
	"kama_chat_server/internal/common/infrastructure/outbox/cdc"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	conf := config.GetConfig()

	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	if conf.OtelConfig.Enabled {
		_, err := otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "cdc_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	// 确保 Kafka 领域事件主题存在
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Warn("ensure kafka topic error", zap.Error(err))
	}

	// Redis 位点存储
	rdb := myredis.NewClient()
	posStore := cdc.NewRedisPositionStore(rdb)

	// Kafka 发布者
	kafkaProducer, err := kafka.NewProducer(kafka.TopicDomainEvents)
	if err != nil {
		zap.L().Fatal("initialize Kafka producer failed", zap.Error(err))
	}
	publishFn := cdc.MakeKafkaPublishFunc(kafkaProducer)

	// MySQL 配置：优先读取容器环境变量，回退至配置文件
	mysqlHost := os.Getenv("MYSQL_HOST")
	if mysqlHost == "" {
		mysqlHost = conf.MysqlConfig.Host
	}
	mysqlPortStr := os.Getenv("MYSQL_PORT")
	mysqlPort := conf.MysqlConfig.Port
	if mysqlPortStr != "" {
		if p, err := strconv.Atoi(mysqlPortStr); err == nil {
			mysqlPort = p
		}
	}
	mysqlUser := os.Getenv("MYSQL_USER")
	if mysqlUser == "" {
		mysqlUser = conf.MysqlConfig.User
	}
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	if mysqlPassword == "" {
		mysqlPassword = conf.MysqlConfig.Password
	}

	syncerCfg := cdc.SyncerConfig{
		Addr:              net.JoinHostPort(mysqlHost, strconv.Itoa(mysqlPort)),
		User:              mysqlUser,
		Password:          mysqlPassword,
		ServerID:          1001,
		IncludeTableRegex: []string{`^chat_.*\.outbox$`},
		PosStore:          posStore,
		PublishFn:         publishFn,
	}

	syncer, err := cdc.NewCDCSyncer(syncerCfg)
	if err != nil {
		zap.L().Fatal("initialize CDC syncer failed", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		zap.L().Info("CDC Syncer service starting...",
			zap.String("mysqlAddr", syncerCfg.Addr),
			zap.String("mysqlUser", syncerCfg.User),
		)
		if err := syncer.Start(ctx); err != nil {
			zap.L().Error("CDC Syncer stopped with error", zap.Error(err))
		}
	}()

	sig := <-quit
	zap.L().Info(fmt.Sprintf("received signal %v, shutting down CDC service...", sig))
	cancel()
	syncer.Close()
	kafkaProducer.Close()
	rdb.Close()
	zap.L().Info("CDC service exited cleanly")
}
