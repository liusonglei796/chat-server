package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	messagereq "kama_chat_server/internal/common/dto/request/message"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
)

type mockMsgStore struct {
	mu             sync.Mutex
	createdMsg     []*model.Message
	querySessionId string
	dbMessages     []model.Message
}

func (m *mockMsgStore) FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMsgStore) FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	return nil, nil
}
func (m *mockMsgStore) FindByGroupIdPaged(ctx context.Context, groupId string, page, pageSize int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMsgStore) FindByGroupIdCursor(ctx context.Context, groupId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	return nil, nil
}
func (m *mockMsgStore) FindBySessionIdCursor(ctx context.Context, sessionId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.querySessionId = sessionId
	return &model.CursorPageMessageResult{
		Messages:   m.dbMessages,
		NextCursor: "",
		HasMore:    false,
	}, nil
}
func (m *mockMsgStore) FindByUuid(ctx context.Context, uuid string) (*model.Message, error) {
	return nil, nil
}
func (m *mockMsgStore) UpdateStatus(ctx context.Context, uuid string, status int8) error {
	return nil
}
func (m *mockMsgStore) UpdateContent(ctx context.Context, uuid, content string, msgType int8) error {
	return nil
}
func (m *mockMsgStore) Create(ctx context.Context, message *model.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdMsg = append(m.createdMsg, message)
	return nil
}

type mockSessStore struct {
	mu          sync.Mutex
	lastContent string
}

func (s *mockSessStore) FindByUuid(ctx context.Context, uuid string) (*model.Session, error) {
	return nil, nil
}
func (s *mockSessStore) FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*model.Session, error) {
	return nil, nil
}
func (s *mockSessStore) FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]model.Session, int64, error) {
	return nil, 0, nil
}
func (s *mockSessStore) FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*model.CursorPageSessionResult, error) {
	return nil, nil
}
func (s *mockSessStore) CreateSession(ctx context.Context, session *model.Session) error {
	return nil
}
func (s *mockSessStore) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	return nil
}
func (s *mockSessStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	return nil
}
func (s *mockSessStore) UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error {
	return nil
}
func (s *mockSessStore) UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error {
	return nil
}
func (s *mockSessStore) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastContent = content
	return nil
}

type zsetItem struct {
	score  float64
	member string
}

type mockCache struct {
	mu     sync.Mutex
	setnx  map[string]string
	failNX bool
	zsets  map[string][]zsetItem
}

func newMockCache() *mockCache {
	return &mockCache{
		setnx: make(map[string]string),
		zsets: make(map[string][]zsetItem),
	}
}
func (c *mockCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return nil
}
func (c *mockCache) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}
func (c *mockCache) GetOrError(ctx context.Context, key string) (string, error) {
	return "", errors.New("miss")
}
func (c *mockCache) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	return "", nil
}
func (c *mockCache) Delete(ctx context.Context, key string) error {
	return nil
}
func (c *mockCache) DeleteByPattern(ctx context.Context, patterns ...string) error {
	return nil
}
func (c *mockCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failNX {
		return false, errors.New("redis down")
	}
	if _, exists := c.setnx[key]; exists {
		return false, nil
	}
	c.setnx[key] = value
	return true, nil
}
func (c *mockCache) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}
func (c *mockCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}
func (c *mockCache) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *mockCache) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	return nil, nil
}
func (c *mockCache) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *mockCache) ZAdd(ctx context.Context, key string, score float64, member string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zsets == nil {
		c.zsets = make(map[string][]zsetItem)
	}
	items := c.zsets[key]
	items = append(items, zsetItem{score: score, member: member})
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})
	c.zsets[key] = items
	return nil
}
func (c *mockCache) ZRevRangeByScore(ctx context.Context, key string, max, min string, offset, count int64) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := c.zsets[key]
	var res []string
	var maxScore float64 = 1e20
	exclusive := false
	if max != "+inf" && max != "" {
		if strings.HasPrefix(max, "(") {
			exclusive = true
			fmt.Sscanf(max[1:], "%f", &maxScore)
		} else {
			fmt.Sscanf(max, "%f", &maxScore)
		}
	}
	var skipped int64
	for _, it := range items {
		if exclusive && it.score >= maxScore {
			continue
		}
		if !exclusive && it.score > maxScore {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		res = append(res, it.member)
		if int64(len(res)) == count {
			break
		}
	}
	return res, nil
}
func (c *mockCache) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := c.zsets[key]
	if len(items) > 200 {
		c.zsets[key] = items[:200]
	}
	return nil
}
func (c *mockCache) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *mockCache) ZCard(ctx context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.zsets[key])), nil
}
func (c *mockCache) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return 0, nil
}
func (c *mockCache) SubmitTask(action func()) {
	action()
}

func TestSendMessage_EmptyReceiver(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	req := messagereq.ChatMessageRequest{
		SendId:    "U1001",
		ReceiveId: "",
		Content:   "hello",
		Type:      int8(msgtype.Text),
	}
	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty receiveId, got nil")
	}
}

func TestSendMessage_Idempotent(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	// Pre-populate dedup key in cache
	cache.setnx["msg:dedup:client-msg-123"] = "1"

	req := messagereq.ChatMessageRequest{
		ClientMsgId: "client-msg-123",
		SendId:      "U1001",
		ReceiveId:   "U1002",
		Content:     "repeat message",
		Type:        int8(msgtype.Text),
	}

	rsp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on idempotent call: %v", err)
	}
	if rsp.MessageUuid != "client-msg-123" {
		t.Fatalf("expected message uuid %s, got %s", "client-msg-123", rsp.MessageUuid)
	}
	if len(msgStore.createdMsg) != 0 {
		t.Fatalf("expected 0 messages created on duplicate, got %d", len(msgStore.createdMsg))
	}
}

func TestResolveSessionId(t *testing.T) {
	// Private chat: both directions must yield identical session_id
	sid1 := ResolveSessionId("U1001", "U1002")
	sid2 := ResolveSessionId("U1002", "U1001")
	if sid1 != "P_U1001_U1002" {
		t.Fatalf("expected P_U1001_U1002, got %s", sid1)
	}
	if sid1 != sid2 {
		t.Fatalf("expected bidirectional consistency: %s != %s", sid1, sid2)
	}

	// Group chat: must yield groupId
	groupSid := ResolveSessionId("U1001", "G2001")
	if groupSid != "G2001" {
		t.Fatalf("expected G2001, got %s", groupSid)
	}
}

func TestSnowflakeUuidToScore(t *testing.T) {
	score1 := snowflakeUuidToScore("M1000000000000000000")
	score2 := snowflakeUuidToScore("M1000000000000000001")
	if score1 <= 0 || score2 <= 0 {
		t.Fatalf("scores should be positive: %f, %f", score1, score2)
	}
	if score1 >= score2 {
		t.Fatalf("snowflake scores must be monotonically increasing: %f >= %f", score1, score2)
	}

	// Timestamp fallback test
	tsScore := snowflakeUuidToScore("1700000000")
	if tsScore != 1700000000*1000000 {
		t.Fatalf("unexpected timestamp score: %f", tsScore)
	}
}

func TestBuildMessageFromRequest_SessionId(t *testing.T) {
	svc := NewMessageService(&mockMsgStore{}, &mockSessStore{}, newMockCache())

	// When SessionId is empty in request, auto-resolve
	req := messagereq.ChatMessageRequest{
		SendId:    "U1002",
		ReceiveId: "U1001",
		Content:   "test",
		Type:      int8(msgtype.Text),
	}
	msg := svc.buildMessageFromRequest(req)
	if msg.SessionId != "P_U1001_U1002" {
		t.Fatalf("expected SessionId P_U1001_U1002, got %s", msg.SessionId)
	}

	// Group chat auto-resolve
	reqGroup := messagereq.ChatMessageRequest{
		SendId:    "U1002",
		ReceiveId: "G3001",
		Content:   "test group",
		Type:      int8(msgtype.Text),
	}
	msgGroup := svc.buildMessageFromRequest(reqGroup)
	if msgGroup.SessionId != "G3001" {
		t.Fatalf("expected SessionId G3001, got %s", msgGroup.SessionId)
	}
}

func TestGetMessagesBySessionIdCursorWithCache_HitZSet(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	sessionId := "P_U1001_U1002"
	key := constants.CacheKeyMessageZSet + sessionId

	// Pre-populate 2 messages in ZSET
	msg1 := model.Message{
		Uuid:      "M1000000000000000001",
		SessionId: sessionId,
		SendId:    "U1001",
		ReceiveId: "U1002",
		Content:   "msg 1",
		Type:      int8(msgtype.Text),
	}
	msg1.CreatedAt = time.Now().Add(-1 * time.Minute)
	msg2 := model.Message{
		Uuid:      "M1000000000000000002",
		SessionId: sessionId,
		SendId:    "U1002",
		ReceiveId: "U1001",
		Content:   "msg 2",
		Type:      int8(msgtype.Text),
	}
	msg2.CreatedAt = time.Now()
	data1, _ := json.Marshal(msg1)
	data2, _ := json.Marshal(msg2)
	_ = cache.ZAdd(context.Background(), key, snowflakeUuidToScore(msg1.Uuid), string(data1))
	_ = cache.ZAdd(context.Background(), key, snowflakeUuidToScore(msg2.Uuid), string(data2))

	// Query from cache
	list, _, hasMore, err := svc.getMessagesBySessionIdCursorWithCache(context.Background(), sessionId, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 messages from cache, got %d", len(list))
	}
	if hasMore {
		t.Fatalf("expected hasMore to be false for 2 items with total < 200")
	}
	// Verify DB was NOT queried
	if msgStore.querySessionId != "" {
		t.Fatalf("expected DB not to be queried, but queried sessionId %s", msgStore.querySessionId)
	}
}

func TestGetMessagesBySessionIdCursorWithCache_FallbackDB(t *testing.T) {
	dbMsg := model.Message{
		Uuid:      "M1000000000000000001",
		SessionId: "P_U1001_U1002",
		SendId:    "U1001",
		ReceiveId: "U1002",
		Content:   "from db",
		Type:      int8(msgtype.Text),
	}
	dbMsg.CreatedAt = time.Now()
	msgStore := &mockMsgStore{
		dbMessages: []model.Message{dbMsg},
	}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	sessionId := "P_U1001_U1002"

	// Cache is empty, query should fallback to DB
	list, _, _, err := svc.getMessagesBySessionIdCursorWithCache(context.Background(), sessionId, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].Content != "from db" {
		t.Fatalf("expected 1 message from DB, got %+v", list)
	}
	if msgStore.querySessionId != sessionId {
		t.Fatalf("expected querySessionId %s, got %s", sessionId, msgStore.querySessionId)
	}

	// Verify backfill into ZSET occurred
	key := constants.CacheKeyMessageZSet + sessionId
	count, _ := cache.ZCard(context.Background(), key)
	if count != 1 {
		t.Fatalf("expected 1 item backfilled into ZSET, got %d", count)
	}
}
