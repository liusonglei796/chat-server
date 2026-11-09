package friendship

import (
	"context"
	"sync"
	"testing"

	"gorm.io/gorm"

	friendshippb "kama_chat_server/api/gen/friendship"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
)

type mockFriendshipStore struct {
	Store
	mu        sync.Mutex
	relations map[string]*Friendship
}

func newMockFriendshipStore() *mockFriendshipStore {
	return &mockFriendshipStore{
		relations: make(map[string]*Friendship),
	}
}

func (m *mockFriendshipStore) FindByUserIdAndFriendId(_ context.Context, userId, friendId string) (*Friendship, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := userId + ":" + friendId
	if fs, ok := m.relations[key]; ok {
		cpy := *fs
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "not found")
}

func (m *mockFriendshipStore) UpdateStatusWithExpected(_ context.Context, userId, friendId string, expectedStatus, targetStatus int8, expectedVersion int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := userId + ":" + friendId
	fs, ok := m.relations[key]
	if !ok || fs.Status != expectedStatus {
		return false, nil
	}
	if expectedVersion > 0 && fs.Version != expectedVersion {
		return false, nil
	}
	fs.Status = targetStatus
	fs.Version++
	return true, nil
}

func (m *mockFriendshipStore) CreateFriendship(_ context.Context, fs *Friendship) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fs.UserId + ":" + fs.FriendId
	cpy := *fs
	m.relations[key] = &cpy
	return nil
}

func (m *mockFriendshipStore) UpsertFriendship(_ context.Context, fs *Friendship) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fs.UserId + ":" + fs.FriendId
	if existing, ok := m.relations[key]; ok {
		existing.Status = fs.Status
		existing.Version++
		return nil
	}
	cpy := *fs
	m.relations[key] = &cpy
	return nil
}

func (m *mockFriendshipStore) SoftDelete(_ context.Context, userId, friendId string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.relations, userId+":"+friendId)
	return nil
}

type mockOutboxStore struct {
	events []string
}

func newTestFriendshipService(store Store, outbox *mockOutboxStore) *FriendshipService {
	svc := NewFriendshipService(nil, store)
	svc.eventRecord = func(_ context.Context, _ *gorm.DB, _, eventType string, _ []byte) error {
		outbox.events = append(outbox.events, eventType)
		return nil
	}
	return svc
}

func TestFriendshipService_BlackFriend_CAS_PreventsDuplicateEvents(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	svc := newTestFriendshipService(fsStore, outbox)

	err1 := svc.BlackFriend(ctx, "U1001", "U1002")
	if err1 != nil {
		t.Fatalf("first BlackFriend should succeed, got %v", err1)
	}

	if fsStore.relations["U1001:U1002"].Status != friendship_status.BLACK {
		t.Fatalf("expected status BLACK, got %d", fsStore.relations["U1001:U1002"].Status)
	}
	if fsStore.relations["U1001:U1002"].Version != 2 {
		t.Fatalf("expected version 2, got %d", fsStore.relations["U1001:U1002"].Version)
	}
	if fsStore.relations["U1002:U1001"].Status != friendship_status.BE_BLACK {
		t.Fatalf("expected status BE_BLACK, got %d", fsStore.relations["U1002:U1001"].Status)
	}
	if fsStore.relations["U1002:U1001"].Version != 2 {
		t.Fatalf("expected version 2, got %d", fsStore.relations["U1002:U1001"].Version)
	}
	if len(outbox.events) != 1 || outbox.events[0] != event.EventFriendBlacked {
		t.Fatalf("expected exactly 1 EventFriendBlacked, got: %v", outbox.events)
	}

	err2 := svc.BlackFriend(ctx, "U1001", "U1002")
	if err2 == nil {
		t.Fatal("second BlackFriend should fail due to CAS check, got nil")
	}

	if len(outbox.events) != 1 {
		t.Fatalf("expected still exactly 1 event in outbox, got %d", len(outbox.events))
	}
}

func TestFriendshipService_UnblackFriend_CAS(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.BLACK, Version: 2}
	fsStore.relations["U1002:U1001"] = &Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.BE_BLACK, Version: 2}

	outbox := &mockOutboxStore{}
	svc := newTestFriendshipService(fsStore, outbox)

	err1 := svc.UnblackFriend(ctx, "U1001", "U1002")
	if err1 != nil {
		t.Fatalf("first UnblackFriend should succeed, got %v", err1)
	}

	if fsStore.relations["U1001:U1002"].Status != friendship_status.NORMAL {
		t.Fatalf("expected status NORMAL, got %d", fsStore.relations["U1001:U1002"].Status)
	}
	if fsStore.relations["U1001:U1002"].Version != 3 {
		t.Fatalf("expected version 3, got %d", fsStore.relations["U1001:U1002"].Version)
	}
	if fsStore.relations["U1002:U1001"].Status != friendship_status.NORMAL {
		t.Fatalf("expected reverse status NORMAL, got %d", fsStore.relations["U1002:U1001"].Status)
	}
	if fsStore.relations["U1002:U1001"].Version != 3 {
		t.Fatalf("expected version 3, got %d", fsStore.relations["U1002:U1001"].Version)
	}

	err2 := svc.UnblackFriend(ctx, "U1001", "U1002")
	if err2 == nil {
		t.Fatal("second UnblackFriend should fail, got nil")
	}
}

func TestFriendshipService_DeleteFriend_Unilateral(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	svc := newTestFriendshipService(fsStore, outbox)

	err := svc.DeleteFriend(ctx, "U1001", "U1002")
	if err != nil {
		t.Fatalf("DeleteFriend failed: %v", err)
	}

	if _, ok := fsStore.relations["U1001:U1002"]; ok {
		t.Fatal("expected U1001:U1002 to be deleted")
	}

	if _, ok := fsStore.relations["U1002:U1001"]; !ok {
		t.Fatal("expected U1002:U1001 to still exist (unilateral deletion)")
	}

	if len(outbox.events) != 1 || outbox.events[0] != event.EventFriendDeleted {
		t.Fatalf("expected EventFriendDeleted, got: %v", outbox.events)
	}

	errRepeat := svc.DeleteFriend(ctx, "U1001", "U1002")
	if errRepeat != nil {
		t.Fatalf("repeat DeleteFriend should succeed idempotently, got: %v", errRepeat)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("repeat DeleteFriend should not emit new events, got %d", len(outbox.events))
	}
}

func TestFriendshipService_ABA_Defense(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	svc := newTestFriendshipService(fsStore, outbox)

	errBlack := svc.BlackFriend(ctx, "U1001", "U1002", 1)
	if errBlack != nil {
		t.Fatalf("BlackFriend should succeed, got: %v", errBlack)
	}
	if fsStore.relations["U1001:U1002"].Status != friendship_status.BLACK || fsStore.relations["U1001:U1002"].Version != 2 {
		t.Fatalf("expected status BLACK and version 2, got status=%d version=%d",
			fsStore.relations["U1001:U1002"].Status, fsStore.relations["U1001:U1002"].Version)
	}

	errUnblack := svc.UnblackFriend(ctx, "U1001", "U1002", 2)
	if errUnblack != nil {
		t.Fatalf("UnblackFriend should succeed, got: %v", errUnblack)
	}
	if fsStore.relations["U1001:U1002"].Status != friendship_status.NORMAL || fsStore.relations["U1001:U1002"].Version != 3 {
		t.Fatalf("expected status NORMAL and version 3, got status=%d version=%d",
			fsStore.relations["U1001:U1002"].Status, fsStore.relations["U1001:U1002"].Version)
	}

	errStale := svc.BlackFriend(ctx, "U1001", "U1002", 1)
	if errStale == nil {
		t.Fatal("expected stale request (expectedVersion=1) to fail on ABA check, but it succeeded!")
	}

	if !friendshippb.IsFriendshipConflict(errStale) {
		t.Fatalf("expected FriendshipConflict for stale ABA request, got: %v", errStale)
	}

	if fsStore.relations["U1001:U1002"].Status != friendship_status.NORMAL {
		t.Fatalf("state was corrupted by ABA request! expected NORMAL, got: %d", fsStore.relations["U1001:U1002"].Status)
	}
	if fsStore.relations["U1001:U1002"].Version != 3 {
		t.Fatalf("expected version to remain 3, got: %d", fsStore.relations["U1001:U1002"].Version)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("expected exactly 1 outbox event, got: %d", len(outbox.events))
	}
}

func TestFriendshipService_EstablishFriendship_ConcurrentBidirectional(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	svc := NewFriendshipService(nil, fsStore)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = svc.EstablishFriendship(ctx, "USER_A", "USER_B")
	}()
	go func() {
		defer wg.Done()
		errs[1] = svc.EstablishFriendship(ctx, "USER_B", "USER_A")
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d returned error: %v", i, err)
		}
	}

	ab, ok1 := fsStore.relations["USER_A:USER_B"]
	ba, ok2 := fsStore.relations["USER_B:USER_A"]
	if !ok1 || ab.Status != friendship_status.NORMAL {
		t.Fatalf("expected USER_A:USER_B to be NORMAL, got: %+v", ab)
	}
	if !ok2 || ba.Status != friendship_status.NORMAL {
		t.Fatalf("expected USER_B:USER_A to be NORMAL, got: %+v", ba)
	}
}
