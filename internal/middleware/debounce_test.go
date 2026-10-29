package middleware

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/transport"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"

	"kama_chat_server/pkg/errorx"
)

type mockDebounceStore struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (m *mockDebounceStore) SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[key] {
		return false, nil
	}
	m.keys[key] = true
	return true, nil
}

type mockTransport struct {
	operation string
	header    http.Header
}

func (m *mockTransport) Kind() transport.Kind         { return transport.KindGRPC }
func (m *mockTransport) Endpoint() string             { return "127.0.0.1:9000" }
func (m *mockTransport) Operation() string            { return m.operation }
func (m *mockTransport) RequestHeader() transport.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return &mockHeader{m.header}
}
func (m *mockTransport) ReplyHeader() transport.Header { return &mockHeader{make(http.Header)} }

type mockHeader struct {
	h http.Header
}

func (m *mockHeader) Get(key string) string          { return m.h.Get(key) }
func (m *mockHeader) Set(key, value string)          { m.h.Set(key, value) }
func (m *mockHeader) Add(key, value string)          { m.h.Add(key, value) }
func (m *mockHeader) Keys() []string                 { return nil }
func (m *mockHeader) Values(key string) []string     { return m.h.Values(key) }

type mockPayload struct {
	Name string `json:"name"`
}

func TestRequestDebounce(t *testing.T) {
	store := &mockDebounceStore{keys: make(map[string]bool)}
	mw := RequestDebounce(store)

	called := 0
	handler := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
		called++
		return "ok", nil
	})

	tr := &mockTransport{
		operation: "/group.GroupService/CreateGroup",
		header:    http.Header{"X-User-Id": []string{"U10001"}},
	}
	baseCtx := transport.NewServerContext(context.Background(), tr)
	ctx := metadata.NewIncomingContext(baseCtx, metadata.Pairs("x-user-id", "U10001"))

	t.Run("first request should pass", func(t *testing.T) {
		req := &mockPayload{Name: "group-1"}
		res, err := handler(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, "ok", res)
		assert.Equal(t, 1, called)
	})

	t.Run("second rapid request with same payload should be blocked by debounce", func(t *testing.T) {
		req := &mockPayload{Name: "group-1"}
		res, err := handler(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, 1, called) // handler not called

		var codeErr *errorx.CodeError
		assert.ErrorAs(t, err, &codeErr)
		assert.Equal(t, errorx.CodeConflict, codeErr.Code)
		assert.Equal(t, debounceMessage, codeErr.Msg)
	})

	t.Run("different payload should pass", func(t *testing.T) {
		req := &mockPayload{Name: "group-2"}
		res, err := handler(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, "ok", res)
		assert.Equal(t, 2, called)
	})
}
