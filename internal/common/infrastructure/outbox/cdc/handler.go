package cdc

import (
	"context"
	"fmt"
	"time"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/outbox"
)

// PublishFunc 投递消息的可注入函数，支持单元测试
type PublishFunc func(ctx context.Context, key []byte, payload []byte, headers []kgo.RecordHeader) error

// OutboxEventHandler 监听 outbox 表的 INSERT binlog 并投递至 Kafka
type OutboxEventHandler struct {
	canal.DummyEventHandler
	posStore  PositionStore
	publishFn PublishFunc
}

// NewOutboxEventHandler 创建事件处理器
func NewOutboxEventHandler(posStore PositionStore, publishFn PublishFunc) *OutboxEventHandler {
	return &OutboxEventHandler{
		posStore:  posStore,
		publishFn: publishFn,
	}
}

// OnRow 拦截行级变更事件
func (h *OutboxEventHandler) OnRow(e *canal.RowsEvent) error {
	// 仅监听 INSERT 事件
	if e.Action != canal.InsertAction {
		return nil
	}

	// 仅拦截 outbox 表
	if e.Table == nil || e.Table.Name != "outbox" {
		return nil
	}

	// 解析列索引
	uuidIdx := -1
	typeIdx := -1
	payloadIdx := -1
	for idx, col := range e.Table.Columns {
		switch col.Name {
		case "uuid":
			uuidIdx = idx
		case "event_type":
			typeIdx = idx
		case "payload":
			payloadIdx = idx
		}
	}

	if uuidIdx == -1 || typeIdx == -1 || payloadIdx == -1 {
		zap.L().Warn("outbox table schema missing required columns",
			zap.String("schema", e.Table.Schema),
			zap.String("table", e.Table.Name),
		)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 逐行解析并投递
	for _, row := range e.Rows {
		uuid := toString(row[uuidIdx])
		eventType := toString(row[typeIdx])
		payloadStr := toString(row[payloadIdx])

		if uuid == "" || eventType == "" {
			continue
		}

		headers := []kgo.RecordHeader{
			{Key: outbox.EventTypeHeader, Value: []byte(eventType)},
		}

		err := h.publishFn(ctx, []byte(uuid), []byte(payloadStr), headers)
		if err != nil {
			zap.L().Error("cdc publish domain event to kafka failed",
				zap.String("schema", e.Table.Schema),
				zap.String("uuid", uuid),
				zap.String("event_type", eventType),
				zap.Error(err),
			)
			return fmt.Errorf("cdc publish event %s failed: %w", uuid, err)
		}

		zap.L().Info("cdc successfully forwarded outbox event",
			zap.String("schema", e.Table.Schema),
			zap.String("uuid", uuid),
			zap.String("event_type", eventType),
		)
	}

	return nil
}

// OnPosSynced 当位点推进时触发持久化
func (h *OutboxEventHandler) OnPosSynced(header *replication.EventHeader, pos mysql.Position, set mysql.GTIDSet, force bool) error {
	if h.posStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return h.posStore.Save(ctx, pos)
}

// String 返回处理器标识
func (h *OutboxEventHandler) String() string {
	return "OutboxEventHandler"
}

// toString 辅助转换 MySQL 字段值为 string
func toString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// MakeKafkaPublishFunc 构造默认向 Kafka 发送消息的函数
func MakeKafkaPublishFunc(cl *kgo.Client) PublishFunc {
	return func(ctx context.Context, key []byte, payload []byte, headers []kgo.RecordHeader) error {
		return kafkainfra.Publish(ctx, cl, key, payload, headers)
	}
}
