package apply

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/idempotent"
)

// ApplyService 申请业务逻辑实现
type ApplyService struct {
	db          *gorm.DB
	store       Store
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewApplyService 构造函数，注入所有依赖
func NewApplyService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *ApplyService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	return &ApplyService{
		db:          db,
		store:       store,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}

// recordEvent 统一记录 outbox 事件（与事务在同一连接/事务上下文中）
func (u *ApplyService) recordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	if u.db == nil {
		return nil
	}
	return outbox.RecordEvent(ctx, database.GetDB(ctx, u.db), aggregateId, eventType, payload)
}
