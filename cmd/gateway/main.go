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

	"google.golang.org/grpc"
	"go.uber.org/zap"

	gatewaypb "kama_chat_server/api/gen/gateway"
	gatewaygrpc "kama_chat_server/internal/apps/gateway/grpc_server"
	"kama_chat_server/internal/apps/gateway/handler"
	"kama_chat_server/internal/apps/gateway/https_server"
	"kama_chat_server/internal/apps/message/chat"
	"kama_chat_server/internal/common/config"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/jwt"
	"kama_chat_server/internal/common/infrastructure/logger"
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

	// 3. 初始化数据库 (已移除，ChatServer不再直连MySQL)
	// stores := mysqlimpl.Init()
	// zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	var cachePort store.AsyncCacheService = cacheService
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)
	zap.L().Info("JWT 初始化成功")

	// 6. 初始化 Validator 国际化
	if err := handler.InitTrans("zh"); err != nil {
		zap.L().Fatal("validator 初始化失败", zap.Error(err))
	}
	zap.L().Info("Validator 国际化初始化成功")

	// 7. 解析网关 gRPC 监听与注册地址
	grpcPort := conf.MainConfig.GrpcPort
	if grpcPort == 0 {
		grpcPort = 8001
	}
	grpcAddr := conf.MainConfig.GrpcAddr
	if grpcAddr == "" {
		grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)
	}

	chatServer := chat.NewChatServer(cachePort, grpcAddr)
	zap.L().Info("ChatServer 初始化成功", zap.String("grpcAddr", grpcAddr))

	// 8. 初始化 gRPC Client
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})
	zap.L().Info("gRPC 客户端初始化成功")

	// 9. 初始化 Handler 层 (依赖注入，包含 ChatServer 的 ClientHub)
	handlers := handler.NewHandlers(chatServer.GetHub())
	zap.L().Info("Handler 层初始化成功")

	// 10. 初始化 HTTPS 服务器
	engine := https_server.Init(handlers, cachePort)
	zap.L().Info("HTTPS 服务器初始化成功")

	// 11. 启动网关内部 gRPC 服务（供后端微服务直推下行消息）
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
	gatewayServerImpl := gatewaygrpc.NewGatewayGrpcServer(chatServer.GetHub())
	gatewaypb.RegisterGatewayServiceServer(gatewayGrpcServer, gatewayServerImpl)

	go func() {
		zap.L().Info("Gateway gRPC Server started", zap.Int("port", grpcPort), zap.String("regAddr", grpcAddr))
		if err := gatewayGrpcServer.Serve(grpcLis); err != nil {
			zap.L().Fatal("gateway grpc serve error", zap.Error(err))
		}
	}()

	// 12. 启动服务
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	// 启动聊天服务器
	go chatServer.Run()

	go func() {
		// Ubuntu22.04云服务器部署
		// 运行 HTTP 服务
		if err := engine.Run(fmt.Sprintf("%s:%d", host, port)); err != nil {
			zap.L().Fatal("server running fault")
			return
		}
	}()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	<-quit

	zap.L().Info("关闭服务器...")

	// 优雅停机网关 gRPC 服务
	gatewayGrpcServer.GracefulStop()

	// 关闭聊天服务器
	chatServer.Shutdown()

	// 关闭 Redis 异步任务池，等待已提交任务完成
	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}

	// 关闭 OpenTelemetry TracerProvider，确保未导出的 span 被刷新
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("OpenTelemetry shutdown error", zap.Error(err))
		}
	}

	zap.L().Info("服务器已关闭")
}
