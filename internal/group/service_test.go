package group

import (
	"context"
	"sync"
	"testing"
	"time"

	grouppb "kama_chat_server/api/gen/group"
	"kama_chat_server/pkg/errorx"
)

type fakeGroupStore struct {
	GroupStore
	mu     sync.Mutex
	groups map[string]*GroupInfo
}

func (f *fakeGroupStore) FindByUuid(_ context.Context, uuid string) (*GroupInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.groups[uuid]; ok {
		cpy := *g
		return &cpy, nil
	}
	return nil, grouppb.ErrorGroupNotFound("群组不存在")
}

func (f *fakeGroupStore) FindByOwnerIdAndName(_ context.Context, ownerId, name string) (*GroupInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.groups {
		if g.OwnerId == ownerId && g.Name == name && g.Status == 0 {
			cpy := *g
			return &cpy, nil
		}
	}
	return nil, grouppb.ErrorGroupNotFound("群组不存在")
}

func (f *fakeGroupStore) CreateGroup(_ context.Context, group *GroupInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.groups == nil {
		f.groups = make(map[string]*GroupInfo)
	}
	f.groups[group.Uuid] = group
	return nil
}

func (f *fakeGroupStore) Update(_ context.Context, group *GroupInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups[group.Uuid] = group
	return nil
}

func (f *fakeGroupStore) UpdateWithVersion(_ context.Context, uuid string, expectedVersion int32, updates map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.groups[uuid]
	if !ok {
		return grouppb.ErrorGroupNotFound("群组不存在")
	}
	if current.Version != expectedVersion {
		return grouppb.ErrorGroupConflict("群资料已被他人修改，请刷新后重试")
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
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.groups[uuid]
	if !ok || current.OwnerId != ownerId || current.Status != 0 {
		return false, nil
	}
	current.Status = 1
	current.Version++
	return true, nil
}

func (f *fakeGroupStore) IncrementMemberCountWithLimit(_ context.Context, uuid string, maxLimit int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.groups[uuid]
	if !ok || current.MemberCnt >= maxLimit {
		return false, nil
	}
	current.MemberCnt++
	return true, nil
}

type fakeGroupMemberStore struct {
	GroupMemberStore
	mu      sync.Mutex
	members map[string]*GroupMember
}

func (f *fakeGroupMemberStore) FindByGroupAndUser(_ context.Context, groupUuid, userUuid string) (*GroupMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := groupUuid + ":" + userUuid
	if m, ok := f.members[key]; ok {
		return m, nil
	}
	return nil, grouppb.ErrorMemberNotFound("成员不存在")
}

func (f *fakeGroupMemberStore) FindByGroupUuid(_ context.Context, groupUuid string) ([]GroupMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []GroupMember
	for _, m := range f.members {
		if m.GroupUuid == groupUuid {
			result = append(result, *m)
		}
	}
	return result, nil
}

func (f *fakeGroupMemberStore) CreateGroupMember(_ context.Context, member *GroupMember) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members == nil {
		f.members = make(map[string]*GroupMember)
	}
	f.members[member.GroupUuid+":"+member.UserUuid] = member
	return nil
}

func (f *fakeGroupMemberStore) CreateGroupMemberOrIgnore(_ context.Context, member *GroupMember) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members == nil {
		f.members = make(map[string]*GroupMember)
	}
	key := member.GroupUuid + ":" + member.UserUuid
	if _, ok := f.members[key]; ok {
		return false, nil
	}
	f.members[key] = member
	return true, nil
}

func (f *fakeGroupMemberStore) DeleteMemberIfNotOwner(_ context.Context, groupUuid, userUuid string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

		if !grouppb.IsGroupConflict(err) && !errorx.IsCode(err, errorx.CodeConflict) {
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
		if !grouppb.IsInvalidParam(err) && !errorx.IsCode(err, errorx.CodeInvalidParam) {
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
	if !grouppb.IsInvalidParam(err3) && !errorx.IsCode(err3, errorx.CodeInvalidParam) {
		t.Errorf("expected CodeInvalidParam, got %v", err3)
	}
}

func TestGroupService_AddGroupMember_CapacityLimit(t *testing.T) {
	ctx := context.Background()
	gStore := &fakeGroupStore{
		groups: map[string]*GroupInfo{
			"G1001": {
				Uuid:      "G1001",
				Name:      "满人群",
				MemberCnt: 499,
				OwnerId:   "U_OWNER",
				Status:    0,
			},
		},
	}
	mStore := &fakeGroupMemberStore{members: make(map[string]*GroupMember)}
	svc := NewGroupService(nil, gStore, mStore, nil)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = svc.AddGroupMember(ctx, "G1001", "USER_1", 1)
	}()
	go func() {
		defer wg.Done()
		errs[1] = svc.AddGroupMember(ctx, "G1001", "USER_2", 1)
	}()
	wg.Wait()

	var successCount, limitCount int
	for _, err := range errs {
		if err == nil {
			successCount++
		} else if grouppb.IsGroupLimitExceeded(err) {
			limitCount++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successCount != 1 || limitCount != 1 {
		t.Fatalf("expected 1 success and 1 limit exceeded, got %d successes and %d limit errors", successCount, limitCount)
	}

	if gStore.groups["G1001"].MemberCnt != 500 {
		t.Fatalf("expected MemberCnt to be 500, got %d", gStore.groups["G1001"].MemberCnt)
	}
}

func TestGroupService_AddGroupMember_Idempotent(t *testing.T) {
	ctx := context.Background()
	gStore := &fakeGroupStore{
		groups: map[string]*GroupInfo{
			"G1001": {
				Uuid:      "G1001",
				Name:      "测试群",
				MemberCnt: 1,
				OwnerId:   "U_OWNER",
				Status:    0,
			},
		},
	}
	mStore := &fakeGroupMemberStore{members: make(map[string]*GroupMember)}
	svc := NewGroupService(nil, gStore, mStore, nil)

	// 第一次添加
	err1 := svc.AddGroupMember(ctx, "G1001", "USER_1", 1)
	if err1 != nil {
		t.Fatalf("first AddGroupMember failed: %v", err1)
	}
	if gStore.groups["G1001"].MemberCnt != 2 {
		t.Fatalf("expected MemberCnt 2, got %d", gStore.groups["G1001"].MemberCnt)
	}

	// 重复添加同一成员：幂等成功，且人数不重复自增
	err2 := svc.AddGroupMember(ctx, "G1001", "USER_1", 1)
	if err2 != nil {
		t.Fatalf("idempotent AddGroupMember failed: %v", err2)
	}
	if gStore.groups["G1001"].MemberCnt != 2 {
		t.Fatalf("expected MemberCnt still 2 after duplicate add, got %d", gStore.groups["G1001"].MemberCnt)
	}
}
