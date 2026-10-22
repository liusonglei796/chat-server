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

	authpb "kama_chat_server/api/gen/auth"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/internal/infra/logger"
	"kama_chat_server/internal/infra/outbox"
	redisinfra "kama_chat_server/internal/infra/redis"
	"kama_chat_server/internal/user"
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
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "user_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", conf.OtelConfig.Endpoint),
			zap.String("serviceName", "user_service"),
		)
	}

	// Step 2.1: 初始化持久化数据层连接
	db, err := database.Open(&conf.MysqlConfig, "chat_user")
	if err != nil {
		zap.L().Fatal("failed to connect chat_user database", zap.Error(err))
	}
	if err := database.AutoMigrate(db, &user.UserInfo{}, &outbox.Outbox{}); err != nil {
		zap.L().Fatal("failed to migrate chat_user database", zap.Error(err))
	}
	uow := user.NewUoW(db)

	// Step 2.2: 初始化 JWT 编解码密钥与有效期
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)

	// Step 2.3: 初始化 Redis 异步缓存服务
	cacheService := redisinfra.Init()

	// Step 3.1: 初始化业务领域服务与处理器实现
	userSvc := user.NewUserService(uow, cacheService)
	userServerImpl := user.NewUserGrpcServer(userSvc)
	authSvc := user.NewAuthService(cacheService, uow.UserStore())
	authServerImpl := user.NewAuthGrpcServer(authSvc, userSvc)

	// Step 4.1: 构建 Kratos 纯 gRPC 传输服务
	grpcPort := 50051
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
	userpb.RegisterUserServiceServer(grpcSrv, userServerImpl)
	authpb.RegisterAuthServiceServer(grpcSrv, authServerImpl)

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
		Name:   "user_service",
		Addr:   fmt.Sprintf("user-service:%d", grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	} else {
		defer legacyRegister.Stop()
	}

	// Step 5.1: 组装并启动 Kratos 应用
	app := kratos.New(
		kratos.Name("user_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(
			grpcSrv,
		),
		kratos.Registrar(kratosRegistrar),
	)

	zap.L().Info("User Service (Pure gRPC Single Protocol) starting...",
		zap.String("grpcAddr", grpcAddr),
	)

	// Step 5.2: 阻塞运行应用直到接收到终止信号
	if err := app.Run(); err != nil {
		zap.L().Fatal("user service exited with error", zap.Error(err))
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
	zap.L().Info("User Service stopped successfully")
}
