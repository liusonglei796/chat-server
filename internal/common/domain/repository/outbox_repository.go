package repository

import (
	"context"

	"kama_chat_server/internal/common/model"
)

// OutboxRepository 事务发件箱数据访问接口
type OutboxRepository interface {
	Create(ctx context.Context, outbox *model.Outbox) error
	FindPending(ctx context.Context, limit int) ([]model.Outbox, error)
	MarkPublished(ctx context.Context, uuids []string) error
	IncrementRetry(ctx context.Context, uuid string) error
}
