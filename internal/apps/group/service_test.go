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
