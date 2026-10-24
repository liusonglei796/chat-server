package group

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 集中导出 group 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewUoW,
	ProvideGroupService,
	NewGrpcServer,
	ProvideEventConsumer,
)

// ProvideDB 初始化 chat_group 数据库连接并自动迁移
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_group")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &GroupInfo{}, &GroupMember{}, &outbox.Outbox{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideGroupService 适配 Wire 依赖注入
func ProvideGroupService(uow UoW, cacheService cacheinfra.AsyncCacheService) *GroupService {
	return NewGroupService(uow, cacheService)
}

// ProvideEventConsumer 启动群组领域事件消费者并导出清理函数
func ProvideEventConsumer(uow UoW, cacheService cacheinfra.AsyncCacheService) (*DomainEventConsumer, func()) {
	consumer := NewDomainEventConsumer(uow, cacheService)
	consumer.Start()
	cleanup := func() {
		consumer.Close()
	}
	return consumer, cleanup
}
