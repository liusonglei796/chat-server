package chat

import (
	"fmt"
	"sync"
	"testing"
)

func TestConnManager_Basic(t *testing.T) {
	cm := NewConnManager()

	c1 := &UserConn{Uuid: "U1"}
	c2 := &UserConn{Uuid: "U2"}

	cm.Store(c1.Uuid, c1)
	cm.Store(c2.Uuid, c2)

	if cm.Count() != 2 {
		t.Fatalf("expected count 2, got %d", cm.Count())
	}

	got1 := cm.Get("U1")
	if got1 == nil || got1.Uuid != "U1" {
		t.Fatalf("expected to get U1, got %v", got1)
	}

	got2, ok := cm.Load("U2")
	if !ok || got2 == nil || got2.Uuid != "U2" {
		t.Fatalf("expected to load U2, got %v", got2)
	}

	cm.Delete("U1")
	if cm.Count() != 1 {
		t.Fatalf("expected count 1 after delete, got %d", cm.Count())
	}

	if cm.Get("U1") != nil {
		t.Fatalf("expected nil for deleted U1")
	}
}

func TestConnManager_Concurrency(t *testing.T) {
	cm := NewConnManager()
	var wg sync.WaitGroup

	numUsers := 1000
	// 并发注册 1000 个连接
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			uid := fmt.Sprintf("user-%d", id)
			conn := &UserConn{Uuid: uid}
			cm.Store(uid, conn)
		}(i)
	}
	wg.Wait()

	if cm.Count() != int64(numUsers) {
		t.Fatalf("expected count %d, got %d", numUsers, cm.Count())
	}

	// 并发读
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			uid := fmt.Sprintf("user-%d", id)
			conn := cm.Get(uid)
			if conn == nil || conn.Uuid != uid {
				t.Errorf("failed to get user %s", uid)
			}
		}(i)
	}
	wg.Wait()

	// 并发删除一半
	for i := 0; i < numUsers/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			uid := fmt.Sprintf("user-%d", id)
			cm.Delete(uid)
		}(i)
	}
	wg.Wait()

	if cm.Count() != int64(numUsers/2) {
		t.Fatalf("expected count %d, got %d", numUsers/2, cm.Count())
	}
}
