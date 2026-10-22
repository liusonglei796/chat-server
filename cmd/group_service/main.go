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

	grouppb "kama_chat_server/api/gen/group"
	"kama_chat_server/internal/group"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/logger"
	"kama_chat_server/internal/infra/outbox"
	redisinfra "kama_chat_server/internal/infra/redis"
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
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "group_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	// Step 2.1: 初始化持久化数据层连接
	db, err := database.Open(&conf.MysqlConfig, "chat_group")
	if err != nil {
		zap.L().Fatal("failed to connect chat_group database", zap.Error(err))
	}
	if err := database.AutoMigrate(db, &group.GroupInfo{}, &group.GroupMember{}, &outbox.Outbox{}); err != nil {
		zap.L().Fatal("failed to migrate chat_group database", zap.Error(err))
	}
	uow := group.NewUoW(db)

	// Step 2.2: 初始化 Redis 异步缓存服务
	cacheService := redisinfra.Init()

	// Step 2.3: 初始化内部微服务 gRPC 客户端
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// Step 3.1: 初始化群组业务领域服务与处理器
	groupSvc := group.NewGroupService(uow, cacheService)
	groupServerImpl := group.NewGrpcServer(groupSvc)

	// Step 3.2: 启动领域事件消费者 (Kafka 异步消费)
	consumer := group.NewDomainEventConsumer(uow, cacheService)
	consumer.Start()
	defer consumer.Close()

	// Step 4.1: 构建 Kratos 纯 gRPC 传输服务
	grpcPort := 50055
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
	grouppb.RegisterGroupServiceServer(grpcSrv, groupServerImpl)

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
		Name:   "group_service",
		Addr:   fmt.Sprintf("group-service:%d", grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	} else {
		defer legacyRegister.Stop()
	}

	// Step 5.1: 组装并启动 Kratos 应用
	app := kratos.New(
		kratos.Name("group_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(
			grpcSrv,
		),
		kratos.Registrar(kratosRegistrar),
	)

	zap.L().Info("Group Service (Pure gRPC Single Protocol) starting...",
		zap.String("grpcAddr", grpcAddr),
	)

	// Step 5.2: 阻塞运行应用直到接收到终止信号
	if err := app.Run(); err != nil {
		zap.L().Fatal("group service exited with error", zap.Error(err))
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
	zap.L().Info("Group Service stopped successfully")
}
