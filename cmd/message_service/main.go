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

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/apps/message/message"
	"kama_chat_server/internal/apps/message/session"
	"kama_chat_server/internal/common/config"
	mysqlimpl "kama_chat_server/internal/common/dao/mysql"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/logger"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// 2.5. 初始化 OpenTelemetry（如果启用），服务名固定为 message_service
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

	// 3. 初始化数据库
	stores := mysqlimpl.InitFor("message")

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	var cachePort store.AsyncCacheService = cacheService

	// 5. 初始化 gRPC 客户端（跨服务调用 user_service/relation_service 等）
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// 6. 初始化 Service
	// 此时暂时传入 nil 给 pushRecallNotify，如果是真实微服务，需要通过 Kafka 给 ChatServer 发送撤回消息通知
	// 或者稍后我们再修改此处
	msgSvc := message.NewMessageService(stores.Message, stores.Session, cachePort, nil)
	sessionSvc := session.NewSessionService(stores.Session, stores.Message, cachePort)

	// 初始化 KafkaProcessor
	kafkaProcessor := message.NewKafkaProcessor(
		stores.Message,
		stores.Session,
		cachePort,
	)
	kafkaProcessor.Start()
	defer kafkaProcessor.Close()

	// 初始化领域事件消费者（消费 outbox 发布的事件，维护本地 session 冗余字段）
	// 先确保 domain_events 主题存在，避免消费者在主题创建前加入消费组而被分配 0 个分区
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Fatal("failed to ensure domain_events topic", zap.Error(err))
	}
	eventHandler := message.NewSessionEventHandler(stores.Session)
	eventConsumer := message.NewDomainEventConsumer(eventHandler)
	eventConsumer.Start()
	defer eventConsumer.Close()

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
		grpc.ChainUnaryInterceptor(
			interceptor.ServerAuthInterceptor(),
			otelinit.ServerTraceInterceptor(),
		),
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

	// 关闭 OpenTelemetry TracerProvider，确保未导出的 span 被刷新
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("Message Service stopped")
}
