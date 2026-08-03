package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	relationpb "kama_chat_server/api/gen/relation"
	"kama_chat_server/internal/common/config"
	mysqlimpl "kama_chat_server/internal/common/dao/mysql"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/domain/repository"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/logger"
	"kama_chat_server/internal/apps/relation/apply"
	"kama_chat_server/internal/apps/relation/friendship"
	"kama_chat_server/internal/apps/relation/group"
	"kama_chat_server/internal/apps/relation/relation"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
	otelinit "kama_chat_server/pkg/otel"
	"kama_chat_server/pkg/outbox"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// 2.5. 初始化 OpenTelemetry（如果启用），服务名固定为 relation_service
	var otelShutdown func(context.Context) error
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "relation_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", conf.OtelConfig.Endpoint),
			zap.String("serviceName", "relation_service"),
		)
	}

	// 3. 初始化数据库
	repos := mysqlimpl.InitFor("relation")

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	var cachePort repository.AsyncCacheService = cacheService

	// 5. 初始化 gRPC 客户端（跨服务调用 user_service 等）
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// 6. 初始化 Service
	friendshipSvc := friendship.NewFriendshipService(repos, cachePort)
	groupSvc := group.NewGroupService(repos, cachePort, repos.Outbox)
	applySvc := apply.NewApplyService(repos, cachePort, repos.Outbox)

	grpcServer := relation.NewGrpcServer(friendshipSvc, groupSvc, applySvc)

	// 启动 outbox 发布器，将本地事务中的领域事件投递到 Kafka
	outbox.NewPublisher(repos.Outbox, outbox.NewProducer()).Start()

	// 7. 启动 gRPC 服务
	port := 50053
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		zap.L().Fatal("failed to listen", zap.Error(err))
	}

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.ServerAuthInterceptor(),
			otelinit.ServerTraceInterceptor(),
		),
	)
	relationpb.RegisterRelationServiceServer(s, grpcServer)
	reflection.Register(s)

	// 7. 注册到 Etcd
	register, err := discovery.NewRegister([]string{"etcd:2379", "127.0.0.1:2379"}, discovery.ServerInfo{
		Name:   "relation_service",
		Addr:   fmt.Sprintf("relation-service:%d", port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Fatal("failed to register to etcd", zap.Error(err))
	}
	defer register.Stop()

	go func() {
		zap.L().Info("Relation Service started", zap.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			zap.L().Fatal("failed to serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down Relation Service...")
	s.GracefulStop()

	// 关闭 Redis 异步任务池，等待已提交任务完成
	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}

	// 关闭 OpenTelemetry TracerProvider，确保未导出的 span 被刷新
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("Relation Service stopped")
}
