// Package kafka 统一封装 Kafka 基础设施：主题常量、生产者/消费者工厂、主题确保与消息收发。
// 全项目所有 Kafka 读写都必须经由本包，禁止各服务手写 kafka-go 构造。
package kafka

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"

	"kama_chat_server/internal/common/config"
)

// 主题常量：全项目 Kafka 主题的唯一出处
const (
	TopicDomainEvents  = "domain_events"
	TopicChatUpstream  = "chat_upstream"
	TopicChatDownstream = "chat_downstream"
)

// NewProducer 创建写入指定主题的生产者
func NewProducer(topic string) *kafka.Writer {
	cfg := config.GetConfig().KafkaConfig
	return &kafka.Writer{
		Addr:                   kafka.TCP(cfg.HostPort),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           cfg.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: true,
	}
}

// NewConsumer 创建消费指定主题的消费者（按消费组负载均衡）
func NewConsumer(topic, groupID string) *kafka.Reader {
	cfg := config.GetConfig().KafkaConfig
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{cfg.HostPort},
		Topic:          topic,
		CommitInterval: cfg.Timeout * time.Second,
		GroupID:        groupID,
		StartOffset:    kafka.LastOffset,
	})
}

// EnsureTopic 确保主题已存在。
// Kafka 数据目录无持久卷，冷启动时主题可能尚未创建；
// 若消费者在主题创建前加入消费组，将被分配 0 个分区且不会自动重平衡。
// CreateTopics 对已存在的主题幂等，无副作用。
func EnsureTopic(ctx context.Context, topic string) error {
	conn, err := kafka.Dial("tcp", config.GetConfig().KafkaConfig.HostPort)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	return controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     3,
		ReplicationFactor: 1,
	})
}

// Publish 发送一条消息到指定主题。
// key 用于分区路由（同一 key 的消息进同一分区，保证有序）；
// headers 透传元数据（如 trace context、事件类型）。nil 表示无头。
func Publish(ctx context.Context, w *kafka.Writer, key []byte, payload []byte, headers []kafka.Header) error {
	return w.WriteMessages(ctx, kafka.Message{
		Key:     key,
		Value:   payload,
		Headers: headers,
	})
}
