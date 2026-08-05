package group

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/outbox"
	"kama_chat_server/internal/common/model"
)

type DomainEventConsumer struct {
	reader *kafka.Reader
	uow    groupUoW
	quit   chan os.Signal
}

func NewDomainEventConsumer(uow groupUoW) *DomainEventConsumer {
	reader := kafkainfra.NewConsumer(kafkainfra.TopicDomainEvents, "group_domain_events")
	return &DomainEventConsumer{reader: reader, uow: uow, quit: make(chan os.Signal, 1)}
}

func (c *DomainEventConsumer) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("domain event consumer panic: %v", r))
			}
		}()
		for {
			msg, err := c.reader.ReadMessage(context.Background())
			if err != nil {
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
// 目前仅处理 group_apply_passed：事务内加成员、递增人数，并发出 group_joined 事件
func (c *DomainEventConsumer) handleEvent(ctx context.Context, eventType string, payload []byte) error {
	if eventType != event.EventGroupApplyPassed {
		return nil
	}

	var e event.GroupApplyPassedEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return err
	}

	return store.WithTx(c.uow, func(tx groupUoW) error {
		newMember := model.GroupMember{
			GroupUuid: e.GroupId,
			UserUuid:  e.UserId,
			Role:      1,
		}
		if err := tx.GroupMemberStore().CreateGroupMember(ctx, &newMember); err != nil {
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
		return tx.RecordEvent(ctx, event.EventGroupJoined, joinedPayload)
	})
}
