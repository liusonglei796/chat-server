package gateway

import (
	"fmt"

	"github.com/google/wire"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
)

// ProviderSet 集中导出 gateway 领域内长连接与下行推送服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideChatServer,
	ProvideConnHub,
	ProvideWsServer,
	NewGatewayGrpcServer,
)

// ProvideChatServer 提供 ChatServer 实例
func ProvideChatServer(cache cacheinfra.AsyncCacheService, conf *config.Config) *ChatServer {
	grpcPort := conf.MainConfig.GrpcPort
	if grpcPort == 0 {
		grpcPort = 8001
	}
	grpcAddr := conf.MainConfig.GrpcAddr
	if grpcAddr == "" {
		grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)
	}
	return NewChatServer(cache, grpcAddr)
}

// ProvideConnHub 从 ChatServer 获取 ConnHub 实例
func ProvideConnHub(cs *ChatServer) *ConnHub {
	return cs.GetHub()
}

// ProvideWsServer 提供原生 WebSocket HTTP 监听服务实例
func ProvideWsServer(chatServer *ChatServer, conf *config.Config) *WsServer {
	httpAddr := fmt.Sprintf("%s:%d", conf.MainConfig.Host, conf.MainConfig.Port)
	return NewWsServer(httpAddr, chatServer.GetHub())
}
