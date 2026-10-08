// Package outbox 提供事务发件箱数据访问层的具体实现
// 本文件实现 OutboxStore 接口，处理 outbox 表的数据库操作
package outbox

import (
	"context"

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
