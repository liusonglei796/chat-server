package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	relationpb "kama_chat_server/api/gen/relation"
	"kama_chat_server/internal/config"
	mysqlimpl "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/grpc_client"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/service/apply"
	"kama_chat_server/internal/service/friendship"
	"kama_chat_server/internal/service/group"
	"kama_chat_server/internal/service/relation"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
	"kama_chat_server/pkg/outbox"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// 3. 初始化数据库
	repos := mysqlimpl.Init()

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	var cachePort repository.AsyncCacheService = cacheService

	// 5. 初始化 gRPC 客户端（跨服务调用 user_service 等）
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// 6. 初始化 Service
	friendshipSvc := friendship.NewFriendshipService(repos, cachePort)
	groupSvc := group.NewGroupService(repos, cachePort)
	applySvc := apply.NewApplyService(repos, cachePort)

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
		grpc.UnaryInterceptor(interceptor.ServerAuthInterceptor()),
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
	zap.L().Info("Relation Service stopped")
}
