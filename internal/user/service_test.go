package user

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/pkg/errorx"
)

type fakeUserStore struct {
	Store
	users map[string]UserInfo
}

func (f *fakeUserStore) FindByUuids(_ context.Context, uuids []string) ([]UserInfo, error) {
	result := make([]UserInfo, 0, len(uuids))
	for _, uuid := range uuids {
		if usr, ok := f.users[uuid]; ok {
			result = append(result, usr)
		}
	}
	return result, nil
}

func (f *fakeUserStore) FindByUuid(_ context.Context, uuid string) (*UserInfo, error) {
	if usr, ok := f.users[uuid]; ok {
		return &usr, nil
	}
	return nil, dberr.WrapDBError(gorm.ErrRecordNotFound, "查询用户")
}

func (f *fakeUserStore) UpdateUserInfo(_ context.Context, user *UserInfo) error {
	if _, ok := f.users[user.Uuid]; !ok {
		return dberr.WrapDBError(gorm.ErrRecordNotFound, "更新用户")
	}
	f.users[user.Uuid] = *user
	return nil
}

func (f *fakeUserStore) UpdateUserInfoWithVersion(_ context.Context, user *UserInfo, expectedVersion int32) (bool, error) {
	current, ok := f.users[user.Uuid]
	if !ok {
		return false, dberr.WrapDBError(gorm.ErrRecordNotFound, "更新用户")
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return false, nil
	}
	user.Version = expectedVersion + 1
	f.users[user.Uuid] = *user
	return true, nil
}

func (f *fakeUserStore) FindByNickname(_ context.Context, nickname string) (*UserInfo, error) {
	for _, u := range f.users {
		if u.Nickname == nickname {
			cpy := u
			return &cpy, nil
		}
	}
	return nil, dberr.WrapDBError(gorm.ErrRecordNotFound, "查询用户")
}

func (f *fakeUserStore) CreateUser(_ context.Context, user *UserInfo) error {
	for _, u := range f.users {
		if u.Nickname == user.Nickname {
			return errorx.New(errorx.CodeUserExist, "用户名已被占用")
		}
	}
	if f.users == nil {
		f.users = make(map[string]UserInfo)
	}
	f.users[user.Uuid] = *user
	return nil
}

func TestBatchGetPublicUserInfo_PreservesOrderAndFiltersMissing(t *testing.T) {
	store := &fakeUserStore{
		users: map[string]UserInfo{
			"U1": {Uuid: "U1", Nickname: "Alice", Avatar: "a.png", Gender: 1, Birthday: "20000101", Signature: "hi"},
			"U3": {Uuid: "U3", Nickname: "Charlie", Avatar: "c.png", Gender: 0, Birthday: "19991231", Signature: "bye"},
		},
	}
	svc := NewUserService(nil, store, nil)

	rsp, err := svc.BatchGetPublicUserInfo(context.Background(), []string{"U1", "U2", "U3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rsp) != 2 {
		t.Fatalf("expected 2 users, got %d", len(rsp))
	}
	if rsp[0].Uuid != "U1" || rsp[0].Nickname != "Alice" {
		t.Errorf("unexpected first user: %+v", rsp[0])
	}
	if rsp[1].Uuid != "U3" || rsp[1].Nickname != "Charlie" {
		t.Errorf("unexpected second user: %+v", rsp[1])
	}
}

func TestUpdateUserInfo_PartialFieldsOnly(t *testing.T) {
	store := &fakeUserStore{
		users: map[string]UserInfo{
			"U1": {
				Uuid:      "U1",
				Nickname:  "OldName",
				Email:     "old@test.com",
				Birthday:  "20000101",
				Signature: "old sig",
				Avatar:    "old.png",
				Version:   1,
			},
		},
	}
	svc := NewUserService(nil, store, nil)

	newName := "NewName"
	newSig := "new sig"
	err := svc.UpdateUserInfo(context.Background(), "U1", UpdateUserInfoRequest{
		Nickname:  &newName,
		Signature: &newSig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := store.users["U1"]
	if updated.Nickname != "NewName" {
		t.Errorf("expected nickname NewName, got %s", updated.Nickname)
	}
	if updated.Signature != "new sig" {
		t.Errorf("expected signature 'new sig', got %s", updated.Signature)
	}
	if updated.Version != 2 {
		t.Errorf("expected version 2, got %d", updated.Version)
	}

	// 冲突校验：用过期版本号 1 更新应该被拦截
	staleVersion := int32(1)
	err = svc.UpdateUserInfo(context.Background(), "U1", UpdateUserInfoRequest{
		Nickname: &newName,
		Version:  &staleVersion,
	})
	if err == nil {
		t.Fatal("expected conflict error on stale version, got nil")
	}
	if updated.Email != "old@test.com" || updated.Birthday != "20000101" || updated.Avatar != "old.png" {
		t.Errorf("unmodified fields were altered: %+v", updated)
	}
}

type mockTestUserGuard struct {
	store map[string]string
}

func (m *mockTestUserGuard) Execute(ctx context.Context, key string, leaseTTL, successTTL time.Duration, fn func(context.Context) (string, error)) (string, bool, error) {
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

func TestUserService_Register_Idempotent(t *testing.T) {
	ctx := context.Background()
	store := &fakeUserStore{users: make(map[string]UserInfo)}
	guard := &mockTestUserGuard{store: make(map[string]string)}

	svc := NewUserService(nil, store, nil, guard)

	req := RegisterRequest{
		Username:    "alice_idempotent",
		Password:    "password123",
		ClientToken: "client-token-8888",
	}

	rsp1, err1 := svc.Register(ctx, req, "127.0.0.1")
	if err1 != nil {
		t.Fatalf("first Register failed: %v", err1)
	}
	if rsp1.Uuid == "" {
		t.Fatal("expected non-empty uuid")
	}
	if len(store.users) != 1 {
		t.Fatalf("expected 1 user in store, got %d", len(store.users))
	}

	rsp2, err2 := svc.Register(ctx, req, "127.0.0.1")
	if err2 != nil {
		t.Fatalf("replay Register failed: %v", err2)
	}
	if rsp2.Uuid != rsp1.Uuid {
		t.Fatalf("expected replayed uuid %s, got %s", rsp1.Uuid, rsp2.Uuid)
	}
	if len(store.users) != 1 {
		t.Fatalf("expected still exactly 1 user in store, got %d", len(store.users))
	}

	reqNoToken := RegisterRequest{
		Username: "alice_idempotent",
		Password: "password123",
	}
	_, err3 := svc.Register(ctx, reqNoToken, "127.0.0.1")
	if err3 == nil {
		t.Fatal("expected duplicate username error, got nil")
	}
	if !errorx.IsCode(err3, errorx.CodeUserExist) {
		t.Errorf("expected CodeUserExist, got %v", err3)
	}
}
