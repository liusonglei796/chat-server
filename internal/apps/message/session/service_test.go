package session

import (
	"context"
	"testing"

	"github.com/go-sql-driver/mysql"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/model"
)

type mockSessionStore struct {
	store.SessionStore
	sessions            map[string]*model.Session
	failOnCreateWithDup bool
}

func (m *mockSessionStore) FindBySendIdAndReceiveId(_ context.Context, sendId, receiveId string) (*model.Session, error) {
	key := sendId + ":" + receiveId
	if s, ok := m.sessions[key]; ok {
		cpy := *s
		return &cpy, nil
	}
	return nil, nil
}

func (m *mockSessionStore) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*model.Session, error) {
	return m.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
}

func (m *mockSessionStore) CreateSession(_ context.Context, session *model.Session) error {
	key := session.SendId + ":" + session.ReceiveId
	if m.failOnCreateWithDup {
		// 模拟并发由另一个协程抢先插入
		m.sessions[key] = &model.Session{
			Uuid:      "EXISTING_SESSION_UUID",
			SendId:    session.SendId,
			ReceiveId: session.ReceiveId,
		}
		return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
	}
	m.sessions[key] = session
	return nil
}

func TestCreateGroupSession_ConcurrentDuplicateKey(t *testing.T) {
	ctx := context.Background()
	mockStore := &mockSessionStore{
		sessions:            make(map[string]*model.Session),
		failOnCreateWithDup: true,
	}
	svc := &SessionService{
		sessionStore: mockStore,
	}

	uuid, err := svc.CreateGroupSession(ctx, "G1001", "U1001", "TestGroup", "")
	if err != nil {
		t.Fatalf("expected nil error on duplicate key fallback, got: %v", err)
	}
	if uuid != "EXISTING_SESSION_UUID" {
		t.Fatalf("expected EXISTING_SESSION_UUID, got %s", uuid)
	}
}
