package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/internal/gateway"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/internal/infra/logger"
	"kama_chat_server/internal/infra/redis"
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
	zap.L().Info("日志初始化成功")

	// 2.5. 初始化 OpenTelemetry（如果启用）
	var otelShutdown func(context.Context) error
	otelCfg := conf.OtelConfig
	if otelCfg.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), otelCfg.Endpoint, otelCfg.ServiceName)
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", otelCfg.Endpoint),
			zap.String("serviceName", otelCfg.ServiceName),
		)
	}

	// 3. 初始化 Redis 缓存
	cacheService := redis.Init()
	zap.L().Info("Redis 初始化成功")

	// 4. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)
	zap.L().Info("JWT 初始化成功")

	// 5. 解析网关 gRPC 监听与注册地址
	grpcPort := conf.MainConfig.GrpcPort
	if grpcPort == 0 {
		grpcPort = 8001
	}
	grpcAddr := conf.MainConfig.GrpcAddr
	if grpcAddr == "" {
		grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)
	}

	chatServer := gateway.NewChatServer(cacheService, grpcAddr)
	zap.L().Info("ChatServer 初始化成功", zap.String("grpcAddr", grpcAddr))

	// 6. 初始化 gRPC Client (用于 WebSocket 上行发消息时转发至后端微服务)
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})
	zap.L().Info("gRPC 客户端初始化成功")

	// 7. 启动网关内部 gRPC 服务（供后端微服务直推下行消息）
	grpcLis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", grpcPort))
	if err != nil {
		zap.L().Fatal("failed to listen on gateway grpc port", zap.Error(err))
	}
	gatewayGrpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.ServerAuthInterceptor(),
			otelinit.ServerTraceInterceptor(),
		),
	)
	gatewayServerImpl := gateway.NewGatewayGrpcServer(chatServer.GetHub())
	gatewaypb.RegisterGatewayServiceServer(gatewayGrpcServer, gatewayServerImpl)

	go func() {
		zap.L().Info("Gateway gRPC Server started", zap.Int("port", grpcPort), zap.String("regAddr", grpcAddr))
		if err := gatewayGrpcServer.Serve(grpcLis); err != nil {
			zap.L().Fatal("gateway grpc serve error", zap.Error(err))
		}
	}()

	// 8. 启动长连接 Hub 广播与分发监听
	go chatServer.Run()

	// 9. 启动专职 WebSocket 原生 HTTP 监听服务
	httpAddr := fmt.Sprintf("%s:%d", conf.MainConfig.Host, conf.MainConfig.Port)
	wsServer := gateway.NewWsServer(httpAddr, chatServer.GetHub())

	go func() {
		if err := wsServer.Start(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("ws server running failed", zap.Error(err))
		}
	}()

	// 10. 监听优雅退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("正在优雅关闭连接中枢服务器...")

	// 优雅停机 HTTP WebSocket 监听服务 (5秒超时)
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer httpCancel()
	if err := wsServer.Shutdown(httpShutdownCtx); err != nil {
		zap.L().Error("ws server shutdown error", zap.Error(err))
	}

	// 优雅停机内部 gRPC 服务
	gatewayGrpcServer.GracefulStop()

	// 关闭聊天长连接池
	chatServer.Shutdown()

	// 关闭 Redis 异步任务池，等待已提交任务完成
	if rc, ok := cacheService.(*redis.RedisCache); ok {
		rc.Release()
	}

	// 关闭 OpenTelemetry TracerProvider
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("OpenTelemetry shutdown error", zap.Error(err))
		}
	}

	zap.L().Info("连接中枢服务器已安全关闭")
}
