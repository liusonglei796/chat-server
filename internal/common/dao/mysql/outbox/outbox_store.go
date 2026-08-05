// Package outbox 提供事务发件箱数据访问层的具体实现
// 本文件实现 OutboxStore 接口，处理 outbox 表的数据库操作
package outbox

import (
	"context"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/model"
)

// outboxStore OutboxStore 接口的实现
type outboxStore struct {
	db *gorm.DB
}

// NewOutboxStore 创建 OutboxStore 实例
func NewOutboxStore(db *gorm.DB) *outboxStore {
	return &outboxStore{db: db}
}

// 确保实现 store.OutboxStore 接口
var _ store.OutboxStore = (*outboxStore)(nil)

// Create 写入一条待发布事件
func (r *outboxStore) Create(ctx context.Context, o *model.Outbox) error {
	return dberr.WrapDBError(r.db.WithContext(ctx).Create(o).Error, "写入outbox")
}

// FindPending 查询待发布事件，按创建时间升序取前 limit 条
func (r *outboxStore) FindPending(ctx context.Context, limit int) ([]model.Outbox, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []model.Outbox
	err := r.db.WithContext(ctx).
		Where("status = ?", 0).
		Order("created_at ASC").
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, dberr.WrapDBError(err, "查询待发布outbox")
	}
	return list, nil
}

// MarkPublished 将一批事件标记为已发布
func (r *outboxStore) MarkPublished(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Outbox{}).
		Where("uuid IN ?", uuids).
		Updates(map[string]interface{}{"status": 1, "published_at": time.Now()}).Error
	return dberr.WrapDBError(err, "标记outbox已发布")
}

// IncrementRetry 发布失败时重试计数 +1
func (r *outboxStore) IncrementRetry(ctx context.Context, uuid string) error {
	err := r.db.WithContext(ctx).Model(&model.Outbox{}).
		Where("uuid = ?", uuid).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
	return dberr.WrapDBError(err, "outbox重试计数")
}
