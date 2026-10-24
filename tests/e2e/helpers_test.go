package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// headerValue 从 Kafka 记录头里取值（franz-go 的 kgo.RecordHeader 没有 Get 方法）
func headerValue(headers []kgo.RecordHeader, key string) string {
	for _, h := range headers {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

// unmarshalData 从统一响应体的 data 字段解析出目标结构
func unmarshalData(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return fmt.Errorf("响应中没有 data 字段")
	}
	return json.Unmarshal(raw, out)
}

// ---------------------------------------------------------------------------
// 环境配置
// ---------------------------------------------------------------------------

type testEnv struct {
	EnvoyURL     string // 生产形态边缘：Envoy grpc_json_transcoder
	GatewayURL   string // Gin 网关（带 JWT 认证）
	ToxiAPI      string // Toxiproxy REST API
	MySQLHost    string
	MySQLPort    string
	MySQLUser    string
	MySQLPass    string
	KafkaBrokers string
	RedisHost    string
	RedisPort    string

	mu     sync.Mutex
	tokens map[string]string // userId -> accessToken
}

// putToken 记录用户登录得到的 access_token
func (e *testEnv) putToken(userID, token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tokens == nil {
		e.tokens = make(map[string]string)
	}
	e.tokens[userID] = token
}

// tokenOf 取某个用户的 access_token
func (e *testEnv) tokenOf(t *testing.T, userID string) string {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	tok, ok := e.tokens[userID]
	if !ok || tok == "" {
		t.Fatalf("没有 %s 的 access_token（是否没有先 login？）", userID)
	}
	return tok
}

// waitGatewayRoute 等待网关把 user:gateway:<userId> 写进 Redis
// （ConnHub.RegisterClient 在 WS 连接建立时异步写入，TTL 120s）
func (e *testEnv) waitGatewayRoute(t *testing.T, userID string, timeout time.Duration) {
	t.Helper()
	rdb := e.redisClient(t)
	key := "user:gateway:" + userID
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		v, err := rdb.Get(context.Background(), key).Result()
		if err == nil && v != "" {
			t.Logf("[夹具] 网关路由已登记 %s = %s", key, v)
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("等待 Redis 网关路由 %s 超时（WS 未连上？）", key)
}

func loadEnv(t *testing.T) *testEnv {
	t.Helper()
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	return &testEnv{
		EnvoyURL:     get("E2E_ENVOY_URL", "http://envoy:9080"),
		GatewayURL:   get("E2E_GATEWAY_URL", "http://chat-server:8000"),
		ToxiAPI:      get("E2E_TOXIPROXY_API", "http://toxiproxy:8474"),
		MySQLHost:    get("E2E_MYSQL_HOST", "mysql"),
		MySQLPort:    get("E2E_MYSQL_PORT", "3306"),
		MySQLUser:    get("E2E_MYSQL_USER", "root"),
		MySQLPass:    get("E2E_MYSQL_PASSWORD", "root123456"),
		KafkaBrokers: get("E2E_KAFKA_BROKERS", "kafka:9092"),
		RedisHost:    get("E2E_REDIS_HOST", "redis"),
		RedisPort:    get("E2E_REDIS_PORT", "6379"),
	}
}

// ---------------------------------------------------------------------------
// HTTP 调用
// ---------------------------------------------------------------------------

// apiResp 统一承载两种完全不同的响应契约：
//   - Gin 网关：HTTP 200 + {"code":1001,"msg":"...","data":null}
//   - Envoy 转码：HTTP 500 + {"code":2,"message":"..."}（gRPC google.rpc.Status 信封）
// 因此断言必须分别看 Status 与 Code，不能只看其中一个。
type apiResp struct {
	Status int
	Raw    []byte
	Code   int             `json:"code"`
	Msg    string          `json:"msg"`
	Message string         `json:"message"`
	Data   json.RawMessage `json:"data"`
}

// Body 摘要，用于失败时的可读输出
func (r *apiResp) Summary() string {
	s := fmt.Sprintf("HTTP=%d", r.Status)
	if r.Code != 0 {
		s += fmt.Sprintf(" code=%d", r.Code)
	}
	msg := r.Msg
	if msg == "" {
		msg = r.Message
	}
	if msg != "" {
		s += fmt.Sprintf(" msg=%q", msg)
	}
	if len(r.Raw) > 0 && len(r.Raw) < 400 {
		s += fmt.Sprintf(" raw=%s", string(r.Raw))
	}
	return s
}

func (e *testEnv) call(t *testing.T, method, url, token string, body any, extra map[string]string) *apiResp {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("[HTTP] %s %s 失败: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := &apiResp{Status: resp.StatusCode, Raw: raw}
	_ = json.Unmarshal(raw, out)
	return out
}

// ---------------------------------------------------------------------------
// Toxiproxy 控制面
// ---------------------------------------------------------------------------

type toxiProxy struct {
	Name     string `json:"name"`
	Listen   string `json:"listen"`
	Upstream string `json:"upstream"`
	Enabled  bool   `json:"enabled"`
}

func (e *testEnv) toxiRequest(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.ToxiAPI+path, rdr)
	if err != nil {
		t.Fatalf("构造 toxiproxy 请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Toxiproxy-Client")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("[Toxiproxy] %s %s 失败: %v", method, path, err)
	}
	return resp
}

// toxiAddToxic 注入故障
func (e *testEnv) toxiAddToxic(t *testing.T, proxy, name, typ, stream string, attrs map[string]any) {
	t.Helper()
	resp := e.toxiRequest(t, http.MethodPost, "/proxies/"+proxy+"/toxics", map[string]any{
		"name": name, "type": typ, "stream": stream, "toxicity": 1.0, "attributes": attrs,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("注入 toxic %s 失败 (code=%d): %s", name, resp.StatusCode, string(b))
	}
	t.Logf("[Toxiproxy] 已注入 %s (%s/%s) 到 %s: %v", name, typ, stream, proxy, attrs)
}

// toxiRemoveToxic 移除故障
func (e *testEnv) toxiRemoveToxic(t *testing.T, proxy, name string) {
	t.Helper()
	resp := e.toxiRequest(t, http.MethodDelete, "/proxies/"+proxy+"/toxics/"+name, nil)
	if resp != nil {
		resp.Body.Close()
	}
	t.Logf("[Toxiproxy] 已移除 toxic %s", name)
}

// toxiClear 清空某代理上的全部 toxic
func (e *testEnv) toxiClear(t *testing.T, proxy string) {
	t.Helper()
	resp := e.toxiRequest(t, http.MethodGet, "/proxies/"+proxy+"/toxics", nil)
	if resp != nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var toxics []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(b, &toxics); err == nil {
			for _, tx := range toxics {
				e.toxiRemoveToxic(t, proxy, tx.Name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 数据库直连（只读断言用；刻意绕过 toxiproxy，避免观测手段干扰被测系统）
// ---------------------------------------------------------------------------

func (e *testEnv) openDB(t *testing.T, dbName string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		e.MySQLUser, e.MySQLPass, e.MySQLHost, e.MySQLPort, dbName)
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("直连 MySQL(%s) 失败: %v", dbName, err)
	}
	return db
}

type fsRow struct {
	UserId   string
	FriendId string
	Status   int8
	Version  int64
}

// friendshipRow 读取单向好友关系行
func friendshipRow(t *testing.T, db *gorm.DB, userId, friendId string) (fsRow, bool) {
	t.Helper()
	var r fsRow
	err := db.Raw("SELECT user_id, friend_id, status, version FROM user_friendship WHERE user_id = ? AND friend_id = ?",
		userId, friendId).Scan(&r).Error
	if err != nil {
		t.Fatalf("读取 user_friendship 失败: %v", err)
	}
	if r.UserId == "" {
		return r, false
	}
	return r, true
}

// friendshipRowNoFatal 同 friendshipRow，但不 Fatal（供等待轮询使用）
func friendshipRowNoFatal(db *gorm.DB, userId, friendId string) (fsRow, bool) {
	var r fsRow
	err := db.Raw("SELECT user_id, friend_id, status, version FROM user_friendship WHERE user_id = ? AND friend_id = ?",
		userId, friendId).Scan(&r).Error
	if err != nil {
		return r, false
	}
	if r.UserId == "" {
		return r, false
	}
	return r, true
}

func outboxCount(t *testing.T, db *gorm.DB, payloadLike string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM outbox WHERE payload LIKE ?", "%"+payloadLike+"%").Scan(&n).Error; err != nil {
		t.Fatalf("统计 outbox 失败: %v", err)
	}
	return n
}

func sessionCount(t *testing.T, db *gorm.DB, a, b string) int64 {
	t.Helper()
	var n int64
	err := db.Raw("SELECT COUNT(*) FROM session WHERE (send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)",
		a, b, b, a).Scan(&n).Error
	if err != nil {
		t.Fatalf("统计 session 失败: %v", err)
	}
	return n
}

// insertSession 直接造一条会话行作为前置夹具。
// 目的是把断言聚焦在「拉黑是否会删掉这条会话」，而不是消息发送链路。
func insertSession(t *testing.T, db *gorm.DB, uuid, sendId, receiveId string) {
	t.Helper()
	err := db.Exec(
		"INSERT INTO session (uuid, send_id, receive_id, last_message, last_message_type, last_seq_id, is_pinned, created_at, updated_at) "+
			"VALUES (?, ?, ?, '', 0, 0, 0, NOW(), NOW())",
		uuid, sendId, receiveId).Error
	if err != nil {
		t.Fatalf("插入 session 夹具失败: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Redis
// ---------------------------------------------------------------------------

func (e *testEnv) redisClient(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: e.RedisHost + ":" + e.RedisPort})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("连接 Redis 失败: %v", err)
	}
	return rdb
}

// ---------------------------------------------------------------------------
// Kafka 事件读取
// ---------------------------------------------------------------------------

type domainEvent struct {
	EventType string
	Payload   string
}

type eventReader struct {
	cl   *kgo.Client
	mu   sync.Mutex
	evs  []domainEvent
	stop chan struct{}
}

func newEventReader(t *testing.T, brokers string) *eventReader {
	t.Helper()
	group := fmt.Sprintf("e2e-observer-%d", time.Now().UnixNano())
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumeTopics("domain_events"),
		kgo.ConsumerGroup(group),
		// 全新 group + 从头消费，确保能看到本次运行期间产生的所有事件
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		t.Fatalf("创建 Kafka 消费者失败: %v", err)
	}
	r := &eventReader{cl: cl, stop: make(chan struct{})}
	go func() {
		for {
			select {
			case <-r.stop:
				return
			default:
			}
			fetches := cl.PollFetches(context.Background())
			if errs := fetches.Errors(); len(errs) > 0 {
				continue
			}
			fetches.EachRecord(func(rec *kgo.Record) {
				r.mu.Lock()
				r.evs = append(r.evs, domainEvent{
					EventType: headerValue(rec.Headers, "event_type"),
					Payload:   string(rec.Value),
				})
				r.mu.Unlock()
			})
		}
	}()
	return r
}

func (r *eventReader) close() {
	close(r.stop)
	r.cl.Close()
}

// countEvents 统计 payload 中包含 needle 且事件类型为 eventType 的条数
func (r *eventReader) countEvents(eventType, needle string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.evs {
		if e.EventType == eventType && strings.Contains(e.Payload, needle) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// WebSocket 客户端
// ---------------------------------------------------------------------------

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
	msgs [][]byte
}

func dialWS(t *testing.T, gatewayURL, token string) *wsClient {
	t.Helper()
	wsURL := strings.Replace(gatewayURL, "http://", "ws://", 1) + "/ws"
	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, hdr)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("[WS] 握手失败 %s (http=%d): %v", wsURL, status, err)
	}
	c := &wsClient{conn: conn}
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			c.mu.Lock()
			c.msgs = append(c.msgs, data)
			c.mu.Unlock()
		}
	}()
	return c
}

func (c *wsClient) close() { c.conn.Close() }

// waitMessage 等待满足条件的消息
func (c *wsClient) waitMessage(t *testing.T, timeout time.Duration, pred func([]byte) bool) ([]byte, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, m := range c.msgs {
			if pred(m) {
				c.mu.Unlock()
				return m, true
			}
		}
		c.mu.Unlock()
		time.Sleep(200 * time.Millisecond)
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// 通用断言辅助
// ---------------------------------------------------------------------------

// breakerCooldown 等待客户端熔断器冷却。
//
// pkg/interceptor/breaker.go 使用 sony/gobreaker，默认 ReadyToTrip =
// 连续 5 次失败即打开，Timeout(冷却) = 10s。
// 而 IsSuccessful 把 codes.Unknown（即本项目所有业务错误）判定为「失败」，
// 所以连续的业务错误会误触发熔断，把后续断言全部变成「服务繁忙」。
// 熔断器是按 gRPC 方法维度维护的，BlackFriend 的失败会持续影响后续 BlackFriend 场景。
func breakerCooldown(t *testing.T, why string) {
	t.Helper()
	// 必须等过 gobreaker 的 Interval(30s)：只有超过这个窗口，
	// 下一次请求才会触发 counts.clear() 把 ConsecutiveFailures 归零。
	// 只等 10s 是不够的 —— 连续失败会跨场景累加，
	// 导致后面的场景实际在测「熔断器打开」而不是在测业务语义。
	t.Logf("[熔断冷却] 等待 32s：%s", why)
	time.Sleep(32 * time.Second)
}

func dbPing(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("MySQL ping 失败: %v", err)
	}
}

var _ = sql.ErrNoRows
