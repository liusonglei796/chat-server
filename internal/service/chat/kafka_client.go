// Package chat 实现了聊天系统的核心服务层
// kafka_client.go
// 核心职责：Kafka 基础设施管理
// 1. 封装 Kafka 底层连接 (Writer/Reader)
// 2. 提供消息写入接口 (WriteMessage)
// 3. 负责 Kafka 资源的初始化和关闭
// 4. 纯技术组件，不包含聊天业务逻辑
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
		zap.L().Error(err.Error())
	}
	if err := k.Consumer.Close(); err != nil {
		zap.L().Error(err.Error())
	}
}

// SendMessage 提供给 Producer (UserConn) 使用的写入接口
// 用于向 Kafka 发送消息
func (k *KafkaClient) SendMessage(ctx context.Context, key, value []byte) error {
	return k.Producer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}
