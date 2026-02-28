# 18. Kafka 集成与消息模式

> 本教程介绍 Kafka 消息队列的集成，实现异步消息处理，解耦消息的生产与消费。

---

## 📌 学习目标

- 掌握 `*MsgConsumer` 设计与依赖注入
- 实现消息生产者与消费者
- 理解 MsgConsumer 的架构设计

---

## 1. 消息模式说明

> 注意：本项目**只支持 Kafka 模式**，已移除 Channel 模式。

**Kafka 模式**：
- **机制**：使用 Kafka 作为消息中间件进行异步解耦
- **数据流**：WebSocket Client → Kafka Producer → Kafka Broker → MsgConsumer → 消息处理 → DB/WebSocket
- **优点**：高吞吐、持久化、削峰填谷、支持集群扩容、消息可追溯
- **缺点**：架构复杂、依赖外部组件、需要运维 Kafka 集群
- **适用**：生产环境、分布式集群、高并发场景

---

## 2. 项目结构

```
internal/service/chat/
├── server.go          # ChatServer 聚合结构 + MsgConsumer
├── ws_gateway.go      # WebSocket 连接管理 (UserConn)
├── kafka_broker.go    # MsgConsumer (Kafka 模式)
└── kafka_client.go    # Kafka 客户端封装
```

---

## 3. 安装依赖

```bash
go get github.com/segmentio/kafka-go
```

---

## 4. MsgConsumer 设计

### 4.1 internal/service/chat/kafka_broker.go

```go
package chat

import (
	"context"
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
)

// MsgConsumer 基于 Kafka 的消息消费者
type MsgConsumer struct {
	Clients         sync.Map              // 在线客户端映射表
	Login           chan *UserConn        // 客户端登录通道
	Logout          chan *UserConn        // 客户端登出通道
	kafkaClient     *KafkaClient          // Kafka 客户端
	messageRepo     mysql.MessageRepository
	friendshipRepo  mysql.FriendshipRepository
	groupMemberRepo mysql.GroupMemberRepository
	sessionRepo     mysql.SessionRepository
	cacheService    myredis.AsyncCacheService
	userRepo        mysql.UserRepository
}

// Publish 发布消息到 Kafka
func (k *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
	return k.kafkaClient.SendMessage(ctx, []byte("0"), msg)
}

// GetMessageRepo 获取消息 Repository
func (k *MsgConsumer) GetMessageRepo() mysql.MessageRepository {
	return k.messageRepo
}
```

### 4.2 ChatServer 结构

```go
// ChatServer 聊天服务器聚合结构
type ChatServer struct {
	Broker          *MsgConsumer
	KafkaClient     *KafkaClient
	messageRepo     mysql.MessageRepository
	friendshipRepo  mysql.FriendshipRepository
	groupMemberRepo mysql.GroupMemberRepository
	sessionRepo     mysql.SessionRepository
	cacheService    myredis.AsyncCacheService
	userRepo        mysql.UserRepository
}

// ChatServerConfig 聊天服务器配置
type ChatServerConfig struct {
	MessageRepo     mysql.MessageRepository
	FriendshipRepo  mysql.FriendshipRepository
	GroupMemberRepo mysql.GroupMemberRepository
	SessionRepo     mysql.SessionRepository
	CacheService    myredis.AsyncCacheService
	UserRepo        mysql.UserRepository
}

// NewChatServer 创建聊天服务器实例
func NewChatServer(chatServerCfg ChatServerConfig) *ChatServer {
	cs := &ChatServer{
		messageRepo:     chatServerCfg.MessageRepo,
		friendshipRepo:  chatServerCfg.FriendshipRepo,
		groupMemberRepo: chatServerCfg.GroupMemberRepo,
		sessionRepo:     chatServerCfg.SessionRepo,
		cacheService:    chatServerCfg.CacheService,
		userRepo:        chatServerCfg.UserRepo,
	}

	// 初始化 Kafka 客户端和消费者
	cs.KafkaClient = NewKafkaClient()
	cs.Broker = NewMsgConsumer(
		cs.KafkaClient,
		cs.messageRepo,
		cs.friendshipRepo,
		cs.groupMemberRepo,
		cs.sessionRepo,
		cs.cacheService,
		cs.userRepo,
	)

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
```

---

## 5. Kafka 客户端封装

### 5.1 internal/service/chat/kafka_client.go

```go
package chat

import (
	"context"
	myconfig "kama_chat_server/internal/config"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaClient Kafka 客户端结构
type KafkaClient struct {
	Producer  *kafka.Writer // 生产者
	Reader    *kafka.Reader // 消费者
	KafkaConn *kafka.Conn   // 连接管理
}

// NewKafkaClient 创建 Kafka 客户端实例
func NewKafkaClient() *KafkaClient {
	return &KafkaClient{}
}

// KafkaInit 初始化 Kafka 客户端
func (k *KafkaClient) KafkaInit() {
	kafkaConfig := myconfig.GetConfig().KafkaConfig
	
	k.Producer = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  kafkaConfig.ChatTopic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: false,
	}
	
	k.Reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          kafkaConfig.ChatTopic,
		CommitInterval: time.Second,
		GroupID:        "chat-consumer-group",
		StartOffset:    kafka.LastOffset,
	})
}

func (k *KafkaClient) KafkaClose() {
	if err := k.Producer.Close(); err != nil {
		zap.L().Error(err.Error())
	}
	if err := k.Reader.Close(); err != nil {
		zap.L().Error(err.Error())
	}
}

// WriteMessage 向 Kafka 发送消息
func (k *KafkaClient) WriteMessage(ctx context.Context, value []byte) error {
	return k.Producer.WriteMessages(ctx, kafka.Message{
		Value: value,
	})
}
```

---

## 6. WebSocket 网关

### 6.1 internal/service/chat/ws_gateway.go

```go
package chat

import (
	"context"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MessageBack 用于回传消息给前端
type MessageBack struct {
	Message []byte
	Uuid    string
}

// UserConn 表示一个 WebSocket 客户端连接
type UserConn struct {
	Conn        *websocket.Conn
	Uuid        string
	SendBack    chan *MessageBack // 给前端
	broker      *MsgConsumer     // 注入的消息消费者
	cleanupOnce sync.Once         // 确保 cleanup 只执行一次
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

var ctx = context.Background()

// Read 从 WebSocket 读取消息并通过 Broker 发布
func (c *UserConn) Read() {
	defer c.cleanup()
	
	// 设置心跳超时
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				zap.L().Error("ws read error", zap.String("userId", c.Uuid), zap.Error(err))
			}
			return
		}
		
		// 安全：由服务端注入 send_id，防止 IDOR 攻击
		securedMessage := injectSenderId(jsonMessage, c.Uuid)
		
		// 通过接口发布消息
		if err := c.broker.Publish(ctx, securedMessage); err != nil {
			zap.L().Error("publish message error", zap.String("userId", c.Uuid), zap.Error(err))
		}
	}
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket
func (c *UserConn) Write() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case messageBack, ok := <-c.SendBack:
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			if err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message); err != nil {
				zap.L().Error("ws write error", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}
			
			// 推送成功后更新消息状态
			if repo := c.broker.GetMessageRepo(); repo != nil {
				_ = repo.UpdateStatus(messageBack.Uuid, message_status.Sent)
			}
		
		case <-ticker.C:
			// 定期发送 Ping 心跳
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// cleanup 清理资源
func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		c.broker.UnregisterClient(c)
		close(c.SendBack)
	})
}

// NewClientInit 初始化新的 WebSocket 客户端
func NewClientInit(c *gin.Context, clientId string, broker *MsgConsumer) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error(err.Error())
		return
	}
	
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		broker:   broker,
	}
	
	broker.RegisterClient(client)
	go client.Read()
	go client.Write()
}
```

---

## 7. MsgConsumer（Kafka 模式）

### 7.1 internal/service/chat/kafka_broker.go

```go
package chat

import (
	"context"
	"encoding/json"
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/pkg/enum/message/message_type"
	"sync"

	"go.uber.org/zap"
)

// MsgConsumer 基于 Kafka 的聊天服务
type MsgConsumer struct {
	Clients         sync.Map
	Producer        *kafka.Writer
	Reader          *kafka.Reader
	
	// 依赖注入
	messageRepo     mysql.MessageRepository
	friendshipRepo  mysql.FriendshipRepository
	groupMemberRepo mysql.GroupMemberRepository
	sessionRepo     mysql.SessionRepository
	cacheService    myredis.AsyncCacheService
	userRepo        mysql.UserRepository
}

// NewMsgConsumer 创建 KafkaBroker 实例（依赖注入）
func NewMsgConsumer(
	kafkaClient *KafkaClient,
	messageRepo mysql.MessageRepository,
	friendshipRepo mysql.FriendshipRepository,
	groupMemberRepo mysql.GroupMemberRepository,
	sessionRepo mysql.SessionRepository,
	cacheService myredis.AsyncCacheService,
	userRepo mysql.UserRepository,
) *MsgConsumer {
	return &MsgConsumer{
		Producer:        kafkaClient.Producer,
		Reader:          kafkaClient.Reader,
		messageRepo:     messageRepo,
		friendshipRepo:  friendshipRepo,
		groupMemberRepo: groupMemberRepo,
		sessionRepo:     sessionRepo,
		cacheService:    cacheService,
		userRepo:        userRepo,
	}
}

// Start 启动 Kafka 消费者服务
func (mc *MsgConsumer) Start() {
	for {
		msg, err := mc.Reader.ReadMessage(context.Background())
		if err != nil {
			zap.L().Error("kafka read error", zap.Error(err))
			break
		}

		var req request.ChatMessageRequest
		if err := json.Unmarshal(msg.Value, &req); err != nil {
			continue
		}

		// 根据消息类型分发处理
		switch req.Type {
		case message_type.Text:
			mc.handleTextMessage(req)
		case message_type.Voice:
			mc.handleFileMessage(req)
		case message_type.File:
			mc.handleFileMessage(req)
		case message_type.AudioOrVideo:
			mc.handleAVMessage(req)
		case message_type.Recall:
			mc.handleRecallMessage(req)
		}
	}
}

// Publish 发布消息到 Kafka
func (mc *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
	return mc.Producer.WriteMessages(ctx, kafka.Message{
		Value: msg,
	})
}

// RegisterClient 注册客户端
func (mc *MsgConsumer) RegisterClient(client *UserConn) {
	mc.Clients.Store(client.Uuid, client)
	zap.L().Info("client registered", zap.String("userId", client.Uuid))
}

// UnregisterClient 注销客户端
func (mc *MsgConsumer) UnregisterClient(client *UserConn) {
	if client != nil {
		mc.Clients.Delete(client.Uuid)
	}
}

// GetClient 获取客户端
func (mc *MsgConsumer) GetClient(userId string) *UserConn {
	value, ok := mc.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}

// KickClient 单点登录互踢
func (mc *MsgConsumer) KickClient(userId string, reason string) {
	client := mc.GetClient(userId)
	if client != nil {
		// 推送下线通知
		notify := map[string]interface{}{
			"type":    message_type.KickNotification,
			"content": reason,
		}
		data, _ := json.Marshal(notify)
		_ = client.Conn.WriteMessage(websocket.TextMessage, data)
		// 断开连接
		client.cleanup()
	}
}

// PushRecallNotify 推送撤回通知
func (mc *MsgConsumer) PushRecallNotify(messageUuid, receiveId string) {
	client := mc.GetClient(receiveId)
	if client != nil {
		notify := map[string]interface{}{
			"type":       message_type.Recall,
			"messageUuid": messageUuid,
		}
		data, _ := json.Marshal(notify)
		_ = client.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

// Close 关闭资源
func (mc *MsgConsumer) Close() {
	mc.Reader.Close()
	mc.Producer.Close()
}

// GetMessageRepo 获取消息仓库
func (mc *MsgConsumer) GetMessageRepo() mysql.MessageRepository {
	return mc.messageRepo
}
```

---

## 8. 配置文件

### 8.1 configs/config.toml

```toml
[kafkaConfig]
hostPort = "localhost:9092"  # Kafka 地址
chatTopic = "chat_topic"     # Topic 名称
partition = 1                # 分区数
timeout = 10                 # 超时时间(秒)
messageMode = "kafka"        # 消息模式（本项目仅支持 kafka）
```

---

## 9. 主程序启动示例

```go
package main

import (
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/service/chat"
	// ... 其他依赖
)

func main() {
	conf := config.GetConfig()

	// 创建聊天服务器
	chatServer := chat.NewChatServer(chat.ChatServerConfig{
		MessageRepo:     repos.Message,
		FriendshipRepo:  repos.Friendship,
		GroupMemberRepo: repos.GroupMember,
		SessionRepo:     repos.Session,
		CacheService:    cacheService,
		UserRepo:        repos.User,
	})

	// 初始化 Kafka 连接
	chatServer.InitKafka()

	// 注入 Broker 到 Handler
	handlers := handler.NewHandlers(services, chatServer.GetBroker())

	// 启动聊天服务器
	go chatServer.Run()

	// 启动 HTTP 服务器...
}
```

---

## ✅ 本节小结

- 本项目**只支持 Kafka 模式**
- `*MsgConsumer` 实现了消息消费者的所有功能
- `MsgConsumer` 负责从 Kafka 消费消息并处理
- WebSocket 网关通过 `Publish` 接口发布消息，不关心底层实现
- 支持单点登录互踢（KickClient）和消息撤回通知（PushRecallNotify）

---

## 📚 阶段五完成！

恭喜！你已经完成了 **阶段五：WebSocket 实时通讯**。

你可以继续完善项目的其他功能，如：
- 音视频通话 WebRTC 集成
- 消息已读/未读状态
- 消息撤回功能
- 离线消息推送
