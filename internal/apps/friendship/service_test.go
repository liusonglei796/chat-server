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

func (m *mockFriendshipStore) SoftDelete(_ context.Context, userId, friendId string) error {
	delete(m.relations, userId+":"+friendId)
	return nil
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

func TestFriendshipService_DeleteFriend_Unilateral(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &model.Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &model.Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	uow := &mockFriendshipUOW{store: fsStore, outbox: outbox}
	svc := NewFriendshipService(uow, &mockCacheService{})

	// U1001 单向删除 U1002
	err := svc.DeleteFriend(ctx, "U1001", "U1002")
	if err != nil {
		t.Fatalf("DeleteFriend failed: %v", err)
	}

	// 验证：U1001 -> U1002 物理删除
	if _, ok := fsStore.relations["U1001:U1002"]; ok {
		t.Fatal("expected U1001:U1002 to be deleted")
	}

	// 核心验证：U1002 -> U1001 依然保留（单向好友/“僵尸粉”）
	if _, ok := fsStore.relations["U1002:U1001"]; !ok {
		t.Fatal("expected U1002:U1001 to still exist (unilateral deletion)")
	}

	// 验证 Outbox 事件触发 EventFriendDeleted
	if len(outbox.events) != 1 || outbox.events[0] != event.EventFriendDeleted {
		t.Fatalf("expected EventFriendDeleted, got: %v", outbox.events)
	}

	// 幂等测试：重复删除静默成功
	errRepeat := svc.DeleteFriend(ctx, "U1001", "U1002")
	if errRepeat != nil {
		t.Fatalf("repeat DeleteFriend should succeed idempotently, got: %v", errRepeat)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("repeat DeleteFriend should not emit new events, got %d", len(outbox.events))
	}
}

// TestFriendshipService_ABA_Defense 严格验证可逆循环状态机（NORMAL ⇄ BLACK）下的 ABA 防御能力
// 场景重现：
// 1. 状态 A：初始为 NORMAL (version = 1)，模拟产生迟到请求 Req_Stale (基于 version=1 发起拉黑)
// 2. 状态 B：用户发起拉黑，跃迁至 BLACK (version = 2)
// 3. 回到状态 A：用户立即反悔，解除拉黑，回到 NORMAL (version = 3)
// 4. 迟到请求到达：Req_Stale (期望版本 1) 到达服务器。虽然此时状态值碰巧又是 NORMAL，但因版本号已增至 3，
//    CAS 乐观锁成功拦截该迟到请求，防止其将已解除拉黑的好友错误地重新拉黑！
func TestFriendshipService_ABA_Defense(t *testing.T) {
	ctx := context.Background()
	fsStore := newMockFriendshipStore()
	fsStore.relations["U1001:U1002"] = &model.Friendship{UserId: "U1001", FriendId: "U1002", Status: friendship_status.NORMAL, Version: 1}
	fsStore.relations["U1002:U1001"] = &model.Friendship{UserId: "U1002", FriendId: "U1001", Status: friendship_status.NORMAL, Version: 1}

	outbox := &mockOutboxStore{}
	uow := &mockFriendshipUOW{store: fsStore, outbox: outbox}
	svc := NewFriendshipService(uow, &mockCacheService{})

	// 阶段一：正常发起拉黑 (状态 A -> B: NORMAL -> BLACK, Version 1 -> 2)
	errBlack := svc.BlackFriend(ctx, "U1001", "U1002", 1)
	if errBlack != nil {
		t.Fatalf("BlackFriend should succeed, got: %v", errBlack)
	}
	if fsStore.relations["U1001:U1002"].Status != friendship_status.BLACK || fsStore.relations["U1001:U1002"].Version != 2 {
		t.Fatalf("expected status BLACK and version 2, got status=%d version=%d",
			fsStore.relations["U1001:U1002"].Status, fsStore.relations["U1001:U1002"].Version)
	}

	// 阶段二：用户立刻取消拉黑 (状态 B -> A: BLACK -> NORMAL, Version 2 -> 3)
	// 此时业务状态值重新回到了 NORMAL (0)
	errUnblack := svc.UnblackFriend(ctx, "U1001", "U1002", 2)
	if errUnblack != nil {
		t.Fatalf("UnblackFriend should succeed, got: %v", errUnblack)
	}
	if fsStore.relations["U1001:U1002"].Status != friendship_status.NORMAL || fsStore.relations["U1001:U1002"].Version != 3 {
		t.Fatalf("expected status NORMAL and version 3, got status=%d version=%d",
			fsStore.relations["U1001:U1002"].Status, fsStore.relations["U1001:U1002"].Version)
	}

	// 阶段三：模拟弱网滞留的旧请求 Req_Stale (基于旧版本 version = 1 尝试拉黑) 到达！
	// 核心验证点：虽然当前 Status 碰巧就是 NORMAL，但 expectedVersion=1 已被判定过期
	errStale := svc.BlackFriend(ctx, "U1001", "U1002", 1)
	if errStale == nil {
		t.Fatal("expected stale request (expectedVersion=1) to fail on ABA check, but it succeeded!")
	}

	// 验证错误码必须为 CodeConflict (版本冲突/请求过期)
	codeErr, ok := errStale.(*errorx.CodeError)
	if !ok || codeErr.Code != errorx.CodeConflict {
		t.Fatalf("expected CodeConflict for stale ABA request, got: %v", errStale)
	}

	// 阶段四：验证数据完整性与无副作用
	// 1. 最终状态必须仍然是用户最后点击的 NORMAL，绝不能被迟到请求篡改为 BLACK
	if fsStore.relations["U1001:U1002"].Status != friendship_status.NORMAL {
		t.Fatalf("state was corrupted by ABA request! expected NORMAL, got: %d", fsStore.relations["U1001:U1002"].Status)
	}
	// 2. 版本号仍然保持为 3
	if fsStore.relations["U1001:U1002"].Version != 3 {
		t.Fatalf("expected version to remain 3, got: %d", fsStore.relations["U1001:U1002"].Version)
	}
	// 3. Outbox 事件仅有阶段一的拉黑事件，迟到请求没有触发重复事件
	if len(outbox.events) != 1 {
		t.Fatalf("expected exactly 1 outbox event, got: %d", len(outbox.events))
	}
}
