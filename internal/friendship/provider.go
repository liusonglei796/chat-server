package friendship

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 中的 friendship 服务提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewStore,
	ProvideFriendshipService,
	NewGrpcServer,
	ProvideEventConsumer,
)

// ProvideDB 初始化 chat_friendship 数据库连接并自动迁移表结构
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_friendship")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &Friendship{}, &outbox.Outbox{}, &idempotency.Record{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideFriendshipService 供 Wire 依赖注入
func ProvideFriendshipService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService) *FriendshipService {
	return NewFriendshipService(db, store, cacheService)
}

// ProvideEventConsumer 事件消费者
func ProvideEventConsumer(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService) (*DomainEventConsumer, func()) {
	consumer := NewDomainEventConsumer(db, store, cacheService)
	consumer.Start()
	cleanup := func() {
		consumer.Close()
	}
	return consumer, cleanup
}
