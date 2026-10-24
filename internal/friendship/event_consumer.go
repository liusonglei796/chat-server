package friendship

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/grpc_client"
	kafkainfra "kama_chat_server/internal/infra/kafka"
	"kama_chat_server/internal/infra/metrics"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/event"
	otelx "kama_chat_server/pkg/otel"
)

// DomainEventConsumer 领域事件消费者
type DomainEventConsumer struct {
	reader *kafkainfra.Consumer
	db     *gorm.DB
	store  Store
	cache  cacheinfra.AsyncCacheService
	quit   chan os.Signal
}

// NewDomainEventConsumer 构造函数
func NewDomainEventConsumer(db *gorm.DB, store Store, cache ...cacheinfra.AsyncCacheService) *DomainEventConsumer {
	reader, err := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "friendship_domain_events")
	if err != nil {
		zap.L().Fatal("failed to init friendship kafka consumer", zap.Error(err))
	}
	var c cacheinfra.AsyncCacheService
	if len(cache) > 0 {
		c = cache[0]
	}
	return &DomainEventConsumer{reader: reader, db: db, store: store, cache: c, quit: make(chan os.Signal, 1)}
}

// Start 启动后台消费协程
func (c *DomainEventConsumer) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("domain event consumer panic: %v", r))
			}
		}()
		for {
			msg, err := c.reader.ReadRecord(context.Background())
			if err != nil {
				if kafkainfra.IsClosed(err) {
					return
				}
				zap.L().Error("read domain event error", zap.Error(err))
				continue
			}
			eventType := outbox.ExtractEventType(msg.Headers)
			// 从消息头还原上游 trace context，并以 producer span 的子 span 承载本次处理
			handleCtx, span := otelx.StartKafkaConsumerSpan(context.Background(), msg.Headers, "kafka consume "+eventType)
			err = c.handleEvent(handleCtx, eventType, msg.Value)
			otelx.EndSpan(span, err)
			if err != nil {
				metrics.KafkaConsumeErrors.Inc()
				zap.L().Error("handle domain event error", zap.String("type", eventType), zap.Error(err))
				continue // 下游处理失败，不提交位点，等待重试
			}
			// 下游存储写入成功后，后置手动同步提交位点 (commitSync)
			if err := c.reader.CommitRecord(context.Background(), msg); err != nil {
				zap.L().Error("commit kafka offset error", zap.String("type", eventType), zap.Error(err))
			}
		}
	}()
}

// Close 关闭 Kafka 消费客户端
func (c *DomainEventConsumer) Close() {
	if c.reader != nil {
		c.reader.Close()
	}
}

// handleEvent 处理领域事件
// 目前仅处理 friend_apply_passed：事务内建立双向好友关系，任一失败整体回滚，错误上抛以便重试
func (c *DomainEventConsumer) handleEvent(ctx context.Context, eventType string, payload []byte) error {
	if eventType != event.EventFriendApplyPassed {
		return nil
	}

	var e event.FriendApplyPassedEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return err
	}

	execTx := func(fn func(txStore Store) error) error {
		if c.db == nil {
			return fn(c.store)
		}
		return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(NewStore(tx))
		})
	}

	err := execTx(func(txStore Store) error {
		newFriendship := Friendship{
			UserId:   e.UserId,
			FriendId: e.FriendId,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := txStore.CreateFriendship(ctx, &newFriendship); err != nil {
			if dberr.IsDuplicateKey(err) {
				return nil
			}
			return err
		}

		anotherFriendship := Friendship{
			UserId:   e.FriendId,
			FriendId: e.UserId,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := txStore.CreateFriendship(ctx, &anotherFriendship); err != nil {
			if dberr.IsDuplicateKey(err) {
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		if dberr.IsDuplicateKey(err) {
			zap.L().Info("好友关系已存在，幂等处理成功", zap.String("userId", e.UserId), zap.String("friendId", e.FriendId))
			return nil
		}
		return err
	}

	if c.cache != nil {
		if gatewayAddr, err := c.cache.Get(ctx, constants.CacheKeyUserGateway+e.FriendId); err == nil && gatewayAddr != "" {
			notice, _ := json.Marshal(map[string]interface{}{
				"type":      "system",
				"event":     "friend_apply_passed",
				"user_id":   e.UserId,
				"friend_id": e.FriendId,
			})
			msgUuid := "FRND_PASS_" + snowflake.GenerateIDString()
			_ = grpc_client.PushToGateway(ctx, gatewayAddr, e.FriendId, msgUuid, notice)
		}
	}
	return nil
}
