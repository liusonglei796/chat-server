// Package outbox 提供事务发件箱的发布基础设施
// 负责将 outbox 表中的事件投递到 Kafka domain_events 主题
package outbox

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"

	"kama_chat_server/internal/config"
)

// DomainEventsTopic 跨服务领域事件主题
const DomainEventsTopic = "domain_events"

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
