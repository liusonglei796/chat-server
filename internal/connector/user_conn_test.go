package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"

	messagepb "kama_chat_server/api/gen/message"
)

// TestUserConn_WriteAndBackpressure 验证基于 coder/websocket 的异步写泵、并发写安全与背压机制
func TestUserConn_WriteAndBackpressure(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	defer hub.Close()

	var (
		serverConnLock sync.Mutex
		serverUserConn *UserConn
		connReady      = make(chan struct{})
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		serverConnLock.Lock()
		client := NewUserConn(c, "U_coder_ws_test")
		hub.RegisterClient(client)
		go client.readPump(hub)
		serverUserConn = client
		serverConnLock.Unlock()
		close(connReady)
	}))
	defer server.Close()

	// 客户端使用 coder/websocket 连接服务端
	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	assert.NoError(t, err)
	defer clientConn.CloseNow()

	select {
	case <-connReady:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for connection upgrade")
	}

	serverConnLock.Lock()
	userConn := serverUserConn
	serverConnLock.Unlock()
	assert.NotNil(t, userConn)

	received := make(chan string, 100)
	readerCtx, readerCancel := context.WithCancel(context.Background())
	defer readerCancel()
	go func() {
		for {
			mt, data, err := clientConn.Read(readerCtx)
			if err != nil {
				return
			}
			if mt == websocket.MessageText {
				received <- string(data)
			}
		}
	}()

	// 消费初始欢迎消息
	select {
	case <-received:
	case <-time.After(2 * time.Second):
	}

	// 1. 测试单条消息非阻塞安全发送
	userConn.SafeSend([]byte("hello coder write"), "msg-1")
	select {
	case msg := <-received:
		assert.Equal(t, "hello coder write", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for single message from writePump")
	}

	// 2. 并发发送 50 条消息，验证下行并发写安全
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			userConn.SafeSend([]byte("bulk_msg"), fmt.Sprintf("uuid-%d", idx))
		}(i)
	}
	wg.Wait()

	count := 0
	timeout := time.After(3 * time.Second)
	for count < 50 {
		select {
		case msg := <-received:
			assert.Equal(t, "bulk_msg", msg)
			count++
		case <-timeout:
			t.Fatalf("timeout waiting for bulk messages, received only %d/50", count)
		}
	}

	// 3. 验证 SendPing
	assert.NoError(t, userConn.SendPing())

	// 4. 验证 Context 取消/关闭状态下的拦截
	userConn.cancel()
	userConn.SafeSend([]byte("after closed"), "closed-1")
}

// TestUserConn_DecoupledOnClose 验证 UserConn 脱离 ConnHub 纯净创建，并通过 OnClose 优雅触发解耦回调
func TestUserConn_DecoupledOnClose(t *testing.T) {
	client := NewUserConn(nil, "U_decoupled")
	assert.NotNil(t, client)
	assert.Equal(t, "U_decoupled", client.Uuid)

	var closedCalled atomic.Int32
	client.OnClose = func(c *UserConn) {
		assert.Equal(t, "U_decoupled", c.Uuid)
		closedCalled.Add(1)
	}

	// 触发 cleanup
	client.cleanup()
	assert.Equal(t, int32(1), closedCalled.Load())

	// 幂等性验证：二次 cleanup 不重复调用 OnClose
	client.cleanup()
	assert.Equal(t, int32(1), closedCalled.Load())
}

// TestConnHub_RegisterClientSymmetryOnClose 验证 ConnHub 注册时挂载对称注销回调
func TestConnHub_RegisterClientSymmetryOnClose(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	defer hub.Close()

	client := NewUserConn(nil, "U_sym_test")
	hub.RegisterClient(client)

	// 验证已登记进 Hub
	assert.Equal(t, client, hub.GetClient("U_sym_test"))
	assert.Equal(t, int64(1), hub.GetOnlineCount())

	// 触发连接 cleanup，验证通过 OnClose 自动从 Hub 注销
	client.cleanup()

	assert.Nil(t, hub.GetClient("U_sym_test"))
	assert.Equal(t, int64(0), hub.GetOnlineCount())
}

// TestIsPingPayload 验证心跳零拷贝极速检测
func TestIsPingPayload(t *testing.T) {
	assert.True(t, isPingPayload([]byte(`{"action":"ping"}`)))
	assert.True(t, isPingPayload([]byte(`{"action": "ping"}`)))
	assert.True(t, isPingPayload([]byte(`{"action":"PING"}`)))
	assert.False(t, isPingPayload([]byte(`{"type":"ping"}`)))
	assert.False(t, isPingPayload([]byte(`{"type": "ping"}`)))
	assert.False(t, isPingPayload([]byte(`{"action":"sync"}`)))
	assert.False(t, isPingPayload([]byte(`{"content":"ping someone"}`)))
	// 超长数据直接短路拦截
	longBytes := make([]byte, 100)
	assert.False(t, isPingPayload(longBytes))
}

// TestWsFrame_IsAction 验证控制信令 action 判定
func TestWsFrame_IsAction(t *testing.T) {
	// 1. 标准 action 字段
	h1 := &WsFrame{Action: "ping"}
	assert.True(t, h1.IsAction("ping"))
	assert.False(t, h1.IsAction("sync"))

	h2 := &WsFrame{Action: "sync", TargetId: "target1"}
	assert.True(t, h2.IsAction("sync"))

	// 2. 业务数字 type 字段 (0/2/4)，严格不被识别为 action
	h3 := &WsFrame{Type: 2}
	assert.False(t, h3.IsAction("sync"))
	assert.False(t, h3.IsAction("ping"))

	// 3. 空指针防御
	var nilHeader *WsFrame
	assert.False(t, nilHeader.IsAction("ping"))
}

// TestWsFrame_ToSendMessageRequest 验证从统一帧单次反序列化转换为 proto 请求
func TestWsFrame_ToSendMessageRequest(t *testing.T) {
	rawJSON := []byte(`{
		"session_id": "s_123",
		"type": 2,
		"content": "hello world",
		"url": "http://img.png",
		"receive_id": "u_receiver",
		"client_msg_id": "c_999",
		"timeout_ms": 4000
	}`)

	var frame WsFrame
	err := json.Unmarshal(rawJSON, &frame)
	assert.NoError(t, err)
	assert.Equal(t, int64(4000), frame.TimeoutMs)
	assert.False(t, frame.IsAction("sync"))

	req := frame.ToSendMessageRequest("u_sender")
	assert.Equal(t, "s_123", req.SessionId)
	assert.Equal(t, int32(2), req.Type)
	assert.Equal(t, "hello world", req.Content)
	assert.Equal(t, "http://img.png", req.Url)
	assert.Equal(t, "u_sender", req.SendId)
	assert.Equal(t, "u_receiver", req.ReceiveId)
	assert.Equal(t, "c_999", req.ClientMsgId)
}

// TestHandleUserMessageTask_CancelledContext 验证连接已关闭或 Context 取消时静默放弃，避免无效下游调用
func TestHandleUserMessageTask_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立刻取消

	conn := &UserConn{
		Uuid:      "test_user",
		sendQueue: make(chan []byte, 10),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 执行时不应 panic，且由于 ctx 已经被 cancel，应直接 return，不向 sendQueue 发送任何消息
	HandleUserMessageTask(conn, []byte(`{"action":"sync","target_id":"target_cancel"}`))
	assert.Equal(t, 0, len(conn.sendQueue))
}

type mockIncomingProducer struct {
	published []*messagepb.SendMessageRequest
	err       error
}

func (m *mockIncomingProducer) PublishIncomingMessage(ctx context.Context, req *messagepb.SendMessageRequest) error {
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, req)
	return nil
}

func (m *mockIncomingProducer) Close() {}

// TestResolveSessionPartitionKey 验证单聊与群聊的物理分区键严格对齐
func TestResolveSessionPartitionKey(t *testing.T) {
	// 单聊双向发送必须产出完全一致的 Partition Key
	assert.Equal(t, "P_U100_U200", ResolveSessionPartitionKey("U100", "U200"))
	assert.Equal(t, "P_U100_U200", ResolveSessionPartitionKey("U200", "U100"))
	// 群聊统一使用群 ID
	assert.Equal(t, "G999", ResolveSessionPartitionKey("U100", "G999"))
}

// TestHandleUserMessageTaskWithProducer_KafkaEchoACK 验证方案二：Connector 仅投递 Kafka，不原地发伪 ACK
func TestHandleUserMessageTaskWithProducer_KafkaEchoACK(t *testing.T) {
	conn := &UserConn{
		Uuid:      "U_alice",
		sendQueue: make(chan []byte, 10),
		ctx:       context.Background(),
	}
	producer := &mockIncomingProducer{}
	raw := []byte(`{"receive_id":"U_bob","content":"hello kafka","type":0,"client_msg_id":"c_echo_1"}`)

	HandleUserMessageTaskWithProducer(conn, raw, producer)

	assert.Len(t, producer.published, 1)
	assert.Equal(t, "U_alice", producer.published[0].SendId)
	assert.Equal(t, "U_bob", producer.published[0].ReceiveId)
	assert.Equal(t, "c_echo_1", producer.published[0].ClientMsgId)
	// Connector 不原地写入 ack，等待下游 message_service 落库后推回 Echo ACK
	assert.Equal(t, 0, len(conn.sendQueue))
}

// TestHandleUserMessageTaskWithProducer_FailFast 验证 Kafka 故障时快速失败向客户端返回 error 帧
func TestHandleUserMessageTaskWithProducer_FailFast(t *testing.T) {
	conn := &UserConn{
		Uuid:      "U_alice",
		sendQueue: make(chan []byte, 10),
		ctx:       context.Background(),
	}
	producer := &mockIncomingProducer{err: fmt.Errorf("kafka broker unreachable")}
	raw := []byte(`{"receive_id":"U_bob","content":"hello fail","type":0,"client_msg_id":"c_fail_1"}`)

	HandleUserMessageTaskWithProducer(conn, raw, producer)

	assert.Equal(t, 1, len(conn.sendQueue))
	errFrame := <-conn.sendQueue
	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(errFrame, &parsed))
	assert.Equal(t, "error", parsed["type"])
	assert.Equal(t, "c_fail_1", parsed["client_msg_id"])
}

// BenchmarkIsPingPayload 评测心跳极速检测性能与内存分配
func BenchmarkIsPingPayload(b *testing.B) {
	data := []byte(`{"action":"ping"}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = isPingPayload(data)
	}
}

// BenchmarkWsFrameSingleUnmarshal 评测统一帧单次反序列化性能
func BenchmarkWsFrameSingleUnmarshal(b *testing.B) {
	data := []byte(`{
		"session_id": "s_123",
		"type": 0,
		"content": "benchmark testing message",
		"receive_id": "u_receiver",
		"client_msg_id": "c_bench_01",
		"timeout_ms": 4000
	}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var frame WsFrame
		_ = json.Unmarshal(data, &frame)
		_ = frame.ToSendMessageRequest("u_sender")
	}
}
