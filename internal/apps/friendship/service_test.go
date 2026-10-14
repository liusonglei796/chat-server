package friendship

import (
	"context"
	"testing"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/errorx"
)

type mockFriendshipStore struct {
	store.FriendshipStore
	relations map[string]*model.Friendship
}

func newMockFriendshipStore() *mockFriendshipStore {
	return &mockFriendshipStore{
		relations: make(map[string]*model.Friendship),
	}
}

func (m *mockFriendshipStore) FindByUserIdAndFriendId(_ context.Context, userId, friendId string) (*model.Friendship, error) {
	key := userId + ":" + friendId
	if fs, ok := m.relations[key]; ok {
		cpy := *fs
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "not found")
}

func (m *mockFriendshipStore) UpdateStatusWithExpected(_ context.Context, userId, friendId string, expectedStatus, targetStatus int8, expectedVersion int64) (bool, error) {
	key := userId + ":" + friendId
	fs, ok := m.relations[key]
	if !ok || fs.Status != expectedStatus {
		return false, nil // CAS 失败
	}
	if expectedVersion > 0 && fs.Version != expectedVersion {
		return false, nil // 版本不匹配
	}
	fs.Status = targetStatus
	fs.Version++
	return true, nil
}

type mockOutboxStore struct {
	events []string
}

type mockFriendshipUOW struct {
	store  *mockFriendshipStore
	outbox *mockOutboxStore
}

func (u *mockFriendshipUOW) FriendshipStore() store.FriendshipStore {
	return u.store
}

func (u *mockFriendshipUOW) RecordEvent(_ context.Context, _ string, eventType string, _ []byte) error {
	u.outbox.events = append(u.outbox.events, eventType)
	return nil
}

func (u *mockFriendshipUOW) WithTx(fn func(tx any) error) error {
	return fn(u)
}

type mockCacheService struct {
	store.AsyncCacheService
}

func (c *mockCacheService) SubmitTask(fn func()) {
	fn()
}

func (c *mockCacheService) RemoveFromSet(_ context.Context, _ string, _ ...interface{}) error {
	return nil
}

func (c *mockCacheService) AddToSet(_ context.Context, _ string, _ ...interface{}) error {
	return nil
}

func TestFriendshipService_BlackFriend_CAS_PreventsDuplicateEvents(t *testing.T) {
	// Step 1.1: 准备双方处于 NORMAL 状态的好友数据
	// 来源: 单元测试初始化
	// 用于: Step 1.2 注入并执行 CAS 拉黑
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &model.Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &model.Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	uow := &mockFriendshipUOW{store: fsStore, outbox: outbox}
	svc := NewFriendshipService(uow, &mockCacheService{})

	// Step 1.2: 首次调用拉黑，断言成功且触发一次 Outbox 事件
	// 来源: U1001 拉黑 U1002
	// 用于: 校验双向状态原子跃迁为 BLACK / BE_BLACK 且版本递增
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

	// Step 1.3: 模拟并发重试发起第二次拉黑，断言被 CAS 乐观锁拦截阻断
	// 来源: 相同 U1001 拉黑 U1002 请求
	// 用于: 验证状态机阻断能力：不重复写事件，不产生副作用
	err2 := svc.BlackFriend(ctx, "U1001", "U1002")
	if err2 == nil {
		t.Fatal("second BlackFriend should fail due to CAS check, got nil")
	}

	// 确认 Outbox 事件并未增加，依然仅有 1 次
	if len(outbox.events) != 1 {
		t.Fatalf("expected still exactly 1 event in outbox, got %d", len(outbox.events))
	}
}

func TestFriendshipService_UnblackFriend_CAS(t *testing.T) {
	// Step 2.1: 准备处于拉黑态的好友数据
	// 来源: U1001 已拉黑 U1002，版本为 2
	// 用于: Step 2.2 测试解除拉黑
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &model.Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.BLACK, Version: 2}
	fsStore.relations["U1002:U1001"] = &model.Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.BE_BLACK, Version: 2}

	outbox := &mockOutboxStore{}
	uow := &mockFriendshipUOW{store: fsStore, outbox: outbox}
	svc := NewFriendshipService(uow, &mockCacheService{})

	// Step 2.2: 首次解除拉黑，断言成功恢复为 NORMAL 且版本增至 3
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

	// Step 2.3: 重复调用解除拉黑，断言被 CAS 状态检查拦截
	err2 := svc.UnblackFriend(ctx, "U1001", "U1002")
	if err2 == nil {
		t.Fatal("second UnblackFriend should fail, got nil")
	}
}
