package group

import (
	"context"
	"testing"
	"time"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/request/group"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/errorx"
)

// fakeGroupStore 模拟 GroupStore
type fakeGroupStore struct {
	store.GroupStore
	groups map[string]*model.GroupInfo
}

func (f *fakeGroupStore) FindByUuid(_ context.Context, uuid string) (*model.GroupInfo, error) {
	if g, ok := f.groups[uuid]; ok {
		cpy := *g
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
}

func (f *fakeGroupStore) FindByOwnerIdAndName(_ context.Context, ownerId, name string) (*model.GroupInfo, error) {
	for _, g := range f.groups {
		if g.OwnerId == ownerId && g.Name == name && g.Status == 0 {
			cpy := *g
			return &cpy, nil
		}
	}
	return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
}

func (f *fakeGroupStore) CreateGroup(_ context.Context, group *model.GroupInfo) error {
	if f.groups == nil {
		f.groups = make(map[string]*model.GroupInfo)
	}
	f.groups[group.Uuid] = group
	return nil
}

func (f *fakeGroupStore) Update(_ context.Context, group *model.GroupInfo) error {
	f.groups[group.Uuid] = group
	return nil
}

func (f *fakeGroupStore) UpdateWithVersion(_ context.Context, uuid string, expectedVersion int32, updates map[string]interface{}) error {
	current, ok := f.groups[uuid]
	if !ok {
		return errorx.New(errorx.CodeNotFound, "群组不存在")
	}
	if current.Version != expectedVersion {
		return errorx.New(errorx.CodeConflict, "群资料已被他人修改，请刷新后重试")
	}
	for k, v := range updates {
		switch k {
		case "name":
			current.Name = v.(string)
		case "avatar":
			current.Avatar = v.(string)
		case "notice":
			current.Notice = v.(string)
		case "add_mode":
			current.AddMode = v.(int8)
		}
	}
	current.Version = expectedVersion + 1
	return nil
}

// fakeGroupMemberStore 模拟 GroupMemberStore
type fakeGroupMemberStore struct {
	store.GroupMemberStore
	members map[string]*model.GroupMember // key: groupUuid + ":" + userUuid
}

func (f *fakeGroupMemberStore) FindByGroupAndUser(_ context.Context, groupUuid, userUuid string) (*model.GroupMember, error) {
	key := groupUuid + ":" + userUuid
	if m, ok := f.members[key]; ok {
		return m, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "成员不存在")
}

func (f *fakeGroupMemberStore) FindByGroupUuid(_ context.Context, groupUuid string) ([]model.GroupMember, error) {
	var result []model.GroupMember
	for _, m := range f.members {
		if m.GroupUuid == groupUuid {
			result = append(result, *m)
		}
	}
	return result, nil
}

func (f *fakeGroupMemberStore) CreateGroupMember(_ context.Context, member *model.GroupMember) error {
	if f.members == nil {
		f.members = make(map[string]*model.GroupMember)
	}
	f.members[member.GroupUuid+":"+member.UserUuid] = member
	return nil
}

// fakeGroupUOW 满足 groupUoW 接口
type fakeGroupUOW struct {
	groupStore       *fakeGroupStore
	groupMemberStore *fakeGroupMemberStore
}

func (f *fakeGroupUOW) GroupStore() store.GroupStore {
	return f.groupStore
}

func (f *fakeGroupUOW) GroupMemberStore() store.GroupMemberStore {
	return f.groupMemberStore
}

func (f *fakeGroupUOW) RecordEvent(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (f *fakeGroupUOW) WithTx(fn func(tx any) error) error {
	return fn(f)
}

// fakeCacheService 满足 store.AsyncCacheService 接口
type fakeCacheService struct {
	store.AsyncCacheService
}

func (f *fakeCacheService) SubmitTask(task func()) {
	// 同步执行或忽略
	task()
}

func (f *fakeCacheService) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *fakeCacheService) InvalidateWithNull(_ context.Context, _ string) error {
	return nil
}

func (f *fakeCacheService) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func TestGroupService_UpdateGroupInfo_OptimisticLock(t *testing.T) {
	ctx := context.Background()

	setupService := func() (*GroupService, *fakeGroupStore) {
		gStore := &fakeGroupStore{
			groups: map[string]*model.GroupInfo{
				"G1001": {
					Uuid:    "G1001",
					Name:    "原群名",
					Version: 1,
				},
			},
		}
		mStore := &fakeGroupMemberStore{
			members: map[string]*model.GroupMember{
				"G1001:U_ADMIN": {
					GroupUuid: "G1001",
					UserUuid:  "U_ADMIN",
					Role:      2, // 管理员
				},
			},
		}
		uow := &fakeGroupUOW{
			groupStore:       gStore,
			groupMemberStore: mStore,
		}
		svc := NewGroupService(uow, &fakeCacheService{})
		return svc, gStore
	}

	t.Run("success with matching version", func(t *testing.T) {
		svc, gStore := setupService()
		newName := "新群名"
		version := int32(1)
		req := group.UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &newName,
			Version: &version,
		}

		err := svc.UpdateGroupInfo(ctx, "U_ADMIN", req)
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}

		updated := gStore.groups["G1001"]
		if updated.Name != "新群名" {
			t.Errorf("expected name '新群名', got '%s'", updated.Name)
		}
		if updated.Version != 2 {
			t.Errorf("expected version 2, got %d", updated.Version)
		}
	})

	t.Run("conflict with mismatched version", func(t *testing.T) {
		svc, _ := setupService()
		newName := "新群名"
		staleVersion := int32(0) // 过期版本号
		req := group.UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &newName,
			Version: &staleVersion,
		}

		err := svc.UpdateGroupInfo(ctx, "U_ADMIN", req)
		if err == nil {
			t.Fatal("expected conflict error, got nil")
		}

		if !errorx.IsCode(err, errorx.CodeConflict) {
			t.Errorf("expected CodeConflict (1013), got code: %d, err: %v", errorx.GetCode(err), err)
		}
	})

	t.Run("conflict version but identical content heals idempotently", func(t *testing.T) {
		svc, gStore := setupService()
		// gStore 中 G1001 的名称为 "测试群聊"
		existingName := gStore.groups["G1001"].Name
		staleVersion := int32(0) // 过期版本号，但名称与数据库一致（模拟重试）
		req := group.UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &existingName,
			Version: &staleVersion,
		}

		err := svc.UpdateGroupInfo(ctx, "U_ADMIN", req)
		if err != nil {
			t.Fatalf("expected nil err for identical content retry, got: %v", err)
		}
	})

	t.Run("error when version is missing", func(t *testing.T) {
		svc, _ := setupService()
		newName := "新群名"
		req := group.UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &newName,
			Version: nil,
		}

		err := svc.UpdateGroupInfo(ctx, "U_ADMIN", req)
		if err == nil {
			t.Fatal("expected error for missing version, got nil")
		}
		if !errorx.IsCode(err, errorx.CodeInvalidParam) {
			t.Errorf("expected CodeInvalidParam, got: %v", err)
		}
	})

	t.Run("only non-nil fields updated", func(t *testing.T) {
		svc, gStore := setupService()
		gStore.groups["G1001"].Notice = "初始公告"
		newName := "仅修改群名"
		version := int32(1)
		req := group.UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &newName,
			Notice:  nil, // 未传递公告
			Version: &version,
		}

		err := svc.UpdateGroupInfo(ctx, "U_ADMIN", req)
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}

		updated := gStore.groups["G1001"]
		if updated.Name != "仅修改群名" {
			t.Errorf("expected name '仅修改群名', got '%s'", updated.Name)
		}
		if updated.Notice != "初始公告" {
			t.Errorf("expected notice '初始公告' to remain untouched, got '%s'", updated.Notice)
		}
		if updated.Version != 2 {
			t.Errorf("expected version 2, got %d", updated.Version)
		}
	})
}

type mockTestGroupGuard struct {
	store map[string]string
}

func (m *mockTestGroupGuard) Execute(ctx context.Context, key string, leaseTTL, successTTL time.Duration, fn func(context.Context) (string, error)) (string, bool, error) {
	if val, ok := m.store[key]; ok {
		return val, true, nil // 模拟命中历史成功结果回显
	}
	res, err := fn(ctx)
	if err != nil {
		return "", false, err
	}
	m.store[key] = res
	return res, false, nil
}

func TestGroupService_CreateGroup_Idempotent(t *testing.T) {
	ctx := context.Background()

	gStore := &fakeGroupStore{groups: make(map[string]*model.GroupInfo)}
	mStore := &fakeGroupMemberStore{members: make(map[string]*model.GroupMember)}
	uow := &fakeGroupUOW{groupStore: gStore, groupMemberStore: mStore}
	guard := &mockTestGroupGuard{store: make(map[string]string)}

	svc := NewGroupService(uow, &fakeCacheService{}, guard)

	req := group.CreateGroupRequest{
		Name:        "幂等架构群",
		Notice:      "测试群公告",
		AddMode:     1,
		ClientToken: "client-token-9999",
	}

	// 1. 首次调用：成功创建群，返回群ID并持久化在 fakeGroupStore
	gid1, err1 := svc.CreateGroup(ctx, "U1001", req)
	if err1 != nil {
		t.Fatalf("first CreateGroup failed: %v", err1)
	}
	if gid1 == "" {
		t.Fatal("expected non-empty group id")
	}
	if len(gStore.groups) != 1 {
		t.Fatalf("expected 1 group in store, got %d", len(gStore.groups))
	}

	// 2. 第二次调用（相同 ClientToken 重试）：触发状态机幂等回放，返回完全一致的群ID，且并未在 store 中产生重复群
	gid2, err2 := svc.CreateGroup(ctx, "U1001", req)
	if err2 != nil {
		t.Fatalf("replay CreateGroup failed: %v", err2)
	}
	if gid2 != gid1 {
		t.Fatalf("expected replayed group id %s, got %s", gid1, gid2)
	}
	if len(gStore.groups) != 1 {
		t.Fatalf("expected still exactly 1 group in store, got %d", len(gStore.groups))
	}

	// 3. 不带 ClientToken 再次尝试创建同名群：触发业务同名校验拦截
	reqNoToken := group.CreateGroupRequest{
		Name:    "幂等架构群",
		Notice:  "不同公告",
		AddMode: 1,
	}
	_, err3 := svc.CreateGroup(ctx, "U1001", reqNoToken)
	if err3 == nil {
		t.Fatal("expected duplicate name error, got nil")
	}
	if !errorx.IsCode(err3, errorx.CodeInvalidParam) {
		t.Errorf("expected CodeInvalidParam, got %v", err3)
	}
}

