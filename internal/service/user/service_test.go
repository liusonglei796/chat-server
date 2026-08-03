package user

import (
	"context"
	"testing"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/model"
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
