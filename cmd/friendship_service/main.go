package main

import (
	"context"
	"log"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/logger"
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
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "friendship_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	// Step 2: 通过 Google Wire 构建应用及自动管理资源生命周期
	app, cleanup, err := wireApp(conf)
	if err != nil {
		zap.L().Fatal("wire app failed", zap.Error(err))
	}
	defer cleanup()

	zap.L().Info("Friendship Service (Pure gRPC Single Protocol) starting via Wire...")

	// Step 3: 阻塞运行应用直到接收到终止信号
	if err := app.Run(); err != nil {
		zap.L().Fatal("friendship service exited with error", zap.Error(err))
	}

	// Step 4: 优雅清理链路追踪资源
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("Friendship Service stopped successfully")
}
