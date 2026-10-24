package group

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"kama_chat_server/pkg/event"
)

type mockConsumerGroupStore struct {
	GroupStore
	group           *GroupInfo
	incrementCalled int
}

func (m *mockConsumerGroupStore) FindByUuid(_ context.Context, uuid string) (*GroupInfo, error) {
	return m.group, nil
}

func (m *mockConsumerGroupStore) IncrementMemberCount(_ context.Context, _ string) error {
	m.incrementCalled++
	return nil
}

type mockConsumerGroupMemberStore struct {
	GroupMemberStore
	err error
}

func (m *mockConsumerGroupMemberStore) CreateGroupMember(_ context.Context, _ *GroupMember) error {
	return m.err
}

func TestGroupConsumer_DuplicateKey_TreatedAsSuccess(t *testing.T) {
	ctx := context.Background()

	t.Run("first join succeeds", func(t *testing.T) {
		gStore := &mockConsumerGroupStore{
			group: &GroupInfo{
				Uuid:   "G1001",
				Name:   "测试群",
				Avatar: "avatar.png",
			},
		}
		mStore := &mockConsumerGroupMemberStore{}

		var recordedEvt bool
		c := &DomainEventConsumer{
			groupStore:  gStore,
			memberStore: mStore,
			eventRecord: func(_ context.Context, _ *gorm.DB, _, _ string, _ []byte) error {
				recordedEvt = true
				return nil
			},
		}
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
		if !recordedEvt {
			t.Fatal("expected EventGroupJoined to be recorded")
		}
	})

	t.Run("duplicate join with 1062 error treated as success and skips increment", func(t *testing.T) {
		gStore := &mockConsumerGroupStore{
			group: &GroupInfo{
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

		var recordedEvt bool
		c := &DomainEventConsumer{
			groupStore:  gStore,
			memberStore: mStore,
			eventRecord: func(_ context.Context, _ *gorm.DB, _, _ string, _ []byte) error {
				recordedEvt = true
				return nil
			},
		}
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
		if recordedEvt {
			t.Fatal("expected EventGroupJoined NOT to be recorded on duplicate join")
		}
	})
}
