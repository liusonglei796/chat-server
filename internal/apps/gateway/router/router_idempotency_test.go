package router

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

	"kama_chat_server/internal/common/infrastructure/middleware"
)

type testCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func newTestCache() *testCache {
	return &testCache{data: make(map[string]string)}
}

func (c *testCache) Set(ctx context.Context, key string, val string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = val
	return nil
}

func (c *testCache) Get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key], nil
}

func (c *testCache) GetOrError(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key], nil
}

func (c *testCache) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	return "", nil
}

func (c *testCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

func (c *testCache) DeleteByPattern(ctx context.Context, patterns ...string) error {
	return nil
}

func (c *testCache) SetNX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[key]; ok {
		return false, nil
	}
	c.data[key] = val
	return true, nil
}

func (c *testCache) Incr(ctx context.Context, key string) (int64, error) { return 0, nil }
func (c *testCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}
func (c *testCache) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *testCache) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	return nil, nil
}
func (c *testCache) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *testCache) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return nil
}
func (c *testCache) ZRevRangeByScore(ctx context.Context, key string, max, min string, offset, count int64) ([]string, error) {
	return nil, nil
}
func (c *testCache) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	return nil
}
func (c *testCache) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return nil
}
func (c *testCache) ZCard(ctx context.Context, key string) (int64, error) { return 0, nil }
func (c *testCache) ZScore(ctx context.Context, key string, member string) (float64, error) {
	return 0, nil
}

func TestRouter_IdempotencyIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newTestCache()

	r := gin.New()

	// 模拟已认证私有路由组（挂载了用户上下文和幂等中间件）
	var writeOpExecutions int32
	private := r.Group("/api")
	private.Use(func(c *gin.Context) {
		c.Set("user_id", "U123456789")
		c.Next()
	})
	private.Use(middleware.Idempotency(cache, 24*time.Hour))
	{
		private.POST("/groups", func(c *gin.Context) {
			atomic.AddInt32(&writeOpExecutions, 1)
			c.JSON(http.StatusOK, gin.H{
				"code": 1000,
				"msg":  "group created successfully",
				"data": gin.H{"group_id": "G999999"},
			})
		})
	}

	// 第一次建群请求，携带 X-Idempotency-Key
	req1 := httptest.NewRequest("POST", "/api/groups", nil)
	req1.Header.Set("X-Idempotency-Key", "idemp-test-create-group-1")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), "G999999")
	assert.Equal(t, int32(1), atomic.LoadInt32(&writeOpExecutions))
	assert.Empty(t, w1.Header().Get("X-Idempotent-Replay"))

	// 第二次由于弱网重试，携带完全相同的 X-Idempotency-Key
	req2 := httptest.NewRequest("POST", "/api/groups", nil)
	req2.Header.Set("X-Idempotency-Key", "idemp-test-create-group-1")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())
	assert.Equal(t, "true", w2.Header().Get("X-Idempotent-Replay"))
	// 验证底层业务处理器完全没有被重复执行，避免产生重复群组！
	assert.Equal(t, int32(1), atomic.LoadInt32(&writeOpExecutions), "底层业务 Handler 必须只被执行 1 次！")

	// 第三次：未携带 X-Idempotency-Key，验证强制拦截返回 400
	req3 := httptest.NewRequest("POST", "/api/groups", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusBadRequest, w3.Code)
	assert.Contains(t, w3.Body.String(), "必须提供 X-Idempotency-Key")
	assert.Equal(t, int32(1), atomic.LoadInt32(&writeOpExecutions), "未带 Key 被拦截，Handler 不得执行")
}
