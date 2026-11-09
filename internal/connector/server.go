package connector

import cacheinfra "kama_chat_server/internal/infra/cache"

// ConnectorServer 长连接状态中枢与生命周期管理器（非网络端口监听服务）。
// 负责托管 ConnHub 实例、启动 30s 惰性心跳清理巡检及连接服务写协程池生命周期。
// 真正的网络监听端口由 HttpServer (8000) 和 GrpcServerWrapper (8001) 承载。
type ConnectorServer struct {
	Hub *ConnHub
}

// NewConnectorServer 创建长连接状态管理器实例
func NewConnectorServer(cache cacheinfra.AsyncCacheService, connGrpcAddr string) *ConnectorServer {
	return &ConnectorServer{
		Hub: NewConnHub(cache, connGrpcAddr),
	}
}

func (cs *ConnectorServer) Run() {
	cs.Hub.Start()
}

func (cs *ConnectorServer) Shutdown() {
	cs.Hub.Close()
}

func (cs *ConnectorServer) GetHub() *ConnHub {
	return cs.Hub
}
