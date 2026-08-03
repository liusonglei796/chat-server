package friendship

import (
	"context"
	"testing"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/errorx"
)

// fakeFriendshipRepo 内嵌 FriendshipRepository 接口占位，仅实现测试所需的方法
type fakeFriendshipRepo struct {
	repository.FriendshipRepository
	fs  *model.Friendship
	err error
}

func (f *fakeFriendshipRepo) FindByUserIdAndFriendId(_ context.Context, _, _ string) (*model.Friendship, error) {
	return f.fs, f.err
}

// fakeUOW 内嵌 UnitOfWork 接口占位，仅实现 FriendshipRepo()
type fakeUOW struct {
	repository.UnitOfWork
	friendshipRepo *fakeFriendshipRepo
}

func (f *fakeUOW) FriendshipRepo() repository.FriendshipRepository {
	return f.friendshipRepo
}

func (f *fakeUOW) OutboxRepo() repository.OutboxRepository { return nil }

func TestGetFriendshipStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("not found returns 0", func(t *testing.T) {
		svc := NewFriendshipService(&fakeUOW{
			friendshipRepo: &fakeFriendshipRepo{err: errorx.New(errorx.CodeNotFound, "好友关系不存在")},
		}, nil)

		status, err := svc.GetFriendshipStatus(ctx, "U1", "U2")
		if err != nil {
			t.Fatalf("GetFriendshipStatus returned error: %v", err)
		}
		if status != 0 {
			t.Fatalf("expected status 0 (not friend), got %d", status)
		}
	})

	t.Run("NORMAL returns 1", func(t *testing.T) {
		svc := NewFriendshipService(&fakeUOW{
			friendshipRepo: &fakeFriendshipRepo{fs: &model.Friendship{Status: friendship_status.NORMAL}},
		}, nil)

		status, err := svc.GetFriendshipStatus(ctx, "U1", "U2")
		if err != nil {
			t.Fatalf("GetFriendshipStatus returned error: %v", err)
		}
		if status != 1 {
			t.Fatalf("expected status 1 (normal), got %d", status)
		}
	})

	t.Run("BE_BLACK returns 3", func(t *testing.T) {
		svc := NewFriendshipService(&fakeUOW{
			friendshipRepo: &fakeFriendshipRepo{fs: &model.Friendship{Status: friendship_status.BE_BLACK}},
		}, nil)

		status, err := svc.GetFriendshipStatus(ctx, "U1", "U2")
		if err != nil {
			t.Fatalf("GetFriendshipStatus returned error: %v", err)
		}
		if status != 3 {
			t.Fatalf("expected status 3 (blocked by other), got %d", status)
		}
	})

	t.Run("BLACK returns 2", func(t *testing.T) {
		svc := NewFriendshipService(&fakeUOW{
			friendshipRepo: &fakeFriendshipRepo{fs: &model.Friendship{Status: friendship_status.BLACK}},
		}, nil)

		status, err := svc.GetFriendshipStatus(ctx, "U1", "U2")
		if err != nil {
			t.Fatalf("GetFriendshipStatus returned error: %v", err)
		}
		if status != 2 {
			t.Fatalf("expected status 2 (blocked other), got %d", status)
		}
	})
}
