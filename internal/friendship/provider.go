package friendship

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 集中导出 friendship 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewStore,
	ProvideFriendshipService,
	NewGrpcServer,
	ProvideEventConsumer,
)

// ProvideDB 初始化 chat_friendship 数据库连接并自动迁移
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_friendship")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &Friendship{}, &outbox.Outbox{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideFriendshipService 适配 Wire 依赖注入
func ProvideFriendshipService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService) *FriendshipService {
	return NewFriendshipService(db, store, cacheService)
}

// ProvideEventConsumer 启动领域事件消费者并导出清理函数
func ProvideEventConsumer(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService) (*DomainEventConsumer, func()) {
	consumer := NewDomainEventConsumer(db, store, cacheService)
	consumer.Start()
	cleanup := func() {
		consumer.Close()
	}
	return consumer, cleanup
}
