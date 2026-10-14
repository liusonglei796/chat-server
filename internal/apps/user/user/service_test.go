package user

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/request/auth"
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

// fakeUserStore 内嵌 UserStore 接口占位，仅实现测试所需的方法
type fakeUserStore struct {
	store.UserStore
	users map[string]model.UserInfo
}

// FindByUuids 按请求顺序返回存在的用户，不存在的 uuid 被过滤掉
func (f *fakeUserStore) FindByUuids(_ context.Context, uuids []string) ([]model.UserInfo, error) {
	result := make([]model.UserInfo, 0, len(uuids))
	for _, uuid := range uuids {
		if usr, ok := f.users[uuid]; ok {
			result = append(result, usr)
		}
	}
	return result, nil
}

// FindByUuid 返回单个用户，未找到时返回被 dberr 包装的 not-found 错误（与真实仓库行为一致）
func (f *fakeUserStore) FindByUuid(_ context.Context, uuid string) (*model.UserInfo, error) {
	if usr, ok := f.users[uuid]; ok {
		return &usr, nil
	}
	return nil, dberr.WrapDBError(gorm.ErrRecordNotFound, "查询用户")
}

// UpdateUserInfo 应用字段变更到内存 map，模拟事务内更新
func (f *fakeUserStore) UpdateUserInfo(_ context.Context, user *model.UserInfo) error {
	if _, ok := f.users[user.Uuid]; !ok {
		return dberr.WrapDBError(gorm.ErrRecordNotFound, "更新用户")
	}
	f.users[user.Uuid] = *user
	return nil
}

// FindByNickname 根据昵称查询用户
func (f *fakeUserStore) FindByNickname(_ context.Context, nickname string) (*model.UserInfo, error) {
	for _, u := range f.users {
		if u.Nickname == nickname {
			cpy := u
			return &cpy, nil
		}
	}
	return nil, dberr.WrapDBError(gorm.ErrRecordNotFound, "查询用户")
}

// CreateUser 模拟创建用户，若昵称重复模拟返回 MySQL 1062
func (f *fakeUserStore) CreateUser(_ context.Context, user *model.UserInfo) error {
	for _, u := range f.users {
		if u.Nickname == user.Nickname {
			return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry for key 'nickname'"}
		}
	}
	if f.users == nil {
		f.users = make(map[string]model.UserInfo)
	}
	f.users[user.Uuid] = *user
	return nil
}

// fakeOutbox 记录写入的 outbox 事件类型，用于断言事件是否发出
type fakeOutbox struct {
	store.OutboxStore
	lastType string
}

func (f *fakeOutbox) Create(_ context.Context, o *model.Outbox) error {
	f.lastType = o.EventType
	return nil
}

// fakeUOW 实现 UserStore/RecordEvent/WithTx，满足 userUoW 接口
type fakeUOW struct {
	userStore   *fakeUserStore
	outboxStore *fakeOutbox
}

func (f *fakeUOW) UserStore() store.UserStore {
	return f.userStore
}

func (f *fakeUOW) RecordEvent(_ context.Context, _ string, eventType string, _ []byte) error {
	if f.outboxStore != nil {
		f.outboxStore.lastType = eventType
	}
	return nil
}

func (f *fakeUOW) WithTx(fn func(tx any) error) error { return fn(f) }

func TestBatchGetPublicUserInfo(t *testing.T) {
	ctx := context.Background()
	svc := NewUserService(&fakeUOW{
		userStore: &fakeUserStore{
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
		userStore: &fakeUserStore{
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
		userStore: &fakeUserStore{users: map[string]model.UserInfo{}},
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

// fakeCache 空实现，避免 UpdateUserInfo 尾部异步缓存清理调用 nil 接口
type fakeCache struct {
	store.AsyncCacheService
}

func (f *fakeCache) SubmitTask(action func()) {}

func TestUpdateUserInfo_Success(t *testing.T) {
	ctx := context.Background()
	ob := &fakeOutbox{}
	store := &fakeUserStore{users: map[string]model.UserInfo{
		"U1": {Uuid: "U1", Nickname: "old", Avatar: "old_av"},
	}}
	svc := NewUserService(&fakeUOW{
		userStore:   store,
		outboxStore: ob,
	}, &fakeCache{})

	nick := "new"
	av := "new_av"
	err := svc.UpdateUserInfo(ctx, "U1", userreq.UpdateUserInfoRequest{
		Nickname: &nick, Avatar: &av,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ob.lastType != "" {
		t.Errorf("expected no outbox event, got %q", ob.lastType)
	}
	if store.users["U1"].Nickname != "new" || store.users["U1"].Avatar != "new_av" {
		t.Errorf("user info not updated correctly: %+v", store.users["U1"])
	}
}

func TestUpdateUserInfo_NoEventWhenOnlyEmailChanged(t *testing.T) {
	ctx := context.Background()
	ob := &fakeOutbox{}
	svc := NewUserService(&fakeUOW{
		userStore: &fakeUserStore{users: map[string]model.UserInfo{
			"U1": {Uuid: "U1", Nickname: "old"},
		}},
		outboxStore: ob,
	}, &fakeCache{})

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

func TestRegister_DuplicateNickname(t *testing.T) {
	ctx := context.Background()
	store := &fakeUserStore{users: map[string]model.UserInfo{
		"U1": {Uuid: "U1", Nickname: "existing_user"},
	}}
	svc := NewUserService(&fakeUOW{
		userStore: store,
	}, &fakeCache{})

	_, err := svc.Register(ctx, auth.RegisterRequest{
		Username: "existing_user",
		Password: "password123",
	}, "127.0.0.1")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errorx.IsCode(err, errorx.CodeUserExist) {
		t.Fatalf("expected CodeUserExist, got %v", err)
	}
}

type mockTestUserGuard struct {
	store map[string]string
}

func (m *mockTestUserGuard) Execute(ctx context.Context, key string, leaseTTL, successTTL time.Duration, fn func(context.Context) (string, error)) (string, bool, error) {
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

func TestRegister_Idempotent_Replay(t *testing.T) {
	// Step 1.1: 初始化上下文与内存模拟存储
	// 来源: 单元测试上下文
	// 用于: Step 1.2 构造带 Mock 幂等守卫的 UserService
	ctx := context.Background()
	store := &fakeUserStore{users: make(map[string]model.UserInfo)}
	guard := &mockTestUserGuard{store: make(map[string]string)}
	svc := NewUserService(&fakeUOW{userStore: store}, &fakeCache{}, guard)

	req := auth.RegisterRequest{
		Username:    "alice_replay",
		Password:    "password123",
		ClientToken: "client-token-abc-123",
	}

	// Step 1.2: 首次发起注册并断言用户实体落库
	// 来源: req 注册凭据
	// 用于: Step 1.3 验证首次注册成功生成 Uuid 与用户档案
	rsp1, err1 := svc.Register(ctx, req, "127.0.0.1")
	if err1 != nil {
		t.Fatalf("first Register failed: %v", err1)
	}
	if rsp1 == nil || rsp1.Uuid == "" {
		t.Fatal("expected non-empty user uuid in first register response")
	}
	if rsp1.Nickname != "alice_replay" {
		t.Errorf("expected nickname alice_replay, got %s", rsp1.Nickname)
	}
	if len(store.users) != 1 {
		t.Fatalf("expected 1 user in store, got %d", len(store.users))
	}

	// Step 1.3: 模拟网络超时后，携带相同 ClientToken 重试注册
	// 来源: 相同 req 与 ClientToken
	// 用于: 验证状态机幂等拦截：不报“用户名已被占用”错误，并 0ms 安全回显原注册实体
	rsp2, err2 := svc.Register(ctx, req, "127.0.0.1")
	if err2 != nil {
		t.Fatalf("idempotent replay Register should succeed, but got error: %v", err2)
	}
	if rsp2.Uuid != rsp1.Uuid {
		t.Fatalf("expected replayed uuid %s, got %s", rsp1.Uuid, rsp2.Uuid)
	}
	if rsp2.Nickname != rsp1.Nickname {
		t.Fatalf("expected replayed nickname %s, got %s", rsp1.Nickname, rsp2.Nickname)
	}
	if len(store.users) != 1 {
		t.Fatalf("store should still contain exactly 1 user, got %d", len(store.users))
	}

	// Step 1.4: 不带 ClientToken 再次注册同名用户，断言正常触发业务唯一性冲突拦截
	// 来源: 相同 Username 但无 ClientToken
	// 用于: 验证非幂等重试场景下的系统保护
	reqNoToken := auth.RegisterRequest{
		Username: "alice_replay",
		Password: "password123",
	}
	_, err3 := svc.Register(ctx, reqNoToken, "127.0.0.1")
	if err3 == nil {
		t.Fatal("expected CodeUserExist for duplicate register without token, got nil")
	}
	if !errorx.IsCode(err3, errorx.CodeUserExist) {
		t.Errorf("expected CodeUserExist, got %v", err3)
	}
}


