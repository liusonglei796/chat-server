// Package chat 实现了聊天系统的核心服务层
// server.go
// 核心职责：聊天服务器聚合结构和依赖注入
// 封装 MessageBroker、KafkaClient 等组件，提供统一的生命周期管理
package chat

import (
	"context"
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"strings"
)

// MessageBroker 定义消息代理接口
// 实现：MsgConsumer (基于 Kafka 的分布式模式)
type MessageBroker interface {
	// Publish 发布消息到 Kafka
	Publish(ctx context.Context, msg []byte) error
	// RegisterClient 注册客户端连接
	RegisterClient(client *UserConn)
	// UnregisterClient 注销客户端连接
	UnregisterClient(client *UserConn)
	// GetClient 获取指定用户的连接
	GetClient(userId string) *UserConn
	// KickClient 向指定用户推送下线通知并断开连接（单点登录互踢）
	KickClient(userId string, reason string)
	// Start 启动消息消费循环
	Start()
	// Close 关闭代理资源
	Close()
	// GetMessageRepo 获取消息 Repository（供 ws_gateway 使用）
	GetMessageRepo() mysql.MessageRepository
}

// normalizePath 将完整 URL 转换为相对路径
// 例如: https://127.0.0.1:8000/static/xxx -> /static/xxx
// 特殊处理: 保留 elemecdn 的默认头像链接
func normalizePath(path string) string {
	// 特殊处理默认头像（如果是远程链接且不含 /static/ 则原样返回）
	if strings.HasPrefix(path, "https://cube.elemecdn.com") {
		return path
	}
	// 查找 "/static/" 的位置
	idx := strings.Index(path, "/static/")
	// 如果没找到 "/static/"，说明不是本地静态资源路径，直接返回原路径
	if idx == -1 {
		return path
	}
	// 返回从 "/static/" 开始的子串，即相对路径
	return path[idx:]
}

// ChatServer 聊天服务器聚合结构
// 封装所有聊天相关组件，通过依赖注入管理生命周期
type ChatServer struct {
	// Broker 消息代理接口（MsgConsumer 实现）
	Broker MessageBroker

	// KafkaClient Kafka 客户端
	KafkaClient *KafkaClient

	// messageRepo 消息 Repository
	messageRepo mysql.MessageRepository

	// friendshipRepo 好友关系 Repository（用于消息权限校验）
	friendshipRepo mysql.FriendshipRepository

	// groupMemberRepo 群成员 Repository
	groupMemberRepo mysql.GroupMemberRepository

	// sessionRepo 会话 Repository（用于更新最后消息）
	sessionRepo mysql.SessionRepository

	// cacheService 缓存服务
	cacheService myredis.AsyncCacheService
}

// ChatServerConfig 聊天服务器配置
type ChatServerConfig struct {
	MessageRepo     mysql.MessageRepository
	FriendshipRepo  mysql.FriendshipRepository
	GroupMemberRepo mysql.GroupMemberRepository
	SessionRepo     mysql.SessionRepository
	CacheService    myredis.AsyncCacheService
}

// NewChatServer 创建聊天服务器实例
func NewChatServer(cfg ChatServerConfig) *ChatServer {
	cs := &ChatServer{
		messageRepo:     cfg.MessageRepo,
		friendshipRepo:  cfg.FriendshipRepo,
		groupMemberRepo: cfg.GroupMemberRepo,
		sessionRepo:     cfg.SessionRepo,
		cacheService:    cfg.CacheService,
	}

	// 初始化 Kafka 客户端和消费者
	cs.KafkaClient = NewKafkaClient()
	cs.Broker = NewMsgConsumer(cs.KafkaClient, cs.messageRepo, cs.friendshipRepo, cs.groupMemberRepo, cs.sessionRepo, cs.cacheService)

	return cs
}

// InitKafka 初始化 Kafka 连接
func (cs *ChatServer) InitKafka() {
	cs.KafkaClient.KafkaInit()
}

// Run 启动聊天服务器
func (cs *ChatServer) Run() {
	cs.Broker.Start()
}

// Shutdown 关闭聊天服务器
func (cs *ChatServer) Shutdown() {
	cs.Broker.Close()
	if cs.KafkaClient != nil {
		cs.KafkaClient.KafkaClose()
	}
}

// GetBroker 获取消息代理
func (cs *ChatServer) GetBroker() MessageBroker {
	return cs.Broker
}
