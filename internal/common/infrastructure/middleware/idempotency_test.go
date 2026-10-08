package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// mockCacheService 内存版 Mock CacheService，用于中间件单测
type mockCacheService struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMockCacheService() *mockCacheService {
	return &mockCacheService{data: make(map[string]string)}
}

func (m *mockCacheService) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockCacheService) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[key], nil
}

func (m *mockCacheService) GetOrError(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	if !ok {
		return "", nil
	}
	return val, nil
}

func (m *mockCacheService) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	return "", nil
}

func (m *mockCacheService) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCacheService) DeleteByPattern(ctx context.Context, patterns ...string) error {
	return nil
}

func (m *mockCacheService) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = value
	return true, nil
}

func (m *mockCacheService) Incr(ctx context.Context, key string) (int64, error) { return 0, nil }
func (m *mockCacheService) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}
func (m *mockCacheService) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (m *mockCacheService) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	return nil, nil
}
func (m *mockCacheService) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (m *mockCacheService) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return nil
}
func (m *mockCacheService) ZRevRangeByScore(ctx context.Context, key string, max, min string, offset, count int64) ([]string, error) {
	return nil, nil
}
func (m *mockCacheService) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	return nil
}
func (m *mockCacheService) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (m *mockCacheService) ZCard(ctx context.Context, key string) (int64, error) { return 0, nil }
func (m *mockCacheService) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return 0, nil
}

func TestIdempotency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newMockCacheService()

	var handlerExecCount int32

	r := gin.New()
	r.Use(Idempotency(cache, time.Hour))
	r.POST("/test/write", func(c *gin.Context) {
		atomic.AddInt32(&handlerExecCount, 1)
		c.JSON(http.StatusOK, gin.H{
			"code": 1000,
			"msg":  "success",
			"data": gin.H{"id": 123},
		})
	})

	// 1. 第一次请求：携带 X-Idempotency-Key
	req1 := httptest.NewRequest("POST", "/test/write", nil)
	req1.Header.Set("X-Idempotency-Key", "req-key-001")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), `"code":1000`)
	assert.Equal(t, int32(1), atomic.LoadInt32(&handlerExecCount))
	assert.Empty(t, w1.Header().Get("X-Idempotent-Replay"))

	// 2. 第二次请求：携带相同 X-Idempotency-Key，验证回放且 Handler 不重复执行
	req2 := httptest.NewRequest("POST", "/test/write", nil)
	req2.Header.Set("X-Idempotency-Key", "req-key-001")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())
	assert.Equal(t, int32(1), atomic.LoadInt32(&handlerExecCount), "Handler 应该只执行了 1 次")
	assert.Equal(t, "true", w2.Header().Get("X-Idempotent-Replay"))

	// 3. 写请求不带 X-Idempotency-Key，验证强制拦截返回 400
	req3 := httptest.NewRequest("POST", "/test/write", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusBadRequest, w3.Code)
	assert.Contains(t, w3.Body.String(), "必须提供 X-Idempotency-Key")
	assert.Equal(t, int32(1), atomic.LoadInt32(&handlerExecCount), "未带 Key 应被拦截，Handler 不得执行")

	// 4. 读请求 (GET) 不带 X-Idempotency-Key，验证正常放行
	r.GET("/test/read", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 1000})
	})
	reqGet := httptest.NewRequest("GET", "/test/read", nil)
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)
	assert.Equal(t, http.StatusOK, wGet.Code)

	// 5. 测试正在执行中（并发）返回 429
	cache.data["idemp:192.0.2.1:POST:/test/write:in-flight-key"] = inProgressValue
	req4 := httptest.NewRequest("POST", "/test/write", nil)
	req4.Header.Set("X-Idempotency-Key", "in-flight-key")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)

	assert.Equal(t, http.StatusTooManyRequests, w4.Code)
	assert.Contains(t, w4.Body.String(), "请求正在处理中")
	assert.Equal(t, int32(1), atomic.LoadInt32(&handlerExecCount))
}
