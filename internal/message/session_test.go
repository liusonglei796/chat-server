package message

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"kama_chat_server/pkg/event"
)

type mockSessionStore struct {
	SessionStore
	sessions            map[string]*Session
	failOnCreateWithDup bool
}

func (m *mockSessionStore) FindBySendIdAndReceiveId(_ context.Context, sendId, receiveId string) (*Session, error) {
	key := sendId + ":" + receiveId
	if s, ok := m.sessions[key]; ok {
		cpy := *s
		return &cpy, nil
	}
	return nil, nil
}

func (m *mockSessionStore) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*Session, error) {
	return m.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
}

func (m *mockSessionStore) CreateSession(_ context.Context, session *Session) error {
	key := session.SendId + ":" + session.ReceiveId
	if m.failOnCreateWithDup {
		m.sessions[key] = &Session{
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
		sessions:            make(map[string]*Session),
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

type mockEventHandlerSessionStore struct {
	mu           sync.Mutex
	sessions     map[string]*Session
	deletedUuids []string
}

func newMockEventHandlerSessionStore() *mockEventHandlerSessionStore {
	return &mockEventHandlerSessionStore{
		sessions: make(map[string]*Session),
	}
}

func (m *mockEventHandlerSessionStore) FindByUuid(ctx context.Context, uuid string) (*Session, error) {
	return nil, nil
}

func (m *mockEventHandlerSessionStore) FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sendId+":"+receiveId]
	if !ok {
		return nil, nil
	}
	return sess, nil
}

func (m *mockEventHandlerSessionStore) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*Session, error) {
	return m.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
}

func (m *mockEventHandlerSessionStore) FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]Session, int64, error) {
	return nil, 0, nil
}

func (m *mockEventHandlerSessionStore) FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*CursorPageSessionResult, error) {
	return nil, nil
}

func (m *mockEventHandlerSessionStore) CreateSession(ctx context.Context, session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.SendId+":"+session.ReceiveId] = session
	return nil
}

func (m *mockEventHandlerSessionStore) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedUuids = append(m.deletedUuids, uuids...)
	return nil
}

func (m *mockEventHandlerSessionStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	return nil
}

func (m *mockEventHandlerSessionStore) UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error {
	return nil
}

func (m *mockEventHandlerSessionStore) UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error {
	return nil
}

func (m *mockEventHandlerSessionStore) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time, seqId int64) (bool, error) {
	return true, nil
}

var _ SessionStore = (*mockEventHandlerSessionStore)(nil)

func TestSessionEventHandler_GroupMemberRemoved(t *testing.T) {
	mockStore := newMockEventHandlerSessionStore()
	mockStore.sessions["user_1:G888"] = &Session{Uuid: "S_1", SendId: "user_1", ReceiveId: "G888"}
	mockStore.sessions["user_2:G888"] = &Session{Uuid: "S_2", SendId: "user_2", ReceiveId: "G888"}
	mockStore.sessions["user_3:G888"] = &Session{Uuid: "S_3", SendId: "user_3", ReceiveId: "G888"}

	handler := NewSessionEventHandler(mockStore)

	payload, err := json.Marshal(event.GroupMemberRemovedEvent{
		GroupId:     "G888",
		OperatorId:  "admin",
		MemberUuids: []string{"user_1", "user_2"},
	})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	err = handler.Handle(context.Background(), event.EventGroupMemberRemoved, payload)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(mockStore.deletedUuids) != 2 {
		t.Fatalf("expected 2 deleted session uuids, got %d", len(mockStore.deletedUuids))
	}
	expectedMap := map[string]bool{"S_1": true, "S_2": true}
	for _, id := range mockStore.deletedUuids {
		if !expectedMap[id] {
			t.Errorf("unexpected deleted uuid: %s", id)
		}
	}
}

func TestSessionEventHandler_GroupMemberLeft(t *testing.T) {
	mockStore := newMockEventHandlerSessionStore()
	mockStore.sessions["user_1:G888"] = &Session{Uuid: "S_1", SendId: "user_1", ReceiveId: "G888"}

	handler := NewSessionEventHandler(mockStore)

	payload, err := json.Marshal(event.GroupMemberLeftEvent{
		GroupId: "G888",
		UserId:  "user_1",
	})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	err = handler.Handle(context.Background(), event.EventGroupMemberLeft, payload)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(mockStore.deletedUuids) != 1 || mockStore.deletedUuids[0] != "S_1" {
		t.Fatalf("expected deleted uuid [S_1], got %v", mockStore.deletedUuids)
	}
}

func TestSessionEventHandler_GroupMemberRemoved_NotFound_Idempotent(t *testing.T) {
	mockStore := newMockEventHandlerSessionStore()
	handler := NewSessionEventHandler(mockStore)

	payload, _ := json.Marshal(event.GroupMemberRemovedEvent{
		GroupId:     "G888",
		OperatorId:  "admin",
		MemberUuids: []string{"non_existent_user"},
	})

	err := handler.Handle(context.Background(), event.EventGroupMemberRemoved, payload)
	if err != nil {
		t.Fatalf("expected nil error for idempotent handling, got: %v", err)
	}
	if len(mockStore.deletedUuids) != 0 {
		t.Fatalf("expected 0 deleted uuids, got %d", len(mockStore.deletedUuids))
	}
}
