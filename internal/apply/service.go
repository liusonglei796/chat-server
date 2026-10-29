package apply

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/lock"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/event"
	"kama_chat_server/pkg/idempotent"
)

// ApplyService 申请业务逻辑实现
type ApplyService struct {
	db          *gorm.DB
	store       Store
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
	idempStore  idempotency.Store
	lock        lock.Distributed
}

// NewApplyService 构造函数，注入所有依赖
func NewApplyService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *ApplyService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc != nil && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	var idemp idempotency.Store
	if db != nil {
		idemp = idempotency.NewGormStore(db)
	}
	var distLock lock.Distributed
	if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc != nil && rc.Client() != nil {
		distLock = lock.NewRedis(rc.Client())
	}
	return &ApplyService{
		db:          db,
		store:       store,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
		idempStore:  idemp,
		lock:        distLock,
	}
}

// recordOutboxEvent 统一记录本地 outbox 待发事件（与事务在同一连接/事务上下文中）
func (u *ApplyService) recordOutboxEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	if u.db == nil {
		return nil
	}
	return outbox.RecordOutboxEvent(ctx, database.GetDB(ctx, u.db), aggregateId, eventType, payload)
}

// recordFriendApplyPassedEvent 记录好友申请通过领域事件
func (u *ApplyService) recordFriendApplyPassedEvent(ctx context.Context, userId, friendId string) error {
	payload, _ := json.Marshal(event.FriendApplyPassedEvent{
		UserId:   userId,
		FriendId: friendId,
	})
	return u.recordOutboxEvent(ctx, userId, event.EventFriendApplyPassed, payload)
}

// recordGroupApplyPassedEvent 记录入群申请通过领域事件
func (u *ApplyService) recordGroupApplyPassedEvent(ctx context.Context, groupId, userId string) error {
	payload, _ := json.Marshal(event.GroupApplyPassedEvent{
		GroupId: groupId,
		UserId:  userId,
	})
	return u.recordOutboxEvent(ctx, groupId, event.EventGroupApplyPassed, payload)
}
