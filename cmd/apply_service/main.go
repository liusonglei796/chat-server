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

	applypb "kama_chat_server/api/gen/apply"
	"kama_chat_server/internal/apply"
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
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "apply_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	// Step 2.1: 初始化持久化数据层连接与工作单元
	db, err := database.Open(&conf.MysqlConfig, "chat_apply")
	if err != nil {
		zap.L().Fatal("failed to connect chat_apply database", zap.Error(err))
	}
	if err := database.AutoMigrate(db, &apply.Apply{}, &outbox.Outbox{}); err != nil {
		zap.L().Fatal("failed to migrate chat_apply database", zap.Error(err))
	}
	uow := apply.NewUoW(db)

	// Step 2.2: 初始化 Redis 异步缓存服务
	cacheService := redisinfra.Init()

	// Step 2.3: 初始化内部微服务 gRPC 客户端
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// Step 3.1: 初始化申请业务领域服务与处理器
	applySvc := apply.NewApplyService(uow, cacheService)
	applyServerImpl := apply.NewGrpcServer(applySvc)

	// Step 4.1: 构建 Kratos 纯 gRPC 传输服务
	grpcPort := 50053
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", grpcPort)
	grpcSrv := kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Timeout(2*time.Second),
		kgrpc.Middleware(
			recovery.Recovery(recovery.WithHandler(func(ctx context.Context, req, err any) error {
				zap.L().Error("grpc server panic recovered", zap.Any("panic", err))
				return recovery.ErrUnknownRequest
			})),
			tracing.Server(),
			metadata.Server(),
		),
	)
	applypb.RegisterApplyServiceServer(grpcSrv, applyServerImpl)

	// Step 4.2: 初始化 etcd 服务注册
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

	// Step 4.3: 兼容注册 (旧版服务发现)
	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   "apply_service",
		Addr:   fmt.Sprintf("apply-service:%d", grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	} else {
		defer legacyRegister.Stop()
	}

	svcName := "apply-service"
	app := kratos.New(
		kratos.Name(svcName),
		kratos.Version("v1.0.0"),
		kratos.Server(grpcSrv),
		kratos.Registrar(kratosRegistrar),
	)

	// Step 5.1: 启动服务并监听优雅停机信号
	zap.L().Info("apply_service started successfully",
		zap.String("grpcAddr", grpcAddr),
	)

	if err := app.Run(); err != nil {
		zap.L().Fatal("apply_service run error", zap.Error(err))
	}

	// Step 6.1: 优雅清理 Redis 异步任务池与监控追踪资源
	if rc, ok := cacheService.(*redisinfra.RedisCache); ok {
		rc.Release()
	}

	if otelShutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(ctx); err != nil {
			zap.L().Error("OpenTelemetry flush error", zap.Error(err))
		}
	}

	zap.L().Info("apply_service exited gracefully")
}
