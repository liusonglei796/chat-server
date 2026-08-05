package friendship

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
)

// TestMain 切换到仓库根目录，使依赖 config.GetConfig() 的代码路径（如 Kafka 配置）能找到 configs/config.toml
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

// fakeFriendshipStore 内嵌 FriendshipStore 接口占位，记录 CreateFriendship 调用
type fakeFriendshipStore struct {
	store.FriendshipStore
	created []model.Friendship
	err     error
}

func (f *fakeFriendshipStore) CreateFriendship(_ context.Context, fs *model.Friendship) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, *fs)
	return nil
}

// fakeUOW 实现 FriendshipStore/RecordEvent/WithTx，满足 friendshipUoW 接口
type fakeUOW struct {
	friendshipStore *fakeFriendshipStore
}

func (f *fakeUOW) FriendshipStore() store.FriendshipStore { return f.friendshipStore }

func (f *fakeUOW) RecordEvent(_ context.Context, _ string, _ []byte) error { return nil }

func (f *fakeUOW) WithTx(fn func(tx any) error) error { return fn(f) }

func TestHandleFriendApplyPassed_CreatesBidirectional(t *testing.T) {
	store := &fakeFriendshipStore{}
	c := NewDomainEventConsumer(&fakeUOW{friendshipStore: store})

	payload, _ := json.Marshal(event.FriendApplyPassedEvent{UserId: "U1", FriendId: "U2"})
	if err := c.handleEvent(context.Background(), event.EventFriendApplyPassed, payload); err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}

	if len(store.created) != 2 {
		t.Fatalf("expected 2 friendships created, got %d", len(store.created))
	}

	me2friend := store.created[0]
	if me2friend.UserId != "U1" || me2friend.FriendId != "U2" || me2friend.Status != friendship_status.NORMAL {
		t.Errorf("unexpected me->friend row: %+v", me2friend)
	}

	friend2me := store.created[1]
	if friend2me.UserId != "U2" || friend2me.FriendId != "U1" || friend2me.Status != friendship_status.NORMAL {
		t.Errorf("unexpected friend->me row: %+v", friend2me)
	}
}

func TestHandleFriendApplyPassed_FirstInsertFails_ReturnsError(t *testing.T) {
	store := &fakeFriendshipStore{err: errors.New("db down")}
	c := NewDomainEventConsumer(&fakeUOW{friendshipStore: store})

	payload, _ := json.Marshal(event.FriendApplyPassedEvent{UserId: "U1", FriendId: "U2"})
	if err := c.handleEvent(context.Background(), event.EventFriendApplyPassed, payload); err == nil {
		t.Fatal("expected error on first insert failure, got nil")
	}

	if len(store.created) != 0 {
		t.Fatalf("expected no rows persisted on failure, got %d", len(store.created))
	}
}

func TestHandle_UnrelatedEvent_Noop(t *testing.T) {
	store := &fakeFriendshipStore{}
	c := NewDomainEventConsumer(&fakeUOW{friendshipStore: store})

	if err := c.handleEvent(context.Background(), event.EventGroupApplyPassed, []byte(`{}`)); err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}

	if len(store.created) != 0 {
		t.Fatalf("expected no rows for unrelated event, got %d", len(store.created))
	}
}
