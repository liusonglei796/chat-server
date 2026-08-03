package user

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/repository"
	"kama_chat_server/internal/common/dto/event"
	userreq "kama_chat_server/internal/common/dto/request/user"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/errorx"
)

// TestMain 切换到仓库根目录，使依赖 config.GetConfig() 的代码路径（如雪花ID）能找到 configs/config.toml
func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(root); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// repoRoot 从当前测试文件向上回溯到仓库根目录（含 configs/config.toml）
func repoRoot() (string, error) {
	dir, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "configs", "config.toml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repo root (configs/config.toml) not found")
		}
		dir = parent
	}
}

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

// UpdateUserInfo 应用字段变更到内存 map，模拟事务内更新
func (f *fakeUserRepo) UpdateUserInfo(_ context.Context, user *model.UserInfo) error {
	if _, ok := f.users[user.Uuid]; !ok {
		return dberr.WrapDBError(gorm.ErrRecordNotFound, "更新用户")
	}
	f.users[user.Uuid] = *user
	return nil
}

// fakeOutbox 记录写入的 outbox 事件类型，用于断言事件是否发出
type fakeOutbox struct {
	repository.OutboxRepository
	lastType string
}

func (f *fakeOutbox) Create(_ context.Context, o *model.Outbox) error {
	f.lastType = o.EventType
	return nil
}

// fakeUOW 内嵌 UnitOfWork 接口占位，实现 UserRepo/OutboxRepo/WithTx
type fakeUOW struct {
	repository.UnitOfWork
	userRepo   *fakeUserRepo
	outboxRepo *fakeOutbox
}

func (f *fakeUOW) UserRepo() repository.UserRepository {
	return f.userRepo
}

func (f *fakeUOW) OutboxRepo() repository.OutboxRepository { return f.outboxRepo }

func (f *fakeUOW) WithTx(fn func(tx repository.UnitOfWork) error) error { return fn(f) }

func TestBatchGetPublicUserInfo(t *testing.T) {
	ctx := context.Background()
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{
			users: map[string]model.UserInfo{
				"U1": {Uuid: "U1", Nickname: "a"},
				"U3": {Uuid: "U3", Nickname: "c"},
			},
		},
	}, nil, nil)

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
	svc := NewUserService(&fakeUOW{}, nil, nil)

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
	}, nil, nil)

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
	}, nil, nil)

	_, err := svc.GetUserStatus(context.Background(), "U-MISSING")
	if err == nil {
		t.Fatal("GetUserStatus(missing) should return error, got nil")
	}
	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) || codeErr.Code != errorx.CodeUserNotExist {
		t.Errorf("expected CodeUserNotExist, got: %v", err)
	}
}

// fakeCache 空实现，避免 UpdateUserInfo 尾部异步缓存清理调用 nil 接口
type fakeCache struct {
	repository.AsyncCacheService
}

func (f *fakeCache) SubmitTask(action func()) {}

func TestUpdateUserInfo_EmitsUserUpdatedEvent(t *testing.T) {
	ctx := context.Background()
	ob := &fakeOutbox{}
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{users: map[string]model.UserInfo{
			"U1": {Uuid: "U1", Nickname: "old", Avatar: "old_av"},
		}},
		outboxRepo: ob,
	}, &fakeCache{}, ob)

	nick := "new"
	av := "new_av"
	err := svc.UpdateUserInfo(ctx, "U1", userreq.UpdateUserInfoRequest{
		Nickname: &nick, Avatar: &av,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ob.lastType != event.EventUserUpdated {
		t.Errorf("expected outbox event %q, got %q", event.EventUserUpdated, ob.lastType)
	}
}

func TestUpdateUserInfo_NoEventWhenOnlyEmailChanged(t *testing.T) {
	ctx := context.Background()
	ob := &fakeOutbox{}
	svc := NewUserService(&fakeUOW{
		userRepo: &fakeUserRepo{users: map[string]model.UserInfo{
			"U1": {Uuid: "U1", Nickname: "old"},
		}},
		outboxRepo: ob,
	}, &fakeCache{}, ob)

	email := "new@example.com"
	err := svc.UpdateUserInfo(ctx, "U1", userreq.UpdateUserInfoRequest{
		Email: &email,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ob.lastType != "" {
		t.Errorf("expected no outbox event when only email changed, got %q", ob.lastType)
	}
}
