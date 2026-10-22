package group

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	kafkainfra "kama_chat_server/internal/infra/kafka"
	"kama_chat_server/internal/infra/metrics"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/event"
	otelx "kama_chat_server/pkg/otel"
)

// DomainEventConsumer 领域事件消费者
type DomainEventConsumer struct {
	reader *kafkainfra.Consumer
	uow    UoW
	cache  cacheinfra.AsyncCacheService
	quit   chan os.Signal
}

// NewDomainEventConsumer 构造函数
func NewDomainEventConsumer(uow UoW, cache ...cacheinfra.AsyncCacheService) *DomainEventConsumer {
	reader, err := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "group_domain_events")
	if err != nil {
		zap.L().Fatal("failed to init group kafka consumer", zap.Error(err))
	}
	var c cacheinfra.AsyncCacheService
	if len(cache) > 0 {
		c = cache[0]
	}
	return &DomainEventConsumer{reader: reader, uow: uow, cache: c, quit: make(chan os.Signal, 1)}
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
			handleCtx, span := otelx.StartKafkaConsumerSpan(context.Background(), msg.Headers, "kafka consume "+eventType)
			err = c.handleEvent(handleCtx, eventType, msg.Value)
			otelx.EndSpan(span, err)
			if err != nil {
				metrics.KafkaConsumeErrors.Inc()
				zap.L().Error("handle domain event error", zap.String("type", eventType), zap.Error(err))
				continue
			}
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
// 目前仅处理 group_apply_passed：事务内加成员、递增人数，并发出 group_joined 事件
func (c *DomainEventConsumer) handleEvent(ctx context.Context, eventType string, payload []byte) error {
	if eventType != event.EventGroupApplyPassed {
		return nil
	}

	var e event.GroupApplyPassedEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return err
	}

	err := WithTx(c.uow, func(tx UoW) error {
		newMember := GroupMember{
			GroupUuid: e.GroupId,
			UserUuid:  e.UserId,
			Role:      1,
		}
		if err := tx.GroupMemberStore().CreateGroupMember(ctx, &newMember); err != nil {
			if dberr.IsDuplicateKey(err) {
				return nil
			}
			return err
		}
		if err := tx.GroupStore().IncrementMemberCount(ctx, e.GroupId); err != nil {
			return err
		}

		grp, err := tx.GroupStore().FindByUuid(ctx, e.GroupId)
		if err != nil {
			return err
		}

		joinedPayload, _ := json.Marshal(event.GroupJoinedEvent{
			GroupId:     e.GroupId,
			UserId:      e.UserId,
			GroupName:   grp.Name,
			GroupAvatar: grp.Avatar,
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
