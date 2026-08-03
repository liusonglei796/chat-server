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

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/config"
	mysqlimpl "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/grpc_client"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/service/message"
	"kama_chat_server/internal/service/session"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
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

	// 5. 初始化 gRPC 客户端（跨服务调用 user_service/relation_service 等）
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// 6. 初始化 Service
	// 此时暂时传入 nil 给 pushRecallNotify，如果是真实微服务，需要通过 Kafka 给 ChatServer 发送撤回消息通知
	// 或者稍后我们再修改此处
	msgSvc := message.NewMessageService(repos.Message, repos.Friendship, repos.Session, cachePort, nil)
	sessionSvc := session.NewSessionService(repos.Session, repos.User, repos.Group, repos.GroupMember, repos.Friendship, repos.Message, cachePort)

	// 初始化 KafkaProcessor
	kafkaProcessor := message.NewKafkaProcessor(
		repos.Message,
		repos.Friendship,
		repos.GroupMember,
		repos.Session,
		cachePort,
		repos.User,
	)
	kafkaProcessor.Start()
	defer kafkaProcessor.Close()

	// 注入 recall notify (通过 KafkaProcessor 发送撤回通知)
	msgSvc.SetPushRecallNotify(kafkaProcessor.PushRecallNotify)

	grpcServer := message.NewGrpcServer(msgSvc, sessionSvc)

	// 7. 启动 gRPC 服务
	port := 50054
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		zap.L().Fatal("failed to listen", zap.Error(err))
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerAuthInterceptor()),
	)
	messagepb.RegisterMessageServiceServer(s, grpcServer)
	reflection.Register(s)

	// 8. 注册到 Etcd
	register, err := discovery.NewRegister([]string{"etcd:2379", "127.0.0.1:2379"}, discovery.ServerInfo{
		Name:   "message_service",
		Addr:   fmt.Sprintf("message-service:%d", port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Fatal("failed to register to etcd", zap.Error(err))
	}
	defer register.Stop()

	go func() {
		zap.L().Info("Message Service started", zap.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			zap.L().Fatal("failed to serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down Message Service...")
	s.GracefulStop()

	// 关闭 Redis 异步任务池，等待已提交任务完成
	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}
	zap.L().Info("Message Service stopped")
}
