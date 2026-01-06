# 18. Kafka 集成与消息模式

> 本教程将引入 Kafka 消息队列，实现异步消息处理，解耦消息的生产与消费。

---

## 📌 学习目标

- 理解 Channel 模式与 Kafka 模式的区别
- 掌握 `segmentio/kafka-go` 的集成
- 实现消息生产者与消费者
- 理解 MsgConsumer 的架构设计

---

## 1. 消息模式对比

项目设计了两种消息传递模式，通过配置文件的 `kafkaConfig.messageMode` 切换：

### 1.1 Channel 模式 (Default)
- **机制**：使用 Go 原生 `channel` 在内存中转发消息
- **数据流**：WebSocket Client → Channel → StandaloneServer.Transmit → 消息处理 → DB/WebSocket
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
├── conn_manager.go       # WebSocket 连接管理 (UserConn)
├── channel_server.go     # StandaloneServer (Channel 模式)
├── kafka_consumer.go     # MsgConsumer (Kafka 模式)
└── mq_manager.go         # Kafka 客户端封装
```

---

## 3. 安装依赖

```bash
go get github.com/segmentio/kafka-go
```

---

## 4. Kafka 客户端封装

### 4.1 internal/service/chat/mq_manager.go

```go
package chat

import (
	"context"
	"kama_chat_server/internal/config"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var ctx = context.Background()

// KafkaClient Kafka 客户端封装
type KafkaClient struct {
	Producer  *kafka.Writer // 生产者
	Consumer  *kafka.Reader // 消费者
	KafkaConn *kafka.Conn   // 连接 (用于创建 Topic)
}

// GlobalKafkaClient 全局 Kafka 客户端实例
var GlobalKafkaClient = new(KafkaClient)

// KafkaInit 初始化 Kafka
func (k *KafkaClient) KafkaInit() {
	kafkaConfig := config.GetConfig().KafkaConfig

	// 初始化生产者
	k.Producer = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  kafkaConfig.ChatTopic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: false,
	}

	// 初始化消费者
	k.Consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          kafkaConfig.ChatTopic,
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "chat",
		StartOffset:    kafka.LastOffset,
	})
}

// KafkaClose 关闭连接
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

## 5. 消息生产（conn_manager.go）

### 5.1 UserConn.Read 方法

```go
func (c *UserConn) Read() {
	zap.L().Info("ws read goroutine start")
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			zap.L().Error(err.Error())
			return
		}

		var message = request.ChatMessageRequest{}
		json.Unmarshal(jsonMessage, &message)
		log.Println("接受到消息为: ", jsonMessage)

		if messageMode == "channel" {
			// Channel 模式：发送到本地 Channel
			// 缓冲策略处理
			for len(GlobalStandaloneServer.Transmit) < constants.CHANNEL_SIZE && len(c.SendTo) > 0 {
				sendToMessage := <-c.SendTo
				GlobalStandaloneServer.SendMessageToTransmit(sendToMessage)
			}
			if len(GlobalStandaloneServer.Transmit) < constants.CHANNEL_SIZE {
				GlobalStandaloneServer.SendMessageToTransmit(jsonMessage)
			} else if len(c.SendTo) < constants.CHANNEL_SIZE {
				c.SendTo <- jsonMessage
			}
		} else {
			// Kafka 模式：直接写入 Kafka
			key := []byte(strconv.Itoa(config.GetConfig().KafkaConfig.Partition))
			if err := GlobalKafkaClient.WriteMessage(ctx, key, jsonMessage); err != nil {
				zap.L().Error(err.Error())
			}
			zap.L().Info("已发送消息：" + string(jsonMessage))
		}
	}
}
```

---

## 6. MsgConsumer（Kafka 消费者）

### 6.1 internal/service/chat/kafka_consumer.go

```go
package chat

import (
	"encoding/json"
	"fmt"
	"sync"

	dao "kama_chat_server/internal/dao/mysql"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/message/message_type_enum"
	"kama_chat_server/pkg/util/snowflake"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MsgConsumer 基于 Kafka 的聊天服务
type MsgConsumer struct {
	Clients sync.Map       // 在线客户端 (sync.Map)
	Login   chan *UserConn // 登录通道
	Logout  chan *UserConn // 登出通道
}

// GlobalMsgConsumer 全局单例
var GlobalMsgConsumer *MsgConsumer

// InitKafkaServer 初始化 MsgConsumer
func InitKafkaServer() {
	if GlobalMsgConsumer == nil {
		GlobalMsgConsumer = &MsgConsumer{
			Login:  make(chan *UserConn),
			Logout: make(chan *UserConn),
		}
	}
}

// Start 启动 Kafka 消费者服务
func (k *MsgConsumer) Start() {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
		}
		close(k.Login)
		close(k.Logout)
	}()

	// 启动 Kafka 消费协程
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("kafka consumer panic: %v", r))
			}
		}()
		for {
			// 从 Kafka 读取消息
			kafkaMessage, err := GlobalKafkaClient.Consumer.ReadMessage(ctx)
			if err != nil {
				zap.L().Error(err.Error())
				continue
			}

			// 反序列化
			var chatMessageReq request.ChatMessageRequest
			if err := json.Unmarshal(kafkaMessage.Value, &chatMessageReq); err != nil {
				zap.L().Error(err.Error())
				continue
			}

			// 根据消息类型分发处理
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
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到kama聊天服务器"))

		case client := <-k.Logout:
			k.Clients.Delete(client.Uuid)
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录"))
		}
	}
}
```

---

## 7. 消息处理方法

MsgConsumer 的消息处理逻辑与 StandaloneServer 基本一致：

```go
// handleTextMessage 处理文本消息
func (k *MsgConsumer) handleTextMessage(req request.ChatMessageRequest) {
	message := model.Message{
		Uuid:       snowflake.GenerateID(),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    req.Content,
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: normalizePath(req.SendAvatar),
		ReceiveId:  req.ReceiveId,
		// ...
	}

	dao.GormDB.Create(&message)

	if message.ReceiveId[0] == 'U' {
		k.sendToUser(message, req.SendAvatar)
	} else if message.ReceiveId[0] == 'G' {
		k.sendToGroup(message, req.SendAvatar)
	}
}

// sendToUser / sendToGroup 方法与 StandaloneServer 类似
// 使用 sync.Map 进行客户端查找
```

---

## 8. 客户端管理方法

```go
func (k *MsgConsumer) SendClientToLogin(client *UserConn) {
	k.Login <- client
}

func (k *MsgConsumer) SendClientToLogout(client *UserConn) {
	k.Logout <- client
}

func (k *MsgConsumer) GetClient(userId string) *UserConn {
	value, ok := k.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}
```

---

## 9. 主程序启动

### 9.1 main.go

```go
package main

import (
	"fmt"
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/service/chat"
	"kama_chat_server/internal/https_server"
	"go.uber.org/zap"
)

func main() {
	conf := config.GetConfig()

	// 初始化 ChatServer
	chat.Init()

	if conf.KafkaConfig.MessageMode == "kafka" {
		// Kafka 模式
		chat.GlobalKafkaClient.KafkaInit()
		chat.InitKafkaServer()
		go chat.GlobalMsgConsumer.Start()
		zap.L().Info("Kafka 模式启动")
	} else {
		// Channel 模式
		go chat.GlobalStandaloneServer.Start()
		zap.L().Info("Channel 模式启动")
	}

	// 启动 HTTP 服务器
	https_server.Init()
	https_server.GE.Run(fmt.Sprintf("%s:%d", conf.MainConfig.Host, conf.MainConfig.Port))
}
```

---

## 10. 配置文件

### 10.1 configs/config.toml

```toml
[kafkaConfig]
hostPort = "localhost:9092"  # Kafka 地址
chatTopic = "chat_topic"     # Topic 名称
partition = 1                # 分区数
timeout = 10                 # 超时时间(秒)
messageMode = "channel"      # 消息模式: "channel" 或 "kafka"
```

---

## 11. Channel vs Kafka 对比

| 对比项 | Channel 模式 | Kafka 模式 |
|-------|-------------|-----------|
| **Server 类型** | StandaloneServer | MsgConsumer |
| **全局变量** | GlobalStandaloneServer | GlobalMsgConsumer |
| **消息队列** | Go channel（内存） | Kafka（分布式） |
| **适用场景** | 开发环境、单机部署 | 生产环境、集群部署 |
| **消息持久化** | 否（重启丢失） | 是（磁盘存储） |
| **横向扩展** | 不支持 | 支持多实例 |
| **消息顺序** | 严格保证 | 分区内有序 |
| **性能** | 极高（内存） | 高（网络+磁盘） |
| **依赖组件** | 无 | Kafka 集群 |
| **故障恢复** | 消息丢失 | 消息可恢复 |

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
- [x] Kafka 客户端封装（Producer/Consumer）
- [x] MsgConsumer 实现
- [x] 消息生产与消费流程
- [x] 模式切换配置

---

## 📚 阶段五完成！

恭喜！你已经完成了 **阶段五：WebSocket 实时通讯**。

你可以继续完善项目的其他功能，如：
- 音视频通话 WebRTC 集成
- 消息已读/未读状态
- 消息撤回功能
- 离线消息推送
