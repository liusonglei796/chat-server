package chat

import "kama_chat_server/internal/common/domain/store"

// ChatServer 聊天网关长连接服务聚合结构
type ChatServer struct {
	Hub *ClientHub
}

// NewChatServer 创建聊天网关实例
func NewChatServer(cache store.AsyncCacheService, gatewayGrpcAddr string) *ChatServer {
	return &ChatServer{
		Hub: NewClientHub(cache, gatewayGrpcAddr),
	}
}

func (cs *ChatServer) Run() {
	cs.Hub.Start()
}

func (cs *ChatServer) Shutdown() {
	cs.Hub.Close()
}

func (cs *ChatServer) GetHub() *ClientHub {
	return cs.Hub
}

// Deprecated: 使用 GetHub 代替
func (cs *ChatServer) GetBroker() *ClientHub {
	return cs.Hub
}
