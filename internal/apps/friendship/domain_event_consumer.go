package friendship

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/grpc_client"
	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/outbox"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
)

type DomainEventConsumer struct {
	reader *kafkainfra.Consumer
	uow    friendshipUoW
	cache  store.AsyncCacheService
	quit   chan os.Signal
}

func NewDomainEventConsumer(uow friendshipUoW, cache ...store.AsyncCacheService) *DomainEventConsumer {
	reader, err := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "friendship_domain_events")
	if err != nil {
		zap.L().Fatal("failed to init friendship kafka consumer", zap.Error(err))
	}
	var c store.AsyncCacheService
	if len(cache) > 0 {
		c = cache[0]
	}
	return &DomainEventConsumer{reader: reader, uow: uow, cache: c, quit: make(chan os.Signal, 1)}
}

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
			if err := c.handleEvent(context.Background(), eventType, msg.Value); err != nil {
				zap.L().Error("handle domain event error", zap.String("type", eventType), zap.Error(err))
			}
		}
	}()
}

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

	err := store.WithTx(c.uow, func(tx friendshipUoW) error {
		newFriendship := model.Friendship{
			UserId:   e.UserId,
			FriendId: e.FriendId,
			Status:   friendship_status.NORMAL,
		}
		if err := tx.FriendshipStore().CreateFriendship(ctx, &newFriendship); err != nil {
			return err
		}

		anotherFriendship := model.Friendship{
			UserId:   e.FriendId,
			FriendId: e.UserId,
			Status:   friendship_status.NORMAL,
		}
		return tx.FriendshipStore().CreateFriendship(ctx, &anotherFriendship)
	})
	if err != nil {
		return err
	}

	if c.cache != nil {
		_ = c.cache.DeleteByPattern(ctx, constants.CacheKeyFriendRelUser+e.UserId+"*")
		_ = c.cache.DeleteByPattern(ctx, constants.CacheKeyFriendRelUser+e.FriendId+"*")

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
