// Package outbox 提供事务发件箱的发布基础设施
// 负责将 outbox 表中的事件投递到 Kafka domain_events 主题
package outbox

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"

	"kama_chat_server/internal/common/config"
)

// DomainEventsTopic 跨服务领域事件主题
const DomainEventsTopic = "domain_events"

// EnsureTopic 确保 domain_events 主题已存在。
// Kafka 数据目录无持久卷，冷启动时主题可能尚未由生产者创建；
// 若消费者在主题创建前加入消费组，将被分配 0 个分区且不会自动重平衡。
func EnsureTopic(ctx context.Context, brokers []string) error {
	conn, err := kafka.Dial("tcp", brokers[0])
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

	// CreateTopics 对已存在的主题幂等，无副作用
	return controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             DomainEventsTopic,
		NumPartitions:     3,
		ReplicationFactor: 1,
	})
}

// NewProducer 创建指向 domain_events 主题的 Kafka 生产者
func NewProducer() *kafka.Writer {
	cfg := config.GetConfig().KafkaConfig
	return &kafka.Writer{
		Addr:                   kafka.TCP(cfg.HostPort),
		Topic:                  DomainEventsTopic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           cfg.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: true,
	}
}

// Publish 将一条事件写入 Kafka
// 以事件 UUID 作为消息 Key，保证同一事件的路由一致性
func Publish(ctx context.Context, w *kafka.Writer, eventType string, eventUuid string, payload []byte) error {
	msg := kafka.Message{
		Key:   []byte(eventUuid),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(eventType)},
		},
	}
	return w.WriteMessages(ctx, msg)
}
