package group

import (
	"context"
	"testing"
	"time"

	"kama_chat_server/pkg/errorx"
)

type fakeGroupStore struct {
	GroupStore
	groups map[string]*GroupInfo
}

func (f *fakeGroupStore) FindByUuid(_ context.Context, uuid string) (*GroupInfo, error) {
	if g, ok := f.groups[uuid]; ok {
		cpy := *g
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
}

func (f *fakeGroupStore) FindByOwnerIdAndName(_ context.Context, ownerId, name string) (*GroupInfo, error) {
	for _, g := range f.groups {
		if g.OwnerId == ownerId && g.Name == name && g.Status == 0 {
			cpy := *g
			return &cpy, nil
		}
	}
	return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
}

func (f *fakeGroupStore) CreateGroup(_ context.Context, group *GroupInfo) error {
	if f.groups == nil {
		f.groups = make(map[string]*GroupInfo)
	}
	f.groups[group.Uuid] = group
	return nil
}

func (f *fakeGroupStore) Update(_ context.Context, group *GroupInfo) error {
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

func (f *fakeGroupStore) DismissGroupWithCAS(_ context.Context, uuid, ownerId string) (bool, error) {
	current, ok := f.groups[uuid]
	if !ok || current.OwnerId != ownerId || current.Status != 0 {
		return false, nil
	}
	current.Status = 1
	current.Version++
	return true, nil
}

func (f *fakeGroupStore) IncrementMemberCountWithLimit(_ context.Context, uuid string, maxLimit int) (bool, error) {
	current, ok := f.groups[uuid]
	if !ok || current.MemberCnt >= maxLimit {
		return false, nil
	}
	current.MemberCnt++
	return true, nil
}

type fakeGroupMemberStore struct {
	GroupMemberStore
	members map[string]*GroupMember
}

func (f *fakeGroupMemberStore) FindByGroupAndUser(_ context.Context, groupUuid, userUuid string) (*GroupMember, error) {
	key := groupUuid + ":" + userUuid
	if m, ok := f.members[key]; ok {
		return m, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "成员不存在")
}

func (f *fakeGroupMemberStore) FindByGroupUuid(_ context.Context, groupUuid string) ([]GroupMember, error) {
	var result []GroupMember
	for _, m := range f.members {
		if m.GroupUuid == groupUuid {
			result = append(result, *m)
		}
	}
	return result, nil
}

func (f *fakeGroupMemberStore) CreateGroupMember(_ context.Context, member *GroupMember) error {
	if f.members == nil {
		f.members = make(map[string]*GroupMember)
	}
	f.members[member.GroupUuid+":"+member.UserUuid] = member
	return nil
}

func (f *fakeGroupMemberStore) DeleteMemberIfNotOwner(_ context.Context, groupUuid, userUuid string) (bool, error) {
	key := groupUuid + ":" + userUuid
	m, ok := f.members[key]
	if !ok || m.Role == 3 {
		return false, nil
	}
	delete(f.members, key)
	return true, nil
}



func TestGroupService_UpdateGroupInfo_OptimisticLock(t *testing.T) {
	ctx := context.Background()

	setupService := func() (*GroupService, *fakeGroupStore) {
		gStore := &fakeGroupStore{
			groups: map[string]*GroupInfo{
				"G1001": {
					Uuid:    "G1001",
					Name:    "原群名",
					Version: 1,
				},
			},
		}
		mStore := &fakeGroupMemberStore{
			members: map[string]*GroupMember{
				"G1001:U_ADMIN": {
					GroupUuid: "G1001",
					UserUuid:  "U_ADMIN",
					Role:      2,
				},
			},
		}
		svc := NewGroupService(nil, gStore, mStore, nil)
		return svc, gStore
	}

	t.Run("success with matching version", func(t *testing.T) {
		svc, gStore := setupService()
		newName := "新群名"
		version := int32(1)
		req := UpdateGroupInfoRequest{
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
		staleVersion := int32(0)
		req := UpdateGroupInfoRequest{
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
		existingName := gStore.groups["G1001"].Name
		staleVersion := int32(0)
		req := UpdateGroupInfoRequest{
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
		req := UpdateGroupInfoRequest{
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
		req := UpdateGroupInfoRequest{
			Uuid:    "G1001",
			Name:    &newName,
			Notice:  nil,
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
		return val, true, nil
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

	gStore := &fakeGroupStore{groups: make(map[string]*GroupInfo)}
	mStore := &fakeGroupMemberStore{members: make(map[string]*GroupMember)}
	guard := &mockTestGroupGuard{store: make(map[string]string)}
	svc := NewGroupService(nil, gStore, mStore, nil, guard)

	req := CreateGroupRequest{
		Name:        "幂等架构群",
		Notice:      "测试群公告",
		AddMode:     1,
		ClientToken: "client-token-9999",
	}

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

	reqNoToken := CreateGroupRequest{
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
