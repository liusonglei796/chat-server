# 18. Kafka 集成与消息模式

> 本教程将引入 Kafka 消息队列，实现异步消息处理，解耦消息的生产与消费。

---

## 📌 学习目标

- 理解 Channel 模式与 Kafka 模式的区别
- 掌握 `MessageBroker` 接口设计与依赖注入
- 实现消息生产者与消费者
- 理解 MsgConsumer 与 StandaloneServer 的架构设计

---

## 1. 消息模式对比

项目设计了两种消息传递模式，通过配置文件的 `kafkaConfig.messageMode` 切换：

### 1.1 Channel 模式 (Default)
- **机制**：使用 Go 原生 `channel` 在内存中转发消息
- **数据流**：WebSocket Client → StandaloneServer.Transmit → 消息处理 → DB/WebSocket
- **优点**：简单、无依赖、部署快、延迟低
- **缺点**：单机受限，重启丢失堆积消息，无法支持多实例集群
- **适用**：开发环境、单机部署、小规模应用

### 1.2 Kafka 模式
- **机制**：使用 Kafka 作为消息中间件进行异步解耦
- **数据流**：WebSocket Client → Kafka Producer → Kafka Broker → MsgConsumer → 消息处理 → DB/WebSocket
- **优点**：高吞吐、持久化、削峰填谷、支持集群扩容、消息可追溯
- **缺点**：架构复杂、依赖外部组件、需要运维 Kafka 集群
- **适用**：生产环境、分布式集群、高并发场景

---

## 2. 项目结构

```
internal/service/chat/
├── server.go          # ChatServer 聚合结构 + MessageBroker 接口
├── ws_gateway.go      # WebSocket 连接管理 (UserConn)
├── channel_broker.go  # StandaloneServer (Channel 模式)
├── kafka_broker.go    # MsgConsumer (Kafka 模式)
└── kafka_client.go    # Kafka 客户端封装
```

---

## 3. 安装依赖

```bash
go get github.com/segmentio/kafka-go
```

---

## 4. MessageBroker 接口设计

### 4.1 internal/service/chat/server.go

```go
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
	// GetMessageRepo 获取消息 Repository
	GetMessageRepo() repository.MessageRepository
}

// ChatServer 聊天服务器聚合结构
type ChatServer struct {
	Broker          MessageBroker
	KafkaClient     *KafkaClient
	messageRepo     repository.MessageRepository
	groupMemberRepo repository.GroupMemberRepository
	cacheService    myredis.AsyncCacheService
	mode            string
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
		cs.Broker = NewMsgConsumer(cs.KafkaClient, cs.messageRepo, cs.groupMemberRepo, cs.cacheService)
	} else {
		// Channel 模式（默认）
		cs.Broker = NewStandaloneServer(cs.messageRepo, cs.groupMemberRepo, cs.cacheService)
	}

	return cs
}

// InitKafka 初始化 Kafka 连接
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
	Consumer  *kafka.Reader // 消费者
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
	k.Consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          kafkaConfig.ChatTopic,
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "chat",
		StartOffset:    kafka.LastOffset,
	})
}

func (k *KafkaClient) KafkaClose() {
	if err := k.Producer.Close(); err != nil {
		zap.L().Error(err.Error())
	}
	if err := k.Consumer.Close(); err != nil {
		zap.L().Error(err.Error())
	}
}

// WriteMessage 向 Kafka 发送消息
func (k *KafkaClient) WriteMessage(ctx context.Context, key, value []byte) error {
	return k.Producer.WriteMessages(ctx, kafka.Message{
		Key:   key,
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
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MessageBack 用于回传消息给前端
type MessageBack struct {
	Message []byte
	Uuid    int64
}

// UserConn 表示一个 WebSocket 客户端连接
type UserConn struct {
	Conn     *websocket.Conn
	Uuid     string
	SendTo   chan []byte       // 缓冲通道
	SendBack chan *MessageBack // 给前端
	broker   MessageBroker     // 注入的消息代理
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

var ctx = context.Background()

// Read 从 WebSocket 读取消息并通过 Broker 发布
func (c *UserConn) Read() {
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			zap.L().Error(err.Error())
			return
		}
		// 通过接口发布消息，不关心具体实现
		if err := c.broker.Publish(ctx, jsonMessage); err != nil {
			zap.L().Error(err.Error())
		}
	}
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket
func (c *UserConn) Write() {
	for messageBack := range c.SendBack {
		err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message)
		if err != nil {
			zap.L().Error(err.Error())
			return
		}
		// 通过 Repository 接口更新消息状态
		if repo := c.broker.GetMessageRepo(); repo != nil {
			repo.UpdateStatus(messageBack.Uuid, message_status_enum.Sent)
		}
	}
}

// NewClientInit 初始化新的 WebSocket 客户端
func NewClientInit(c *gin.Context, clientId string, broker MessageBroker) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error(err.Error())
		return
	}
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendTo:   make(chan []byte, constants.CHANNEL_SIZE),
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		broker:   broker,
	}
	broker.RegisterClient(client)
	go client.Read()
	go client.Write()
}
```

---

## 7. StandaloneServer（Channel 模式）

### 7.1 internal/service/chat/channel_broker.go

```go
package chat

import (
	"context"
	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/pkg/constants"
	"sync"
)

// StandaloneServer 单机模式聊天服务器
type StandaloneServer struct {
	Clients         sync.Map
	Transmit        chan []byte
	Login           chan *UserConn
	Logout          chan *UserConn
	messageRepo     repository.MessageRepository
	groupMemberRepo repository.GroupMemberRepository
	cacheService    myredis.AsyncCacheService
}

// NewStandaloneServer 创建 ChannelBroker 实例（依赖注入）
func NewStandaloneServer(
	messageRepo repository.MessageRepository,
	groupMemberRepo repository.GroupMemberRepository,
	cacheService myredis.AsyncCacheService,
) *StandaloneServer {
	return &StandaloneServer{
		Transmit:        make(chan []byte, constants.CHANNEL_SIZE),
		Login:           make(chan *UserConn, constants.CHANNEL_SIZE),
		Logout:          make(chan *UserConn, constants.CHANNEL_SIZE),
		messageRepo:     messageRepo,
		groupMemberRepo: groupMemberRepo,
		cacheService:    cacheService,
	}
}

// Start 启动 Channel Server 主循环
func (s *StandaloneServer) Start() {
	for {
		select {
		case client := <-s.Login:
			s.Clients.Store(client.Uuid, client)
		case client := <-s.Logout:
			s.Clients.Delete(client.Uuid)
		case data := <-s.Transmit:
			// 反序列化并根据消息类型分发处理
			s.handleMessage(data)
		}
	}
}

// Publish 实现 MessageBroker 接口
func (s *StandaloneServer) Publish(ctx context.Context, msg []byte) error {
	s.Transmit <- msg
	return nil
}
```

---

## 8. MsgConsumer（Kafka 模式）

### 8.1 internal/service/chat/kafka_broker.go

```go
package chat

import (
	"context"
	"encoding/json"
	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/pkg/enum/message/message_type_enum"
	"sync"

	"go.uber.org/zap"
)

// MsgConsumer 基于 Kafka 的聊天服务
type MsgConsumer struct {
	Clients         sync.Map
	Login           chan *UserConn
	Logout          chan *UserConn
	kafkaClient     *KafkaClient
	messageRepo     repository.MessageRepository
	groupMemberRepo repository.GroupMemberRepository
	cacheService    myredis.AsyncCacheService
}

// NewMsgConsumer 创建 KafkaBroker 实例（依赖注入）
func NewMsgConsumer(
	kafkaClient *KafkaClient,
	messageRepo repository.MessageRepository,
	groupMemberRepo repository.GroupMemberRepository,
	cacheService myredis.AsyncCacheService,
) *MsgConsumer {
	return &MsgConsumer{
		Login:           make(chan *UserConn),
		Logout:          make(chan *UserConn),
		kafkaClient:     kafkaClient,
		messageRepo:     messageRepo,
		groupMemberRepo: groupMemberRepo,
		cacheService:    cacheService,
	}
}

// Start 启动 Kafka 消费者服务
func (k *MsgConsumer) Start() {
	// 启动 Kafka 消费协程
	go func() {
		for {
			kafkaMessage, err := k.kafkaClient.Consumer.ReadMessage(ctx)
			if err != nil {
				zap.L().Error(err.Error())
				continue
			}

			var chatMessageReq request.ChatMessageRequest
			if err := json.Unmarshal(kafkaMessage.Value, &chatMessageReq); err != nil {
				zap.L().Error(err.Error())
				continue
			}

			switch chatMessageReq.Type {
			case message_type_enum.Text:
				k.handleTextMessage(chatMessageReq)
			case message_type_enum.File:
				k.handleFileMessage(chatMessageReq)
			case message_type_enum.AudioOrVideo:
				k.handleAVMessage(chatMessageReq)
			}
		}
	}()

	// 主循环：处理登录/登出
	for {
		select {
		case client := <-k.Login:
			k.Clients.Store(client.Uuid, client)
		case client := <-k.Logout:
			k.Clients.Delete(client.Uuid)
		}
	}
}

// Publish 实现 MessageBroker 接口
func (k *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
	key := []byte("0")
	return k.kafkaClient.WriteMessage(ctx, key, msg)
}
```

---

## 9. 配置文件

### 9.1 configs/config.toml

```toml
[kafkaConfig]
hostPort = "localhost:9092"  # Kafka 地址
chatTopic = "chat_topic"     # Topic 名称
partition = 1                # 分区数
timeout = 10                 # 超时时间(秒)
messageMode = "channel"      # 消息模式: "channel" 或 "kafka"
```

---

## 10. 主程序启动示例

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
		Mode:            conf.KafkaConfig.MessageMode,
		MessageRepo:     messageRepo,     // 注入 Repository
		GroupMemberRepo: groupMemberRepo, // 注入 Repository
		CacheService:    cacheService,    // 注入 Redis 缓存服务
	})

	// Kafka 模式需要初始化连接
	if conf.KafkaConfig.MessageMode == "kafka" {
		chatServer.InitKafka()
	}

	// 启动聊天服务器
	go chatServer.Start()

	// 启动 HTTP 服务器...
}
```

---

## 11. Channel vs Kafka 对比

| 对比项 | Channel 模式 | Kafka 模式 |
|-------|-------------|-----------|
| **Server 类型** | StandaloneServer | MsgConsumer |
| **消息队列** | Go channel（内存） | Kafka（分布式） |
| **适用场景** | 开发环境、单机部署 | 生产环境、集群部署 |
| **消息持久化** | 否（重启丢失） | 是（磁盘存储） |
| **横向扩展** | 不支持 | 支持多实例 |
| **消息顺序** | 严格保证 | 分区内有序 |
| **依赖组件** | 无 | Kafka 集群 |

---

## 12. 选择建议

| 场景 | 推荐模式 |
|-----|---------|
| 本地开发 | Channel |
| 功能测试 | Channel |
| 小规模生产（<100人） | Channel |
| 中大规模生产 | Kafka |
| 需要消息持久化 | Kafka |
| 需要水平扩展 | Kafka |

---

## ✅ 本节完成

你已经完成了：
- [x] Channel 与 Kafka 模式对比
- [x] MessageBroker 接口设计
- [x] ChatServer 聚合与依赖注入
- [x] Kafka 客户端封装
- [x] MsgConsumer 与 StandaloneServer 实现
- [x] 模式切换配置

---

## 📚 阶段五完成！

恭喜！你已经完成了 **阶段五：WebSocket 实时通讯**。

你可以继续完善项目的其他功能，如：
- 音视频通话 WebRTC 集成
- 消息已读/未读状态
- 消息撤回功能
- 离线消息推送
