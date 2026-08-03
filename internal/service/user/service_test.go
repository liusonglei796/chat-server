package user

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"kama_chat_server/internal/dao/mysql/dberr"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"
)

// fakeUserRepo 内嵌 UserRepository 接口占位，仅实现测试所需的方法
type fakeUserRepo struct {
	repository.UserRepository
	users map[string]model.UserInfo
}

// FindByUuids 按请求顺序返回存在的用户，不存在的 uuid 被过滤掉
func (f *fakeUserRepo) FindByUuids(_ context.Context, uuids []string) ([]model.UserInfo, error) {
	result := make([]model.UserInfo, 0, len(uuids))
	for _, uuid := range uuids {
		if usr, ok := f.users[uuid]; ok {
			result = append(result, usr)
		}
	}
	return result, nil
}

// FindByUuid 返回单个用户，未找到时返回被 dberr 包装的 not-found 错误（与真实仓库行为一致）
func (f *fakeUserRepo) FindByUuid(_ context.Context, uuid string) (*model.UserInfo, error) {
	if usr, ok := f.users[uuid]; ok {
		return &usr, nil
	}
	return nil, dberr.WrapDBError(gorm.ErrRecordNotFound, "查询用户")
}

// fakeUOW 内嵌 UnitOfWork 接口占位，仅实现 UserRepo()
type fakeUOW struct {
	repository.UnitOfWork
	userRepo *fakeUserRepo
}

func (f *fakeUOW) UserRepo() repository.UserRepository {
	return f.userRepo
}

func TestBatchGetPublicUserInfo(t *testing.T) {
	ctx := context.Background()
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{
			users: map[string]model.UserInfo{
				"U1": {Uuid: "U1", Nickname: "a"},
				"U3": {Uuid: "U3", Nickname: "c"},
			},
		},
	}, nil)

	rsp, err := svc.BatchGetPublicUserInfo(ctx, []string{"U1", "U2", "U3"})
	if err != nil {
		t.Fatalf("BatchGetPublicUserInfo returned error: %v", err)
	}
	if len(rsp) != 2 {
		t.Fatalf("expected 2 users (U1,U3), got %d: %+v", len(rsp), rsp)
	}
	got := make(map[string]string, len(rsp))
	for _, info := range rsp {
		got[info.Uuid] = info.Nickname
	}
	if got["U1"] != "a" {
		t.Errorf("U1 nickname = %q, want %q", got["U1"], "a")
	}
	if got["U3"] != "c" {
		t.Errorf("U3 nickname = %q, want %q", got["U3"], "c")
	}
	if _, ok := got["U2"]; ok {
		t.Errorf("U2 should be filtered out (user not found), got %q", got["U2"])
	}
}

func TestBatchGetPublicUserInfoEmpty(t *testing.T) {
	svc := NewUserService(&fakeUOW{}, nil)

	rsp, err := svc.BatchGetPublicUserInfo(context.Background(), nil)
	if err != nil {
		t.Fatalf("empty input should not return error, got: %v", err)
	}
	if len(rsp) != 0 {
		t.Fatalf("expected empty result, got %d: %+v", len(rsp), rsp)
	}
}

func TestGetUserStatus(t *testing.T) {
	ctx := context.Background()
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{
			users: map[string]model.UserInfo{
				"U-NORMAL":  {Uuid: "U-NORMAL", Status: 0},
				"U-DISABLE": {Uuid: "U-DISABLE", Status: 1},
			},
		},
	}, nil)

	status, err := svc.GetUserStatus(ctx, "U-NORMAL")
	if err != nil {
		t.Fatalf("GetUserStatus(NORMAL) returned error: %v", err)
	}
	if status != 0 {
		t.Errorf("NORMAL status = %d, want 0", status)
	}

	status, err = svc.GetUserStatus(ctx, "U-DISABLE")
	if err != nil {
		t.Fatalf("GetUserStatus(DISABLE) returned error: %v", err)
	}
	if status != 1 {
		t.Errorf("DISABLE status = %d, want 1", status)
	}
}

func TestGetUserStatusNotFound(t *testing.T) {
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{users: map[string]model.UserInfo{}},
	}, nil)

	_, err := svc.GetUserStatus(context.Background(), "U-MISSING")
	if err == nil {
		t.Fatal("GetUserStatus(missing) should return error, got nil")
	}
	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) || codeErr.Code != errorx.CodeUserNotExist {
		t.Errorf("expected CodeUserNotExist, got: %v", err)
	}
}
