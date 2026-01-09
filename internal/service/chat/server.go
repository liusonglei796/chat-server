// Package chat 实现了聊天系统的核心服务层
// server.go
// 核心职责：聊天服务器聚合结构和依赖注入
// 封装 MessageBroker、KafkaClient 等组件，提供统一的生命周期管理
package chat

import (
	"context"
	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
)

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
	// GetMessageRepo 获取消息 Repository（供 ws_gateway 使用）
	GetMessageRepo() repository.MessageRepository
}

// ChatServer 聊天服务器聚合结构
// 封装所有聊天相关组件，通过依赖注入管理生命周期
type ChatServer struct {
	// Broker 消息代理，实现 MessageBroker 接口
	// 根据配置可能是 ChannelBroker 或 KafkaBroker
	Broker MessageBroker

	// KafkaClient Kafka 客户端（仅 Kafka 模式使用）
	KafkaClient *KafkaClient

	// messageRepo 消息 Repository
	messageRepo repository.MessageRepository

	// groupMemberRepo 群成员 Repository
	groupMemberRepo repository.GroupMemberRepository

	// cacheService 缓存服务
	cacheService myredis.AsyncCacheService

	// mode 运行模式: "channel" 或 "kafka"
	mode string
}

// ChatServerConfig 聊天服务器配置
type ChatServerConfig struct {
	Mode            string // "channel" 或 "kafka"
	MessageRepo     repository.MessageRepository
	GroupMemberRepo repository.GroupMemberRepository
	CacheService    myredis.AsyncCacheService
	KafkaHostPort   string
	KafkaTopic      string
}

// NewChatServer 创建聊天服务器实例
// 根据配置选择 ChannelBroker 或 KafkaBroker
func NewChatServer(cfg ChatServerConfig) *ChatServer {
	cs := &ChatServer{
		messageRepo:     cfg.MessageRepo,
		groupMemberRepo: cfg.GroupMemberRepo,
		cacheService:    cfg.CacheService,
		mode:            cfg.Mode,
	}

	if cfg.Mode == "kafka" {
		// Kafka 模式
		cs.KafkaClient = NewKafkaClient()
		kafkaBroker := NewMsgConsumer(cs.KafkaClient, cs.messageRepo, cs.groupMemberRepo, cs.cacheService)
		cs.Broker = kafkaBroker
	} else {
		// Channel 模式（默认）
		channelBroker := NewStandaloneServer(cs.messageRepo, cs.groupMemberRepo, cs.cacheService)
		cs.Broker = channelBroker
	}

	return cs
}

// InitKafka 初始化 Kafka 连接（仅 Kafka 模式需要调用）
func (cs *ChatServer) InitKafka() {
	if cs.KafkaClient != nil {
		cs.KafkaClient.KafkaInit()
	}
}

// Start 启动聊天服务器
func (cs *ChatServer) Start() {
	cs.Broker.Start()
}

// Close 关闭聊天服务器
func (cs *ChatServer) Close() {
	cs.Broker.Close()
	if cs.KafkaClient != nil {
		cs.KafkaClient.KafkaClose()
	}
}

// GetBroker 获取消息代理
func (cs *ChatServer) GetBroker() MessageBroker {
	return cs.Broker
}
