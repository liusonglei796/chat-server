package message

import (
	"context"

	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/kafka"
)

// ProviderSet 集中导出 message 领域内所有服务的依赖提供者
var ProviderSet = wire.NewSet(
	ProvideDB,
	NewMessageStore,
	NewSessionStore,
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
	if err := database.AutoMigrate(db, &Message{}, &Session{}, &idempotency.Record{}); err != nil {
		return nil, err
	}
	return db, nil
}

// ProvideMessageService 适配 Wire 依赖注入
func ProvideMessageService(db *gorm.DB, msgStore MessageStore, sessionStore SessionStore, cacheService cacheinfra.AsyncCacheService) *MessageService {
	return NewMessageService(msgStore, sessionStore, cacheService, db)
}

// ProvideSessionService 适配 Wire 依赖注入
func ProvideSessionService(sessionStore SessionStore, msgStore MessageStore, cacheService cacheinfra.AsyncCacheService) *SessionService {
	return NewSessionService(sessionStore, msgStore, cacheService)
}

// ProvideEventConsumer 启动会话事件消费者并导出清理函数
func ProvideEventConsumer(sessionStore SessionStore, cacheService cacheinfra.AsyncCacheService) (*DomainEventConsumer, func()) {
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Fatal("failed to ensure domain_events topic", zap.Error(err))
	}
	eventHandler := NewSessionEventHandler(sessionStore, cacheService)
	eventConsumer := NewDomainEventConsumer(eventHandler)
	eventConsumer.Start()
	cleanup := func() {
		eventConsumer.Close()
	}
	return eventConsumer, cleanup
}
