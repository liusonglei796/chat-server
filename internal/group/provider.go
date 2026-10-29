package group

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 中的 group 服务提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewGroupStore,
	NewGroupMemberStore,
	ProvideGroupService,
	NewGrpcServer,
	ProvideEventConsumer,
)

// ProvideDB 初始化 chat_group 数据库连接并自动迁移表结构
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_group")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &GroupInfo{}, &GroupMember{}, &outbox.Outbox{}, &idempotency.Record{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideGroupService 供 Wire 依赖注入
func ProvideGroupService(
	db *gorm.DB,
	groupStore GroupStore,
	memberStore GroupMemberStore,
	cacheService cacheinfra.AsyncCacheService,
) *GroupService {
	return NewGroupService(db, groupStore, memberStore, cacheService)
}

// ProvideEventConsumer 群领域事件消费者
func ProvideEventConsumer(
	db *gorm.DB,
	groupStore GroupStore,
	memberStore GroupMemberStore,
	cacheService cacheinfra.AsyncCacheService,
) (*DomainEventConsumer, func()) {
	consumer := NewDomainEventConsumer(db, groupStore, memberStore, cacheService)
	consumer.Start()
	cleanup := func() {
		consumer.Close()
	}
	return consumer, cleanup
}
