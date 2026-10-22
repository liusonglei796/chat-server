package outbox

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/snowflake"
	otelx "kama_chat_server/pkg/otel"
)

// RecordEvent 向当前事务/连接的 outbox 表中写入一条领域事件
func RecordEvent(ctx context.Context, db *gorm.DB, aggregateId, eventType string, payload []byte) error {
	traceParent, traceState := otelx.CaptureTraceContext(ctx)
	o := Outbox{
		Uuid:        fmt.Sprintf("O%s", snowflake.GenerateIDString()),
		AggregateId: aggregateId,
		EventType:   eventType,
		Payload:     string(payload),
		TraceParent: traceParent,
		TraceState:  traceState,
		CreatedAt:   time.Now(),
	}
	return db.WithContext(ctx).Create(&o).Error
}
