package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/internal/infra/logger"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// 3. 初始化 OpenTelemetry（如果启用）
	var otelShutdown func(context.Context) error
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, conf.OtelConfig.ServiceName)
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", conf.OtelConfig.Endpoint),
			zap.String("serviceName", conf.OtelConfig.ServiceName),
		)
	}

	// 4. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)

	// 5. 使用 Google Wire 编织并装配 GatewayApp
	app, cleanup, err := wireApp(conf)
	if err != nil {
		zap.L().Fatal("wire app failed", zap.Error(err))
	}
	defer cleanup()

	// 6. 启动内部 gRPC 服务（供微服务直推下行消息）
	go func() {
		zap.L().Info("Gateway gRPC Server started", zap.Int("port", app.GrpcWrap.Port), zap.String("regAddr", app.GrpcWrap.Addr))
		if err := app.GrpcWrap.Server.Serve(app.GrpcWrap.Listener); err != nil {
			zap.L().Fatal("gateway grpc serve error", zap.Error(err))
		}
	}()

	// 7. 启动长连接 Hub 广播与分发监听
	go app.ChatServer.Run()

	// 8. 启动专职 WebSocket 原生 HTTP 监听服务
	go func() {
		if err := app.WsServer.Start(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("ws server running failed", zap.Error(err))
		}
	}()

	// 9. 监听优雅退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("正在优雅关闭连接中枢服务器...")

	// 优雅停机 HTTP WebSocket 监听服务 (5秒超时)
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer httpCancel()
	if err := app.WsServer.Shutdown(httpShutdownCtx); err != nil {
		zap.L().Error("ws server shutdown error", zap.Error(err))
	}

	// 关闭聊天长连接池
	app.ChatServer.Shutdown()

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
