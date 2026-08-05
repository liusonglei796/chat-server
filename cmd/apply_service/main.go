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

	applypb "kama_chat_server/api/gen/apply"
	"kama_chat_server/internal/apps/apply"
	"kama_chat_server/internal/common/config"
	mysqlimpl "kama_chat_server/internal/common/dao/mysql"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/logger"
	outbox "kama_chat_server/internal/common/infrastructure/outbox"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	conf := config.GetConfig()

	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	var otelShutdown func(context.Context) error
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "apply_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	stores := mysqlimpl.InitFor("apply")
	cacheService := myredis.Init()
	var cachePort store.AsyncCacheService = cacheService

	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	applySvc := apply.NewApplyService(stores, cachePort)

	outbox.NewPublisher(stores.Outbox, kafka.NewProducer(kafka.TopicDomainEvents)).Start()

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
	applypb.RegisterApplyServiceServer(s, apply.NewGrpcServer(applySvc))
	reflection.Register(s)

	register, err := discovery.NewRegister([]string{"etcd:2379", "127.0.0.1:2379"}, discovery.ServerInfo{
		Name:   "apply_service",
		Addr:   fmt.Sprintf("apply_service:%d", port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Fatal("failed to register to etcd", zap.Error(err))
	}
	defer register.Stop()

	go func() {
		zap.L().Info("apply_service started", zap.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			zap.L().Fatal("failed to serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down apply_service...")
	s.GracefulStop()

	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}

	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("apply_service stopped")
}
