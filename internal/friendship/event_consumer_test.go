package friendship

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"

	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/event"
)

type fakeFriendshipStore struct {
	Store
	created []Friendship
	err     error
}

func (f *fakeFriendshipStore) CreateFriendship(_ context.Context, fs *Friendship) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, *fs)
	return nil
}

type fakeUOW struct {
	friendshipStore *fakeFriendshipStore
}

func (f *fakeUOW) FriendshipStore() Store { return f.friendshipStore }

func (f *fakeUOW) RecordEvent(_ context.Context, _ string, _ string, _ []byte) error { return nil }

func (f *fakeUOW) WithTx(fn func(tx any) error) error { return fn(f) }

func TestHandleFriendApplyPassed_CreatesBidirectional(t *testing.T) {
	store := &fakeFriendshipStore{}
	c := &DomainEventConsumer{uow: &fakeUOW{friendshipStore: store}}

	payload, _ := json.Marshal(event.FriendApplyPassedEvent{UserId: "U1", FriendId: "U2"})
	if err := c.handleEvent(context.Background(), event.EventFriendApplyPassed, payload); err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}

	if len(store.created) != 2 {
		t.Fatalf("expected 2 friendships created, got %d", len(store.created))
	}

	me2friend := store.created[0]
	if me2friend.UserId != "U1" || me2friend.FriendId != "U2" || me2friend.Status != friendship_status.NORMAL {
		t.Errorf("unexpected me->friend row: %+v", me2friend)
	}

	friend2me := store.created[1]
	if friend2me.UserId != "U2" || friend2me.FriendId != "U1" || friend2me.Status != friendship_status.NORMAL {
		t.Errorf("unexpected friend->me row: %+v", friend2me)
	}
}

func TestHandleFriendApplyPassed_FirstInsertFails_ReturnsError(t *testing.T) {
	store := &fakeFriendshipStore{err: errors.New("db down")}
	c := &DomainEventConsumer{uow: &fakeUOW{friendshipStore: store}}

	payload, _ := json.Marshal(event.FriendApplyPassedEvent{UserId: "U1", FriendId: "U2"})
	if err := c.handleEvent(context.Background(), event.EventFriendApplyPassed, payload); err == nil {
		t.Fatal("expected error on first insert failure, got nil")
	}

	if len(store.created) != 0 {
		t.Fatalf("expected no rows persisted on failure, got %d", len(store.created))
	}
}

func TestHandle_UnrelatedEvent_Noop(t *testing.T) {
	store := &fakeFriendshipStore{}
	c := &DomainEventConsumer{uow: &fakeUOW{friendshipStore: store}}

	if err := c.handleEvent(context.Background(), event.EventGroupApplyPassed, []byte(`{}`)); err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}

	if len(store.created) != 0 {
		t.Fatalf("expected no rows for unrelated event, got %d", len(store.created))
	}
}

func TestHandleFriendApplyPassed_DuplicateKey_TreatedAsSuccess(t *testing.T) {
	store := &fakeFriendshipStore{err: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}}
	c := &DomainEventConsumer{uow: &fakeUOW{friendshipStore: store}}

	payload, _ := json.Marshal(event.FriendApplyPassedEvent{UserId: "U1", FriendId: "U2"})
	if err := c.handleEvent(context.Background(), event.EventFriendApplyPassed, payload); err != nil {
		t.Fatalf("expected nil error on duplicate key, got %v", err)
	}
}
