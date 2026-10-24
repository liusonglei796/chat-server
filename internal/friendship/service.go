package friendship

import (
	"context"

	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
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

// recordOutboxEvent 统一记录本地 outbox 待发事件（与事务在同一连接/事务上下文中）
func (s *FriendshipService) recordOutboxEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	db := database.GetDB(ctx, s.db)
	if s.eventRecord != nil {
		return s.eventRecord(ctx, db, aggregateId, eventType, payload)
	}
	if db == nil {
		return nil
	}
	return outbox.RecordOutboxEvent(ctx, db, aggregateId, eventType, payload)
}
