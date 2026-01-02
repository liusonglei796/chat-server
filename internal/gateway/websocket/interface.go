package websocket

import "context"

// MessageWriter Kafka 消息写入接口
// 用于解耦 websocket 包对 mq 包的依赖
type MessageWriter interface {
	WriteMessage(ctx context.Context, key, value []byte) error
}

// ClientManager 客户端登录登出管理接口
// 用于解耦 websocket 包对 mq 包的依赖 (Kafka模式)
type ClientManager interface {
	SendClientToLogin(client *Client)
	SendClientToLogout(client *Client)
	GetClient(uuid string) *Client
}

// 存储注入的实现
var messageWriter MessageWriter
var clientManager ClientManager

// SetMessageWriter 注入 MessageWriter 实现
func SetMessageWriter(writer MessageWriter) {
	messageWriter = writer
}

// GetMessageWriter 获取 MessageWriter 实现
func GetMessageWriter() MessageWriter {
	return messageWriter
}

// SetClientManager 注入 ClientManager 实现
func SetClientManager(manager ClientManager) {
	clientManager = manager
}

// GetClientManager 获取 ClientManager 实现
func GetClientManager() ClientManager {
	return clientManager
}
