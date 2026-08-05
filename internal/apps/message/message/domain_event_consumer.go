package message

import (
	"context"
	"fmt"
	"os"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/outbox"
)

// DomainEventConsumer 消费 domain_events topic，处理跨服务会话变更
type DomainEventConsumer struct {
	reader  *kafka.Reader
	handler *SessionEventHandler
	quit    chan os.Signal
}

// NewDomainEventConsumer 创建领域事件消费者
func NewDomainEventConsumer(handler *SessionEventHandler) *DomainEventConsumer {
	reader := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "message_domain_events")
	return &DomainEventConsumer{reader: reader, handler: handler, quit: make(chan os.Signal, 1)}
}

// Start 后台协程持续读取并处理领域事件
func (c *DomainEventConsumer) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("domain event consumer panic: %v", r))
			}
		}()
		for {
			msg, err := c.reader.ReadMessage(context.Background())
			if err != nil {
				zap.L().Error("read domain event error", zap.Error(err))
				continue
			}
			eventType := outbox.ExtractEventType(msg.Headers)
			if err := c.handler.Handle(context.Background(), eventType, msg.Value); err != nil {
				zap.L().Error("handle domain event error", zap.String("type", eventType), zap.Error(err))
			}
		}
	}()
}

// Close 关闭 Kafka 消费者
func (c *DomainEventConsumer) Close() {
	if c.reader != nil {
		c.reader.Close()
	}
}
