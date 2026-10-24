package apply

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
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

// execTx 简明事务辅助函数：如果有 db 则走 GORM 事务，无 db（单测）则直接在当前 store 上执行
func (u *ApplyService) execTx(ctx context.Context, fn func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error) error {
	if u.db == nil {
		return fn(u.store, func(string, string, []byte) error { return nil })
	}
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txStore := NewStore(tx)
		return fn(txStore, func(aggregateId, eventType string, payload []byte) error {
			return outbox.RecordEvent(ctx, tx, aggregateId, eventType, payload)
		})
	})
}
