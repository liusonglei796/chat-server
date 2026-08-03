package group

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kama_chat_server/internal/common/domain/repository"
	"kama_chat_server/internal/common/dto/event"
	groupreq "kama_chat_server/internal/common/dto/request/group"
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
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// fakeGroupMemberRepo 内嵌 GroupMemberRepository 接口占位，仅实现测试所需的方法
type fakeGroupMemberRepo struct {
	repository.GroupMemberRepository
	members []model.GroupMember
	member  *model.GroupMember
	err     error
}

func (f *fakeGroupMemberRepo) FindByGroupUuid(_ context.Context, _ string) ([]model.GroupMember, error) {
	return f.members, f.err
}

func (f *fakeGroupMemberRepo) FindByGroupAndUser(_ context.Context, _, _ string) (*model.GroupMember, error) {
	return f.member, f.err
}

func (f *fakeGroupMemberRepo) CreateGroupMember(_ context.Context, _ *model.GroupMember) error {
	return nil
}

// fakeGroupRepo 内嵌 GroupRepository 接口占位，仅实现 CreateGroup
type fakeGroupRepo struct {
	repository.GroupRepository
	created []model.GroupInfo
}

func (f *fakeGroupRepo) CreateGroup(_ context.Context, group *model.GroupInfo) error {
	f.created = append(f.created, *group)
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

// fakeUOW 内嵌 UnitOfWork 接口占位，实现 GroupMemberRepo/GroupRepo/OutboxRepo/WithTx
type fakeUOW struct {
	repository.UnitOfWork
	memberRepo *fakeGroupMemberRepo
	groupRepo  *fakeGroupRepo
	outboxRepo *fakeOutbox
}

func (f *fakeUOW) GroupMemberRepo() repository.GroupMemberRepository {
	return f.memberRepo
}

func (f *fakeUOW) GroupRepo() repository.GroupRepository { return f.groupRepo }

func (f *fakeUOW) OutboxRepo() repository.OutboxRepository { return f.outboxRepo }

func (f *fakeUOW) WithTx(fn func(tx repository.UnitOfWork) error) error { return fn(f) }

func TestCreateGroup_EmitsGroupCreatedEvent(t *testing.T) {
	ob := &fakeOutbox{}
	svc := NewGroupService(&fakeUOW{
		memberRepo: &fakeGroupMemberRepo{},
		groupRepo:  &fakeGroupRepo{},
		outboxRepo: ob,
	}, nil, ob)

	err := svc.CreateGroup(context.Background(), "U1", groupreq.CreateGroupRequest{
		Name: "test", Notice: "", AddMode: 1, Avatar: "avatar.png",
	})
	if err != nil {
		t.Fatalf("CreateGroup returned error: %v", err)
	}
	if ob.lastType != event.EventGroupCreated {
		t.Errorf("expected outbox event %q, got %q", event.EventGroupCreated, ob.lastType)
	}
}

func TestListGroupMemberIds(t *testing.T) {
	svc := NewGroupService(&fakeUOW{
		memberRepo: &fakeGroupMemberRepo{
			members: []model.GroupMember{{UserUuid: "U1"}, {UserUuid: "U2"}},
		},
	}, nil, nil)

	ids, err := svc.ListGroupMemberIds(context.Background(), "G1")
	if err != nil {
		t.Fatalf("ListGroupMemberIds returned error: %v", err)
	}
	if !reflect.DeepEqual(ids, []string{"U1", "U2"}) {
		t.Fatalf("expected [U1 U2], got %v", ids)
	}
}

func TestIsGroupMember(t *testing.T) {
	ctx := context.Background()

	t.Run("not found returns false", func(t *testing.T) {
		svc := NewGroupService(&fakeUOW{
			memberRepo: &fakeGroupMemberRepo{err: errorx.New(errorx.CodeNotFound, "不是群成员")},
		}, nil, nil)

		isMember, err := svc.IsGroupMember(ctx, "G1", "U1")
		if err != nil {
			t.Fatalf("IsGroupMember returned error: %v", err)
		}
		if isMember {
			t.Fatalf("expected false, got true")
		}
	})

	t.Run("found returns true", func(t *testing.T) {
		svc := NewGroupService(&fakeUOW{
			memberRepo: &fakeGroupMemberRepo{member: &model.GroupMember{UserUuid: "U1"}},
		}, nil, nil)

		isMember, err := svc.IsGroupMember(ctx, "G1", "U1")
		if err != nil {
			t.Fatalf("IsGroupMember returned error: %v", err)
		}
		if !isMember {
			t.Fatalf("expected true, got false")
		}
	})
}
