package group

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-sql-driver/mysql"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/model"
)

type mockConsumerGroupStore struct {
	store.GroupStore
	group           *model.GroupInfo
	incrementCalled int
}

func (m *mockConsumerGroupStore) FindByUuid(_ context.Context, uuid string) (*model.GroupInfo, error) {
	return m.group, nil
}

func (m *mockConsumerGroupStore) IncrementMemberCount(_ context.Context, _ string) error {
	m.incrementCalled++
	return nil
}

type mockConsumerGroupMemberStore struct {
	store.GroupMemberStore
	err error
}

func (m *mockConsumerGroupMemberStore) CreateGroupMember(_ context.Context, _ *model.GroupMember) error {
	return m.err
}

type mockConsumerUOW struct {
	gStore      *mockConsumerGroupStore
	mStore      *mockConsumerGroupMemberStore
	eventTypes  []string
	recordedEvt bool
}

func (m *mockConsumerUOW) GroupStore() store.GroupStore {
	return m.gStore
}

func (m *mockConsumerUOW) GroupMemberStore() store.GroupMemberStore {
	return m.mStore
}

func (m *mockConsumerUOW) RecordEvent(_ context.Context, _ string, eventType string, _ []byte) error {
	m.recordedEvt = true
	m.eventTypes = append(m.eventTypes, eventType)
	return nil
}

func (m *mockConsumerUOW) WithTx(fn func(tx any) error) error {
	return fn(m)
}

func TestGroupConsumer_DuplicateKey_TreatedAsSuccess(t *testing.T) {
	ctx := context.Background()

	t.Run("first join succeeds", func(t *testing.T) {
		gStore := &mockConsumerGroupStore{
			group: &model.GroupInfo{
				Uuid:   "G1001",
				Name:   "测试群",
				Avatar: "avatar.png",
			},
		}
		mStore := &mockConsumerGroupMemberStore{}
		uow := &mockConsumerUOW{
			gStore: gStore,
			mStore: mStore,
		}

		c := NewDomainEventConsumer(uow, nil)
		payload, _ := json.Marshal(event.GroupApplyPassedEvent{
			GroupId: "G1001",
			UserId:  "U1001",
		})

		err := c.handleEvent(ctx, event.EventGroupApplyPassed, payload)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if gStore.incrementCalled != 1 {
			t.Fatalf("expected incrementCalled=1, got %d", gStore.incrementCalled)
		}
		if !uow.recordedEvt {
			t.Fatal("expected EventGroupJoined to be recorded")
		}
	})

	t.Run("duplicate join with 1062 error treated as success and skips increment", func(t *testing.T) {
		gStore := &mockConsumerGroupStore{
			group: &model.GroupInfo{
				Uuid:   "G1001",
				Name:   "测试群",
				Avatar: "avatar.png",
			},
		}
		mStore := &mockConsumerGroupMemberStore{
			err: &mysql.MySQLError{
				Number:  1062,
				Message: "Duplicate entry for key 'idx_group_user'",
			},
		}
		uow := &mockConsumerUOW{
			gStore: gStore,
			mStore: mStore,
		}

		c := NewDomainEventConsumer(uow, nil)
		payload, _ := json.Marshal(event.GroupApplyPassedEvent{
			GroupId: "G1001",
			UserId:  "U1001",
		})

		err := c.handleEvent(ctx, event.EventGroupApplyPassed, payload)
		if err != nil {
			t.Fatalf("expected nil error on duplicate join, got %v", err)
		}
		if gStore.incrementCalled != 0 {
			t.Fatalf("expected incrementCalled=0 on duplicate join, got %d", gStore.incrementCalled)
		}
		if uow.recordedEvt {
			t.Fatal("expected EventGroupJoined NOT to be recorded on duplicate join")
		}
	})
}
