package group

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
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

// execTx 简明事务辅助函数：如果有 db 则走 GORM 事务，无 db（单测）则直接在当前 store 上执行
func (g *GroupService) execTx(ctx context.Context, fn func(txGroup GroupStore, txMember GroupMemberStore, recordEvent func(aggregateId, eventType string, payload []byte) error) error) error {
	if g.db == nil {
		record := func(aggregateId, eventType string, payload []byte) error {
			if g.eventRecord != nil {
				return g.eventRecord(ctx, nil, aggregateId, eventType, payload)
			}
			return nil
		}
		return fn(g.groupStore, g.memberStore, record)
	}
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGroup := NewGroupStore(tx)
		txMember := NewGroupMemberStore(tx)
		record := func(aggregateId, eventType string, payload []byte) error {
			if g.eventRecord != nil {
				return g.eventRecord(ctx, tx, aggregateId, eventType, payload)
			}
			return outbox.RecordEvent(ctx, tx, aggregateId, eventType, payload)
		}
		return fn(txGroup, txMember, record)
	})
}
