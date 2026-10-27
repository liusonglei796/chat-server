package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/lesismal/nbio/nbhttp"
	nbws "github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/stretchr/testify/assert"
)

// TestUserConn_ReactorWriteAndBackpressure 验证 Reactor 模式下的非阻塞写入、并发写安全与背压机制
func TestUserConn_ReactorWriteAndBackpressure(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	defer hub.Close()

	// 1. 获取动态空闲端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := listener.Addr().String()
	_ = listener.Close()

	upgrader := nbws.NewUpgrader()
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	var (
		serverConnLock sync.Mutex
		serverUserConn *UserConn
		connReady      = make(chan struct{})
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnLock.Lock()
		serverUserConn = InitReactorClient(c, "U_reactor_test", hub)
		serverConnLock.Unlock()
		close(connReady)
	})

	engine := nbhttp.NewEngine(nbhttp.Config{
		Network: "tcp",
		Addrs:   []string{addr},
		Handler: mux,
	})
	assert.NoError(t, engine.Start())
	defer func() { _ = engine.Shutdown(context.Background()) }()

	// 2. 客户端使用 RFC 6455 标准 WebSocket 连接 Reactor 服务端
	wsURL := fmt.Sprintf("ws://%s/ws", addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	assert.NoError(t, err)
	defer clientConn.CloseNow()

	select {
	case <-connReady:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for reactor connection upgrade")
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

	// 3. 测试单条消息非阻塞安全发送
	userConn.SafeSend([]byte("hello reactor write"), "msg-1")
	select {
	case msg := <-received:
		assert.Equal(t, "hello reactor write", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for single message from reactor write")
	}

	// 4. 并发发送 50 条消息，验证 Reactor 下行并发写安全
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

	// 5. 验证 SendPing
	assert.NoError(t, userConn.SendPing())

	// 6. 验证关闭状态下的背压拦截
	userConn.closed.Store(true)
	userConn.SafeSend([]byte("after closed"), "closed-1")
}
