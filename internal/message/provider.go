package message

import (
	"context"

	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/kafka"
)

// ProviderSet 集中导出 message 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewUoW,
	ProvideMessageStore,
	ProvideSessionStore,
	ProvideMessageService,
	ProvideSessionService,
	NewGrpcServer,
	ProvideEventConsumer,
)

// ProvideDB 初始化 chat_message 数据库连接并自动迁移
func ProvideDB(conf *config.Config) (*gorm.DB, error) {
	db, err := database.Open(&conf.MysqlConfig, "chat_message")
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(db, &Message{}, &Session{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideMessageStore 提供 MessageStore
func ProvideMessageStore(uow UoW) MessageStore {
	return uow.MessageStore()
}

// ProvideSessionStore 提供 SessionStore
func ProvideSessionStore(uow UoW) SessionStore {
	return uow.SessionStore()
}

// ProvideMessageService 适配 Wire 依赖注入
func ProvideMessageService(msgStore MessageStore, sessionStore SessionStore, cacheService cacheinfra.AsyncCacheService) *MessageService {
	return NewMessageService(msgStore, sessionStore, cacheService)
}

// ProvideSessionService 适配 Wire 依赖注入
func ProvideSessionService(sessionStore SessionStore, msgStore MessageStore, cacheService cacheinfra.AsyncCacheService) *SessionService {
	return NewSessionService(sessionStore, msgStore, cacheService)
}

// ProvideEventConsumer 启动会话事件消费者并导出清理函数
func ProvideEventConsumer(uow UoW, cacheService cacheinfra.AsyncCacheService) (*DomainEventConsumer, func()) {
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Fatal("failed to ensure domain_events topic", zap.Error(err))
	}
	eventHandler := NewSessionEventHandler(uow.SessionStore(), cacheService)
	eventConsumer := NewDomainEventConsumer(eventHandler)
	eventConsumer.Start()
	cleanup := func() {
		eventConsumer.Close()
	}
	return eventConsumer, cleanup
}
