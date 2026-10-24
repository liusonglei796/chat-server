package friendship

import (
	"context"

	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/outbox"
)

// FriendshipService 好友关系业务逻辑实现
type FriendshipService struct {
	db          *gorm.DB
	store       Store
	cache       cacheinfra.AsyncCacheService
	eventRecord func(ctx context.Context, tx *gorm.DB, aggregateId, eventType string, payload []byte) error
}

// NewFriendshipService 构造函数，注入所有依赖
func NewFriendshipService(db *gorm.DB, store Store, cacheService ...cacheinfra.AsyncCacheService) *FriendshipService {
	var c cacheinfra.AsyncCacheService
	if len(cacheService) > 0 {
		c = cacheService[0]
	}
	return &FriendshipService{
		db:    db,
		store: store,
		cache: c,
	}
}

// execTx 简明事务辅助函数：如果有 db 则走 GORM 事务，无 db（单测）则直接在当前 store 上执行
func (s *FriendshipService) execTx(ctx context.Context, fn func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error) error {
	if s.db == nil {
		record := func(aggregateId, eventType string, payload []byte) error {
			if s.eventRecord != nil {
				return s.eventRecord(ctx, nil, aggregateId, eventType, payload)
			}
			return nil
		}
		return fn(s.store, record)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txStore := NewStore(tx)
		record := func(aggregateId, eventType string, payload []byte) error {
			if s.eventRecord != nil {
				return s.eventRecord(ctx, tx, aggregateId, eventType, payload)
			}
			return outbox.RecordEvent(ctx, tx, aggregateId, eventType, payload)
		}
		return fn(txStore, record)
	})
}
