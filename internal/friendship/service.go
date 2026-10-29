package friendship

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/lock"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/event"
)

// FriendshipService 好友关系业务逻辑实现
type FriendshipService struct {
	db          *gorm.DB
	store       Store
	cache       cacheinfra.AsyncCacheService
	lock        lock.Distributed
	eventRecord func(ctx context.Context, tx *gorm.DB, aggregateId, eventType string, payload []byte) error
}

// NewFriendshipService 构造函数注入
func NewFriendshipService(db *gorm.DB, store Store, cacheService ...cacheinfra.AsyncCacheService) *FriendshipService {
	var c cacheinfra.AsyncCacheService
	if len(cacheService) > 0 {
		c = cacheService[0]
	}
	var distLock lock.Distributed
	if rc, ok := c.(interface{ Client() *redis.Client }); ok && rc != nil && rc.Client() != nil {
		distLock = lock.NewRedis(rc.Client())
	}
	return &FriendshipService{
		db:    db,
		store: store,
		cache: c,
		lock:  distLock,
	}
}

// recordOutboxEvent 统一记录 outbox 事件（同一事务/连接中）
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

// recordFriendDeletedEvent 记录删除好友事件
func (s *FriendshipService) recordFriendDeletedEvent(ctx context.Context, userId, friendId string) error {
	payload, _ := json.Marshal(event.FriendDeletedEvent{
		UserId:   userId,
		FriendId: friendId,
	})
	return s.recordOutboxEvent(ctx, userId, event.EventFriendDeleted, payload)
}

// recordFriendBlackedEvent 记录拉黑好友事件
func (s *FriendshipService) recordFriendBlackedEvent(ctx context.Context, userId, friendId string) error {
	payload, _ := json.Marshal(event.FriendBlackedEvent{
		UserId:   userId,
		FriendId: friendId,
	})
	return s.recordOutboxEvent(ctx, userId, event.EventFriendBlacked, payload)
}
