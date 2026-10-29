package user

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 集中导出 user 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewStore,
	ProvideUserService,
	NewUserGrpcServer,
	ProvideAuthService,
	NewAuthGrpcServer,
)

// ProvideDB 初始化 chat_user 数据库连接并自动迁移
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_user")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &UserInfo{}, &outbox.Outbox{}, &idempotency.Record{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideUserService 适配 Wire 依赖注入
func ProvideUserService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService) *UserService {
	return NewUserService(db, store, cacheService)
}

// ProvideAuthService 适配 Wire 依赖注入
func ProvideAuthService(cache cacheinfra.AsyncCacheService, userStore Store) *AuthService {
	return NewAuthService(cache, userStore)
}
