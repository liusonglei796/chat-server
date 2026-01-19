// Package chat 实现了聊天系统的核心服务层
// kafka_client.go
// 核心职责：Kafka 基础设施管理
//
// 架构说明（为什么这样设计？）：
//
// 1. 用户 (WebSocket Client) <-> Go 服务器:
//   - 用户端（手机/网页）只通过 WebSocket 与 Go 服务器保持长连接。
//   - 用户完全**不知道** Kafka 的存在，他们只是单纯地发送和接收消息。
//
// 2. Go 服务器 (Kafka Client) <-> Kafka 集群:
//   - **生产者角色**: 当 Go 服务器收到用户的 WebSocket 消息时，把它包装后写入 Kafka Topic。
//   - **消费者角色**: Go 服务器（集群中的任意节点）从 Kafka 订阅消息，解析出接收者是谁，然后通过该接收者的 WebSocket 连接把消息推送到他手机上。
//
// 3. 核心优势：
//   - **解耦**: 发送者和接收者不需要连在同一个 Go 服务器节点上（支持分布式集群）。
//   - **缓冲**: 应对流量削峰填谷。
//   - **安全**: 只有后端服务器能连 Kafka，外网用户无法接触核心消息队列。
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
	Producer  *kafka.Writer // 生产者：负责写入消息
	Consumer  *kafka.Reader // 消费者：负责读取消息
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
		zap.L().Error("service error", zap.Error(err))
	}
	if err := k.Consumer.Close(); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
}

// SendMessage 提供给 Producer (UserConn) 使用的写入接口
// 用于向 Kafka集群 发送消息
func (k *KafkaClient) SendMessage(ctx context.Context, key, value []byte) error {
	return k.Producer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}
