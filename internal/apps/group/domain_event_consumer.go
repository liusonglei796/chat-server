package group

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/outbox"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
)

type DomainEventConsumer struct {
	reader *kafkainfra.Consumer
	uow    groupUoW
	cache  store.AsyncCacheService
	quit   chan os.Signal
}

func NewDomainEventConsumer(uow groupUoW, cache store.AsyncCacheService) *DomainEventConsumer {
	reader, err := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "group_domain_events")
	if err != nil {
		zap.L().Fatal("failed to init group kafka consumer", zap.Error(err))
	}
	return &DomainEventConsumer{reader: reader, uow: uow, cache: cache, quit: make(chan os.Signal, 1)}
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
				continue // 下游处理失败，不提交位点，等待重试
			}
			// 下游存储写入成功后，后置手动同步提交位点 (commitSync)
			if err := c.reader.CommitRecord(context.Background(), msg); err != nil {
				zap.L().Error("commit kafka offset error", zap.String("type", eventType), zap.Error(err))
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
// 目前仅处理 group_apply_passed：事务内加成员、递增人数，并发出 group_joined 事件
func (c *DomainEventConsumer) handleEvent(ctx context.Context, eventType string, payload []byte) error {
	if eventType != event.EventGroupApplyPassed {
		return nil
	}

	var e event.GroupApplyPassedEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return err
	}

	err := store.WithTx(c.uow, func(tx groupUoW) error {
		newMember := model.GroupMember{
			GroupUuid: e.GroupId,
			UserUuid:  e.UserId,
			Role:      1,
		}
		if err := tx.GroupMemberStore().CreateGroupMember(ctx, &newMember); err != nil {
			if dberr.IsDuplicateKey(err) {
				// 该成员已存在（重复事件），直接返回 nil，不重复递增人数与发事件
				return nil
			}
			return err
		}
		if err := tx.GroupStore().IncrementMemberCount(ctx, e.GroupId); err != nil {
			return err
		}

		group, err := tx.GroupStore().FindByUuid(ctx, e.GroupId)
		if err != nil {
			return err
		}

		joinedPayload, _ := json.Marshal(event.GroupJoinedEvent{
			GroupId:     e.GroupId,
			UserId:      e.UserId,
			GroupName:   group.Name,
			GroupAvatar: group.Avatar,
		})
		return tx.RecordEvent(ctx, e.GroupId, event.EventGroupJoined, joinedPayload)
	})
	if err != nil {
		if dberr.IsDuplicateKey(err) {
			zap.L().Info("群成员已加入，幂等处理成功", zap.String("groupId", e.GroupId), zap.String("userId", e.UserId))
			return nil
		}
		return err
	}

	if c.cache != nil {
		_ = c.cache.Delete(ctx, constants.CacheKeyGroupMembers+e.GroupId)
		_ = c.cache.Delete(ctx, constants.CacheKeyGroupInfo+e.GroupId)
	}
	return nil
}
