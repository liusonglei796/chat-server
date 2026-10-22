package main

import (
	"context"
	"fmt"
	"log"
	"time"

	etcdregistry "github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/kafka"
	"kama_chat_server/internal/infra/logger"
	redisinfra "kama_chat_server/internal/infra/redis"
	"kama_chat_server/internal/message"
	"kama_chat_server/pkg/discovery"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	// Step 1.1: 加载应用主配置
	conf := config.GetConfig()

	// Step 1.2: 初始化结构化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// Step 1.3: 初始化 OpenTelemetry 链路追踪
	var otelShutdown func(context.Context) error
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "message_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", conf.OtelConfig.Endpoint),
			zap.String("serviceName", "message_service"),
		)
	}

	// Step 2.1: 初始化持久化数据层连接与自动迁移
	db, err := database.Open(&conf.MysqlConfig, "chat_message")
	if err != nil {
		zap.L().Fatal("failed to connect chat_message database", zap.Error(err))
	}
	if err := database.AutoMigrate(db, &message.Message{}, &message.Session{}); err != nil {
		zap.L().Fatal("failed to migrate chat_message database", zap.Error(err))
	}
	uow := message.NewUoW(db)

	// Step 2.2: 初始化 Redis 异步缓存服务
	cacheService := redisinfra.Init()

	// Step 2.3: 初始化内部微服务 gRPC 客户端
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// Step 3.1: 初始化领域服务与 gRPC 服务实现
	msgSvc := message.NewMessageService(uow.MessageStore(), uow.SessionStore(), cacheService)
	sessionSvc := message.NewSessionService(uow.SessionStore(), uow.MessageStore(), cacheService)
	messageServerImpl := message.NewGrpcServer(msgSvc, sessionSvc)

	// Step 3.2: 启动领域事件消费者 (Kafka 异步消费)
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Fatal("failed to ensure domain_events topic", zap.Error(err))
	}
	eventHandler := message.NewSessionEventHandler(uow.SessionStore(), cacheService)
	eventConsumer := message.NewDomainEventConsumer(eventHandler)
	eventConsumer.Start()
	defer eventConsumer.Close()

	// Step 4.1: 构建 Kratos 纯 gRPC 传输服务
	grpcPort := 50056
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", grpcPort)
	grpcSrv := kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Timeout(2*time.Second),
		kgrpc.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			metadata.Server(),
		),
	)
	messagepb.RegisterMessageServiceServer(grpcSrv, messageServerImpl)

	// Step 4.2: 实例化 Etcd 官方客户端与 Kratos 服务注册中心
	etcdEndpoints := []string{"etcd:2379", "127.0.0.1:2379"}
	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		zap.L().Fatal("failed to connect etcd", zap.Error(err))
	}
	defer etcdCli.Close()
	kratosRegistrar := etcdregistry.New(etcdCli)

	// Step 4.3: 维持旧版服务发现兼容注册
	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   "message_service",
		Addr:   fmt.Sprintf("message-service:%d", grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	} else {
		defer legacyRegister.Stop()
	}

	// Step 5.1: 组装并启动 Kratos 应用
	app := kratos.New(
		kratos.Name("message_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(
			grpcSrv,
		),
		kratos.Registrar(kratosRegistrar),
	)

	zap.L().Info("Message Service (Pure gRPC Single Protocol) starting...",
		zap.String("grpcAddr", grpcAddr),
	)

	// Step 5.2: 阻塞运行应用直到接收到终止信号
	if err := app.Run(); err != nil {
		zap.L().Fatal("message service exited with error", zap.Error(err))
	}

	// Step 6.1: 优雅清理 Redis 异步任务池与监控追踪资源
	if rc, ok := cacheService.(*redisinfra.RedisCache); ok {
		rc.Release()
	}
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("Message Service stopped successfully")
}
