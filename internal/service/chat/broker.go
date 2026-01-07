// Package chat 实现了聊天系统的核心服务层
// broker.go
// 核心职责：定义消息代理接口
// 抽象消息发布和客户端管理，支持 Kafka 和 Channel 两种实现
package chat

import "context"

// MessageBroker 定义消息代理接口
// 支持多种实现：KafkaBroker (分布式), ChannelBroker (单机)
type MessageBroker interface {
	// Publish 发布消息到消息队列/通道
	Publish(ctx context.Context, msg []byte) error
	// RegisterClient 注册客户端连接
	RegisterClient(client *UserConn)
	// UnregisterClient 注销客户端连接
	UnregisterClient(client *UserConn)
	// GetClient 获取指定用户的连接
	GetClient(userId string) *UserConn
	// Start 启动消息消费循环
	Start()
	// Close 关闭代理资源
	Close()
}

// GlobalBroker 全局消息代理实例
// 在 main.go 中根据配置初始化为 KafkaBroker 或 ChannelBroker
var GlobalBroker MessageBroker
