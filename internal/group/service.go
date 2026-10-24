package group

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/event"
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

// recordOutboxEvent 记录领域事件至本地 Outbox 发件箱表，支持单测 hook 拦截
func (g *GroupService) recordOutboxEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	if g.eventRecord != nil {
		return g.eventRecord(ctx, database.GetDB(ctx, g.db), aggregateId, eventType, payload)
	}
	db := database.GetDB(ctx, g.db)
	if db == nil {
		return nil
	}
	return outbox.RecordOutboxEvent(ctx, db, aggregateId, eventType, payload)
}

// recordGroupCreatedEvent 记录建群领域事件
func (g *GroupService) recordGroupCreatedEvent(ctx context.Context, group *GroupInfo) error {
	payload, _ := json.Marshal(event.GroupCreatedEvent{
		GroupId:     group.Uuid,
		OwnerId:     group.OwnerId,
		GroupName:   group.Name,
		GroupAvatar: group.Avatar,
	})
	return g.recordOutboxEvent(ctx, group.Uuid, event.EventGroupCreated, payload)
}

// recordGroupDismissedEvent 记录解散群领域事件
func (g *GroupService) recordGroupDismissedEvent(ctx context.Context, groupId string) error {
	payload, _ := json.Marshal(event.GroupDismissedEvent{
		GroupId: groupId,
	})
	return g.recordOutboxEvent(ctx, groupId, event.EventGroupDismissed, payload)
}

// recordGroupMemberLeftEvent 记录成员退群领域事件
func (g *GroupService) recordGroupMemberLeftEvent(ctx context.Context, groupId, userId string) error {
	payload, _ := json.Marshal(event.GroupMemberLeftEvent{
		GroupId: groupId,
		UserId:  userId,
	})
	return g.recordOutboxEvent(ctx, groupId, event.EventGroupMemberLeft, payload)
}

// recordGroupMemberRemovedEvent 记录成员被移出群领域事件
func (g *GroupService) recordGroupMemberRemovedEvent(ctx context.Context, groupId, operatorId string, memberUuids []string) error {
	payload, _ := json.Marshal(event.GroupMemberRemovedEvent{
		GroupId:     groupId,
		OperatorId:  operatorId,
		MemberUuids: memberUuids,
	})
	return g.recordOutboxEvent(ctx, groupId, event.EventGroupMemberRemoved, payload)
}
