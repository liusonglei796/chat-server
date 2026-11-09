package connector

import (
	"context"
	"fmt"

	"github.com/google/wire"
	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/kafka"
)

// ProviderSet 集中导出 connector 领域内长连接与下行推送服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideIncomingMessageProducer,
	ProvideConnectorServer,
	ProvideConnHub,
	ProvideHttpServer,
	NewConnectorGrpcServer,
)

// ProvideIncomingMessageProducer 初始化上行聊天消息 Kafka 削峰生产者并导出清理函数
func ProvideIncomingMessageProducer() (IncomingMessageProducer, func()) {
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicChatMessagesIncoming); err != nil {
		zap.L().Warn("failed to ensure chat_messages_incoming topic for producer", zap.Error(err))
	}
	producer := NewKafkaIncomingMessageProducer()
	cleanup := func() {
		producer.Close()
	}
	return producer, cleanup
}

// ProvideConnectorServer 提供 ConnectorServer 实例（长连接状态中枢与生命周期管理器，非网络端口监听服务）
func ProvideConnectorServer(cache cacheinfra.AsyncCacheService, conf *config.Config, incomingProducer IncomingMessageProducer) *ConnectorServer {
	grpcPort := conf.MainConfig.GrpcPort
	if grpcPort == 0 {
		grpcPort = 8001
	}
	grpcAddr := conf.MainConfig.GrpcAddr
	if grpcAddr == "" {
		grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)
	}
	srv := NewConnectorServer(cache, grpcAddr)
	srv.GetHub().SetIncomingProducer(incomingProducer)
	return srv
}

// ProvideConnHub 从 ConnectorServer 获取 ConnHub 实例
func ProvideConnHub(cs *ConnectorServer) *ConnHub {
	return cs.GetHub()
}

// ProvideHttpServer 提供专职 WebSocket 握手升级与探活的 HTTP 监听服务实例 (8000 端口)
func ProvideHttpServer(server *ConnectorServer, conf *config.Config) *HttpServer {
	httpAddr := fmt.Sprintf("%s:%d", conf.MainConfig.Host, conf.MainConfig.Port)
	return NewHttpServer(httpAddr, server.GetHub())
}
