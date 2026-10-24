package group

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/idempotent"
)

// GroupService 群组业务逻辑实现
type GroupService struct {
	db          *gorm.DB
	groupStore  GroupStore
	memberStore GroupMemberStore
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
	eventRecord func(ctx context.Context, tx *gorm.DB, aggregateId, eventType string, payload []byte) error
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(
	db *gorm.DB,
	groupStore GroupStore,
	memberStore GroupMemberStore,
	cacheService cacheinfra.AsyncCacheService,
	guard ...idempotent.Guard,
) *GroupService {
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
	return &GroupService{
		db:          db,
		groupStore:  groupStore,
		memberStore: memberStore,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}

// recordEvent 记录领域事件，支持单测 hook 拦截
func (g *GroupService) recordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	if g.eventRecord != nil {
		return g.eventRecord(ctx, database.GetDB(ctx, g.db), aggregateId, eventType, payload)
	}
	db := database.GetDB(ctx, g.db)
	if db == nil {
		return nil
	}
	return outbox.RecordEvent(ctx, db, aggregateId, eventType, payload)
}
