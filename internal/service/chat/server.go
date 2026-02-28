// Package chat 实现了聊天系统的核心服务层
// server.go
// 核心职责：聊天服务器聚合结构和依赖注入
// 封装 KafkaClient 等组件，提供统一的生命周期管理
package chat

import (
	"kama_chat_server/internal/service/mysqlinterface"
	redisinterface "kama_chat_server/internal/service/redisinterface"
	"strings"
)

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
	// Broker 消息消费者（基于 Kafka）
	Broker *MsgConsumer

	// KafkaClient Kafka 客户端
	KafkaClient *KafkaClient

	// messageRepo 消息 Repository
	messageRepo mysqlinterface.MessageRepository

	// friendshipRepo 好友关系 Repository（用于消息权限校验）
	friendshipRepo mysqlinterface.FriendshipRepository

	// groupMemberRepo 群成员 Repository
	groupMemberRepo mysqlinterface.GroupMemberRepository

	// sessionRepo 会话 Repository（用于更新最后消息）
	sessionRepo mysqlinterface.SessionRepository

	// cacheService 缓存服务
	cacheService redisinterface.AsyncCacheService

	// userRepo 用户 Repository（用于检查用户状态，如是否被禁用）
	userRepo mysqlinterface.UserRepository
}

// ChatServerConfig 聊天服务器配置
type ChatServerConfig struct {
	MessageRepo     mysqlinterface.MessageRepository
	FriendshipRepo  mysqlinterface.FriendshipRepository
	GroupMemberRepo mysqlinterface.GroupMemberRepository
	SessionRepo     mysqlinterface.SessionRepository
	CacheService    redisinterface.AsyncCacheService
	UserRepo        mysqlinterface.UserRepository // 新增：用户仓库，用于消息发送权限校验
}

// NewChatServer 创建聊天服务器实例
func NewChatServer(chatServerCfg ChatServerConfig) *ChatServer {
	cs := &ChatServer{
		messageRepo:     chatServerCfg.MessageRepo,
		friendshipRepo:  chatServerCfg.FriendshipRepo,
		groupMemberRepo: chatServerCfg.GroupMemberRepo,
		sessionRepo:     chatServerCfg.SessionRepo,
		cacheService:    chatServerCfg.CacheService,
		userRepo:        chatServerCfg.UserRepo, // 新增：用户仓库
	}

	// 初始化 Kafka 客户端和消费者
	cs.KafkaClient = NewKafkaClient()
	// 新增：传递 userRepo 用于消息发送权限校验（检查用户是否被禁用）
	cs.Broker = NewMsgConsumer(cs.KafkaClient, cs.messageRepo, cs.friendshipRepo, cs.groupMemberRepo, cs.sessionRepo, cs.cacheService, cs.userRepo)

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

// GetBroker 获取消息消费者
func (cs *ChatServer) GetBroker() *MsgConsumer {
	return cs.Broker
}
