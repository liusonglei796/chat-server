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

	authpb "kama_chat_server/api/gen/auth"
	"kama_chat_server/internal/config"
	mysqlimpl "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/infrastructure/jwt"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/service/auth"
	"kama_chat_server/internal/service/user"
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

	// 初始化 JWT（因为登录会签发 JWT）
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)

	// 5. 初始化 Service
	userSvc := user.NewUserService(repos, cachePort)
	authSvc := auth.NewAuthService(cachePort, repos.User)
	grpcServer := auth.NewGrpcServer(authSvc, userSvc)

	// 6. 启动 gRPC 服务
	port := 50052
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		zap.L().Fatal("failed to listen", zap.Error(err))
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerAuthInterceptor()),
	)
	authpb.RegisterAuthServiceServer(s, grpcServer)
	reflection.Register(s)

	// 7. 注册到 Etcd
	register, err := discovery.NewRegister([]string{"etcd:2379", "127.0.0.1:2379"}, discovery.ServerInfo{
		Name:   "auth_service",
		Addr:   fmt.Sprintf("auth_service:%d", port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Fatal("failed to register to etcd", zap.Error(err))
	}
	defer register.Stop()

	go func() {
		zap.L().Info("Auth Service started", zap.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			zap.L().Fatal("failed to serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down Auth Service...")
	s.GracefulStop()
	zap.L().Info("Auth Service stopped")
}
