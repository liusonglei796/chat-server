package outbox

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"kama_chat_server/internal/common/domain/store"
	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
)

// EventTypeHeader 领域事件类型在消息头中的 key
const EventTypeHeader = "event_type"

// ExtractEventType 从消息头中读取事件类型，供各服务领域事件消费者解析
func ExtractEventType(headers []kafka.Header) string {
	for _, h := range headers {
		if h.Key == EventTypeHeader {
			return string(h.Value)
		}
	}
	return ""
}

// PublishFunc 发送事件的可注入函数，便于单测替换真实 Kafka
type PublishFunc func(ctx context.Context, eventType string, uuid string, payload []byte) error

// Publisher 轮询 outbox 表，将待发布事件发送到 Kafka domain_events
type Publisher struct {
	outboxStore store.OutboxStore
	producer   *kafka.Writer
	interval   time.Duration
	batchSize  int
	publishFn  PublishFunc
}

// NewPublisher 创建发布器，默认 1s 轮询、每批 100 条
func NewPublisher(store store.OutboxStore, producer *kafka.Writer) *Publisher {
	return &Publisher{
		outboxStore: store,
		producer:   producer,
		interval:   1 * time.Second,
		batchSize:  100,
		publishFn: func(ctx context.Context, eventType string, uuid string, payload []byte) error {
			headers := []kafka.Header{{Key: EventTypeHeader, Value: []byte(eventType)}}
			return kafkainfra.Publish(ctx, producer, []byte(uuid), payload, headers)
		},
	}
}

// Start 后台协程周期性执行 Dispatch
func (p *Publisher) Start() {
	go func() {
		for {
			time.Sleep(p.interval)
			if err := p.Dispatch(context.Background()); err != nil {
				zap.L().Error("outbox dispatch error", zap.Error(err))
			}
		}
	}()
}

// Dispatch 拉取一批待发布事件并逐条发送，成功后标记已发布，失败则重试计数 +1
func (p *Publisher) Dispatch(ctx context.Context) error {
	events, err := p.outboxStore.FindPending(ctx, p.batchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	var published []string
	for _, e := range events {
		if err := p.publishFn(ctx, e.EventType, e.Uuid, []byte(e.Payload)); err != nil {
			zap.L().Error("publish outbox event error", zap.String("uuid", e.Uuid), zap.String("type", e.EventType), zap.Error(err))
			_ = p.outboxStore.IncrementRetry(ctx, e.Uuid)
			continue
		}
		published = append(published, e.Uuid)
	}
	if len(published) > 0 {
		return p.outboxStore.MarkPublished(ctx, published)
	}
	return nil
}
