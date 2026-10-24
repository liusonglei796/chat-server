package apply

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/outbox"
)

// ProviderSet 集中导出 apply 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewUoW,
	ProvideApplyService,
	NewGrpcServer,
)

// ProvideDB 初始化 chat_apply 数据库连接并自动迁移
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_apply")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &Apply{}, &outbox.Outbox{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideApplyService 适配 Wire 依赖注入
func ProvideApplyService(uow UoW, cacheService cacheinfra.AsyncCacheService) *ApplyService {
	return NewApplyService(uow, cacheService)
}
