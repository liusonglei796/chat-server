package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	friendshippb "kama_chat_server/api/gen/friendship"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/constants"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/errorx"
)

// ==================== Mocks ====================

type mockRecallMessageStore struct {
	MessageStore
	msg          *Message
	updateOk     bool
	updateCalled int
}

func (m *mockRecallMessageStore) FindByUuid(_ context.Context, uuid string) (*Message, error) {
	if m.msg != nil && m.msg.Uuid == uuid {
		cpy := *m.msg
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "消息不存在")
}

func (m *mockRecallMessageStore) UpdateContent(_ context.Context, uuid, content string, msgType int8) (bool, error) {
	m.updateCalled++
	if m.updateOk {
		m.msg.Content = content
		m.msg.Type = msgType
		return true, nil
	}
	return false, nil
}

type mockMsgStore struct {
	mu             sync.Mutex
	createdMsg     []*Message
	querySessionId string
	dbMessages     []Message
	failCreate     bool
	maxSeq         int64
	syncMessages   []Message
}

func (m *mockMsgStore) FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMsgStore) FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	return nil, nil
}
func (m *mockMsgStore) FindByGroupIdPaged(ctx context.Context, groupId string, page, pageSize int) ([]Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMsgStore) FindByGroupIdCursor(ctx context.Context, groupId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	return nil, nil
}
func (m *mockMsgStore) FindBySessionIdCursor(ctx context.Context, sessionId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.querySessionId = sessionId
	return &CursorPageMessageResult{
		Messages:   m.dbMessages,
		NextCursor: "",
		HasMore:    false,
	}, nil
}
func (m *mockMsgStore) FindBySessionIdSync(ctx context.Context, sessionId string, afterSeq int64, limit int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncMessages, nil
}
func (m *mockMsgStore) GetMaxSeqBySessionId(ctx context.Context, sessionId string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxSeq, nil
}
func (m *mockMsgStore) FindByUuid(ctx context.Context, uuid string) (*Message, error) {
	return nil, nil
}
func (m *mockMsgStore) UpdateContent(ctx context.Context, uuid, content string, msgType int8) (bool, error) {
	return true, nil
}
func (m *mockMsgStore) Create(ctx context.Context, message *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCreate {
		return errors.New("db create error")
	}
	m.createdMsg = append(m.createdMsg, message)
	return nil
}

type mockSessStore struct {
	mu          sync.Mutex
	lastContent string
}

func (s *mockSessStore) FindByUuid(ctx context.Context, uuid string) (*Session, error) {
	return nil, nil
}
func (s *mockSessStore) FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*Session, error) {
	return nil, nil
}
func (s *mockSessStore) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*Session, error) {
	return nil, nil
}
func (s *mockSessStore) FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]Session, int64, error) {
	return nil, 0, nil
}
func (s *mockSessStore) FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*CursorPageSessionResult, error) {
	return nil, nil
}
func (s *mockSessStore) CreateSession(ctx context.Context, session *Session) error {
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
func (s *mockSessStore) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time, seqId int64) error {
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
	status map[string]string
	res    map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{
		setnx:  make(map[string]string),
		zsets:  make(map[string][]zsetItem),
		status: make(map[string]string),
		res:    make(map[string]string),
	}
}

func (c *mockCache) Execute(ctx context.Context, key string, leaseTTL, successTTL time.Duration, fn func(ctx context.Context) (string, error)) (string, bool, error) {
	c.mu.Lock()
	if c.status == nil {
		c.status = make(map[string]string)
		c.res = make(map[string]string)
	}
	st := c.status[key]
	if st == "SUCCESS" {
		res := c.res[key]
		c.mu.Unlock()
		return res, true, nil
	}
	if st == "PROCESSING" {
		c.mu.Unlock()
		return "", false, errorx.New(errorx.CodeTooManyRequests, "请求正在处理中，请勿重复提交")
	}
	c.status[key] = "PROCESSING"
	c.mu.Unlock()

	res, err := fn(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		delete(c.status, key)
		delete(c.res, key)
		return "", false, err
	}
	c.status[key] = "SUCCESS"
	c.res[key] = res
	return res, false, nil
}
func (c *mockCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setnx[key] = value
	return nil
}
func (c *mockCache) Get(ctx context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if val, ok := c.setnx[key]; ok {
		return val, nil
	}
	return "", nil
}
func (c *mockCache) GetOrError(ctx context.Context, key string) (string, error) {
	return "", errors.New("miss")
}
func (c *mockCache) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	return "", nil
}
func (c *mockCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.setnx, key)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	var current int64 = 0
	if val, ok := c.setnx[key]; ok {
		current, _ = strconv.ParseInt(val, 10, 64)
	}
	current++
	c.setnx[key] = strconv.FormatInt(current, 10)
	return current, nil
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
func (c *mockCache) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(keys) > 0 && len(args) > 0 {
		key := keys[0]
		if target, ok := args[0].(int64); ok {
			c.setnx[key] = strconv.FormatInt(target, 10)
			return target, nil
		}
	}
	return int64(1), nil
}
func (c *mockCache) SubmitTask(action func()) {
	action()
}

// ==================== Tests ====================

func TestMessageService_RecallMessage_Atomic(t *testing.T) {
	ctx := context.Background()

	t.Run("first recall succeeds and triggers notify", func(t *testing.T) {
		msgStore := &mockRecallMessageStore{
			msg: &Message{
				Uuid:      "M1001",
				SendId:    "U1",
				ReceiveId: "U2",
				Type:      int8(msgtype.Text),
				Content:   "hello",
			},
			updateOk: true,
		}
		msgStore.msg.CreatedAt = time.Now()

		svc := NewMessageService(msgStore, nil, nil)
		notifyCalled := false
		svc.SetPushRecallNotify(func(messageUuid, receiveId string) {
			notifyCalled = true
		})

		err := svc.RecallMessage(ctx, "U1", RecallMessageRequest{
			MessageUuid: "M1001",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !notifyCalled {
			t.Fatal("expected notify to be called")
		}
		if msgStore.updateCalled != 1 {
			t.Fatalf("expected updateCalled=1, got %d", msgStore.updateCalled)
		}
	})

	t.Run("concurrent duplicate recall fails and does not notify", func(t *testing.T) {
		msgStore := &mockRecallMessageStore{
			msg: &Message{
				Uuid:      "M1001",
				SendId:    "U1",
				ReceiveId: "U2",
				Type:      int8(msgtype.Text),
				Content:   "hello",
			},
			updateOk: false,
		}
		msgStore.msg.CreatedAt = time.Now()

		svc := NewMessageService(msgStore, nil, nil)
		notifyCalled := false
		svc.SetPushRecallNotify(func(messageUuid, receiveId string) {
			notifyCalled = true
		})

		err := svc.RecallMessage(ctx, "U1", RecallMessageRequest{
			MessageUuid: "M1001",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errorx.IsCode(err, errorx.CodeInvalidParam) {
			t.Fatalf("expected CodeInvalidParam, got %v", err)
		}
		if notifyCalled {
			t.Fatal("expected notify NOT to be called on duplicate recall")
		}
	})

	t.Run("retry recall on already recalled message heals idempotently with nil error and no notify", func(t *testing.T) {
		msgStore := &mockRecallMessageStore{
			msg: &Message{
				Uuid:      "M1001",
				SendId:    "U1",
				ReceiveId: "U2",
				Type:      int8(msgtype.Recall),
				Content:   "",
			},
			updateOk: false,
		}
		msgStore.msg.CreatedAt = time.Now()

		svc := NewMessageService(msgStore, nil, nil)
		notifyCalled := false
		svc.SetPushRecallNotify(func(messageUuid, receiveId string) {
			notifyCalled = true
		})

		err := svc.RecallMessage(ctx, "U1", RecallMessageRequest{
			MessageUuid: "M1001",
		})
		if err != nil {
			t.Fatalf("expected nil error for idempotent healing, got %v", err)
		}
		if notifyCalled {
			t.Fatal("expected notify NOT to be called on idempotent healing")
		}
		if msgStore.updateCalled != 0 {
			t.Fatalf("expected updateCalled=0 when already recalled, got %d", msgStore.updateCalled)
		}
	})
}

func TestSendMessage_EmptyReceiver(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	req := ChatMessageRequest{
		ClientMsgId: "msg-fail-empty-rcv",
		SendId:      "U1001",
		ReceiveId:   "",
		Content:     "hello",
		Type:        int8(msgtype.Text),
	}
	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty receiveId, got nil")
	}
	if _, exists := cache.status["msg:dedup:msg-fail-empty-rcv"]; exists {
		t.Fatal("expected dedup key to be deleted on validation failure, but it still exists")
	}
}

func TestSendMessage_DbCreateFailDeletesDedupKey(t *testing.T) {
	msgStore := &mockMsgStore{failCreate: true}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	req := ChatMessageRequest{
		ClientMsgId: "msg-fail-db",
		SendId:      "U1001",
		ReceiveId:   "U1002",
		Content:     "hello db fail",
		Type:        int8(msgtype.Text),
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on db create failure, got nil")
	}

	if _, exists := cache.status["msg:dedup:msg-fail-db"]; exists {
		t.Fatal("expected dedup key to be deleted on db create failure, but it still exists")
	}

	msgStore.failCreate = false
	rsp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if rsp.MessageUuid == "" {
		t.Fatal("expected valid message uuid on retry, got empty")
	}
	if len(msgStore.createdMsg) != 1 {
		t.Fatalf("expected 1 message created on retry, got %d", len(msgStore.createdMsg))
	}
}

func TestSendMessage_Idempotent(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	cachedRsp := SendMessageRespond{
		MessageUuid: "client-msg-123",
		CreatedAt:   "2026-10-10 12:00:00",
	}
	data, _ := json.Marshal(cachedRsp)
	cache.status["msg:dedup:client-msg-123"] = "SUCCESS"
	cache.res["msg:dedup:client-msg-123"] = string(data)

	req := ChatMessageRequest{
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

func TestSendMessage_InProgress_ReturnsTooManyRequests(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	cache.status["msg:dedup:client-msg-in-prog"] = "PROCESSING"

	req := ChatMessageRequest{
		ClientMsgId: "client-msg-in-prog",
		SendId:      "U1001",
		ReceiveId:   "U1002",
		Content:     "concurrent message",
		Type:        int8(msgtype.Text),
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on IN_PROGRESS request, got nil")
	}
	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) || codeErr.Code != errorx.CodeTooManyRequests {
		t.Fatalf("expected CodeTooManyRequests, got %v", err)
	}
}

func TestSendMessage_SuccessReplayAndCachedResponse(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	clientMsgId := "client-msg-replay-1"
	req := ChatMessageRequest{
		ClientMsgId: clientMsgId,
		SendId:      "U1001",
		ReceiveId:   "U1002",
		Content:     "hello replay",
		Type:        int8(msgtype.Text),
	}

	rsp1, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on first send: %v", err)
	}
	if rsp1.MessageUuid == "" {
		t.Fatal("expected non-empty message uuid")
	}
	if len(msgStore.createdMsg) != 1 {
		t.Fatalf("expected 1 created message, got %d", len(msgStore.createdMsg))
	}

	key := "msg:dedup:" + clientMsgId
	val, ok := cache.status[key]
	if !ok || val != "SUCCESS" {
		t.Fatalf("expected valid cached SUCCESS status, got: %s", val)
	}

	rsp2, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on replay send: %v", err)
	}
	if rsp2.MessageUuid != rsp1.MessageUuid {
		t.Fatalf("expected replayed message uuid %s, got %s", rsp1.MessageUuid, rsp2.MessageUuid)
	}
	if rsp2.CreatedAt != rsp1.CreatedAt {
		t.Fatalf("expected replayed created_at %s, got %s", rsp1.CreatedAt, rsp2.CreatedAt)
	}
	if len(msgStore.createdMsg) != 1 {
		t.Fatalf("expected still 1 created message on replay, got %d", len(msgStore.createdMsg))
	}
}

func TestResolveSessionId(t *testing.T) {
	sid1 := ResolveSessionId("U1001", "U1002")
	sid2 := ResolveSessionId("U1002", "U1001")
	if sid1 != "P_U1001_U1002" {
		t.Fatalf("expected P_U1001_U1002, got %s", sid1)
	}
	if sid1 != sid2 {
		t.Fatalf("expected bidirectional consistency: %s != %s", sid1, sid2)
	}

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

	tsScore := snowflakeUuidToScore("1700000000")
	if tsScore != 1700000000*1000000 {
		t.Fatalf("unexpected timestamp score: %f", tsScore)
	}
}

func TestBuildMessageFromRequest_SessionId(t *testing.T) {
	svc := NewMessageService(&mockMsgStore{}, &mockSessStore{}, newMockCache())

	req := ChatMessageRequest{
		SendId:    "U1002",
		ReceiveId: "U1001",
		Content:   "test",
		Type:      int8(msgtype.Text),
	}
	msg := svc.buildMessageFromRequest(req)
	if msg.SessionId != "P_U1001_U1002" {
		t.Fatalf("expected SessionId P_U1001_U1002, got %s", msg.SessionId)
	}

	reqGroup := ChatMessageRequest{
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

	msg1 := Message{
		Uuid:      "M1000000000000000001",
		SessionId: sessionId,
		SendId:    "U1001",
		ReceiveId: "U1002",
		Content:   "msg 1",
		Type:      int8(msgtype.Text),
	}
	msg1.CreatedAt = time.Now().Add(-1 * time.Minute)
	msg2 := Message{
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
	if msgStore.querySessionId != "" {
		t.Fatalf("expected DB not to be queried, but queried sessionId %s", msgStore.querySessionId)
	}
}

func TestGetMessagesBySessionIdCursorWithCache_FallbackDB(t *testing.T) {
	dbMsg := Message{
		Uuid:      "M1000000000000000001",
		SessionId: "P_U1001_U1002",
		SendId:    "U1001",
		ReceiveId: "U1002",
		Content:   "from db",
		Type:      int8(msgtype.Text),
	}
	dbMsg.CreatedAt = time.Now()
	msgStore := &mockMsgStore{
		dbMessages: []Message{dbMsg},
	}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	sessionId := "P_U1001_U1002"

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

	key := constants.CacheKeyMessageZSet + sessionId
	count, _ := cache.ZCard(context.Background(), key)
	if count != 1 {
		t.Fatalf("expected 1 item backfilled into ZSET, got %d", count)
	}
}

func TestNextSessionSeq_MonotonicAndColdStartRecovery(t *testing.T) {
	msgStore := &mockMsgStore{
		maxSeq: 0,
	}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	sessionId := "P_U1001_U1002"

	seq1, err := svc.nextSessionSeq(context.Background(), sessionId)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq1 != 1 {
		t.Fatalf("expected seq 1, got %d", seq1)
	}

	seq2, err := svc.nextSessionSeq(context.Background(), sessionId)
	if err != nil || seq2 != 2 {
		t.Fatalf("expected seq 2, got %d (err: %v)", seq2, err)
	}
	seq3, err := svc.nextSessionSeq(context.Background(), sessionId)
	if err != nil || seq3 != 3 {
		t.Fatalf("expected seq 3, got %d (err: %v)", seq3, err)
	}

	_ = cache.Delete(context.Background(), constants.CacheKeySessionSeq+sessionId)
	msgStore.maxSeq = 50

	seqRecovered, err := svc.nextSessionSeq(context.Background(), sessionId)
	if err != nil {
		t.Fatalf("unexpected error on cold start recovery: %v", err)
	}
	if seqRecovered != 51 {
		t.Fatalf("expected calibrated seq 51, got %d", seqRecovered)
	}

	seqNext, err := svc.nextSessionSeq(context.Background(), sessionId)
	if err != nil || seqNext != 52 {
		t.Fatalf("expected next seq 52, got %d (err: %v)", seqNext, err)
	}
}

func TestSyncMessages(t *testing.T) {
	msgStore := &mockMsgStore{
		syncMessages: []Message{
			{
				Uuid:      "M1001",
				SessionId: "P_U1001_U1002",
				SendId:    "U1001",
				ReceiveId: "U1002",
				Content:   "hello seq 1",
				SeqId:     1,
			},
			{
				Uuid:      "M1002",
				SessionId: "P_U1001_U1002",
				SendId:    "U1002",
				ReceiveId: "U1001",
				Content:   "world seq 2",
				SeqId:     2,
			},
		},
	}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	_, err := svc.SyncMessages(context.Background(), "U9999", "P_U1001_U1002", 0, 10)
	if err == nil {
		t.Fatal("expected forbidden error for non-participant")
	}

	res, err := svc.SyncMessages(context.Background(), "U1001", "P_U1001_U1002", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(res))
	}
	if res[0].SeqId != 1 || res[1].SeqId != 2 {
		t.Fatalf("expected seq 1 and 2, got %d and %d", res[0].SeqId, res[1].SeqId)
	}
}

type mockFriendshipClient struct {
	friendshippb.FriendshipServiceClient
	status int32
}

func (m *mockFriendshipClient) CheckFriendship(ctx context.Context, in *friendshippb.CheckFriendshipRequest, opts ...grpc.CallOption) (*friendshippb.CheckFriendshipResponse, error) {
	return &friendshippb.CheckFriendshipResponse{Status: m.status}, nil
}

func TestCheckSendPermission_FriendshipStatuses(t *testing.T) {
	svc := NewMessageService(&mockMsgStore{}, &mockSessStore{}, newMockCache())
	ctx := context.Background()

	oldClient := grpcinfra.FriendshipClient
	defer func() {
		grpcinfra.FriendshipClient = oldClient
	}()

	tests := []struct {
		name          string
		status        int32
		expectedError string
	}{
		{
			name:          "Status 0 - 非好友，拦截并提示添加好友",
			status:        0,
			expectedError: "你们不是好友，请先添加好友",
		},
		{
			name:          "Status 1 - 正常好友，鉴权通过放行",
			status:        1,
			expectedError: "",
		},
		{
			name:          "Status 2 - 我拉黑了对方，微信同款单向特权放行发送",
			status:        2,
			expectedError: "",
		},
		{
			name:          "Status 3 - 被对方拉黑，拦截并提示拒收",
			status:        3,
			expectedError: "消息已发出，但被对方拒收了",
		},
		{
			name:          "Status 4 - 被对方单向删除（僵尸粉），拦截并提示朋友验证",
			status:        4,
			expectedError: "开启了朋友验证，你还不是他（她）朋友",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcinfra.FriendshipClient = &mockFriendshipClient{status: tt.status}
			err := svc.checkSendPermission(ctx, "U1001", "U1002")
			if tt.expectedError == "" {
				if err != nil {
					t.Fatalf("expected nil error for status %d, got: %v", tt.status, err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error for status %d, got nil", tt.status)
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("expected error containing %q, got: %v", tt.expectedError, err)
				}
			}
		})
	}
}
