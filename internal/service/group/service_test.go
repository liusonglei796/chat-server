package group

import (
	"context"
	"reflect"
	"testing"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"
)

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

// fakeUOW 内嵌 UnitOfWork 接口占位，仅实现 GroupMemberRepo()
type fakeUOW struct {
	repository.UnitOfWork
	memberRepo *fakeGroupMemberRepo
}

func (f *fakeUOW) GroupMemberRepo() repository.GroupMemberRepository {
	return f.memberRepo
}

func (f *fakeUOW) OutboxRepo() repository.OutboxRepository { return nil }

func TestListGroupMemberIds(t *testing.T) {
	svc := NewGroupService(&fakeUOW{
		memberRepo: &fakeGroupMemberRepo{
			members: []model.GroupMember{{UserUuid: "U1"}, {UserUuid: "U2"}},
		},
	}, nil)

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
		}, nil)

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
		}, nil)

		isMember, err := svc.IsGroupMember(ctx, "G1", "U1")
		if err != nil {
			t.Fatalf("IsGroupMember returned error: %v", err)
		}
		if !isMember {
			t.Fatalf("expected true, got false")
		}
	})
}
