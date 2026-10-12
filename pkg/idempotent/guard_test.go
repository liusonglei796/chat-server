package idempotent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGuard struct {
	executeFunc func(ctx context.Context, key string, leaseTTL time.Duration, successTTL time.Duration, fn func(ctx context.Context) (string, error)) (string, bool, error)
}

func (m *mockGuard) Execute(ctx context.Context, key string, leaseTTL time.Duration, successTTL time.Duration, fn func(ctx context.Context) (string, error)) (string, bool, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, key, leaseTTL, successTTL, fn)
	}
	res, err := fn(ctx)
	return res, false, err
}

func TestExecute_NilGuardOrEmptyKey(t *testing.T) {
	ctx := context.Background()

	// 1. Guard 为 nil 时正常直连执行
	var g Guard = nil
	called := false
	res, isReplay, err := ExecuteJSON(ctx, g, "any-key", time.Second, time.Minute, func(c context.Context) (map[string]string, error) {
		called = true
		return map[string]string{"foo": "bar"}, nil
	})
	require.NoError(t, err)
	assert.False(t, isReplay)
	assert.True(t, called)
	assert.Equal(t, "bar", res["foo"])

	// 2. key 为空时正常直连执行
	mock := &mockGuard{}
	res2, isReplay2, err2 := ExecuteJSON(ctx, mock, "", time.Second, time.Minute, func(c context.Context) (string, error) {
		return "hello", nil
	})
	require.NoError(t, err2)
	assert.False(t, isReplay2)
	assert.Equal(t, "hello", res2)
}

func TestExecuteJSON_SuccessAndReplay(t *testing.T) {
	ctx := context.Background()

	type OrderResult struct {
		OrderID string `json:"order_id"`
		Amount  int    `json:"amount"`
	}

	mockStore := make(map[string]string)
	mock := &mockGuard{
		executeFunc: func(c context.Context, key string, leaseTTL time.Duration, successTTL time.Duration, fn func(context.Context) (string, error)) (string, bool, error) {
			if val, ok := mockStore[key]; ok {
				return val, true, nil // 模拟命中幂等回放
			}
			val, err := fn(c)
			if err != nil {
				return "", false, err
			}
			mockStore[key] = val
			return val, false, nil
		},
	}

	callCount := 0
	action := func(c context.Context) (OrderResult, error) {
		callCount++
		return OrderResult{OrderID: "ORD_1001", Amount: 99}, nil
	}

	// 第一次调用：执行业务逻辑
	res1, isReplay1, err1 := ExecuteJSON(ctx, mock, "k1", 5*time.Second, time.Hour, action)
	require.NoError(t, err1)
	assert.False(t, isReplay1)
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "ORD_1001", res1.OrderID)

	// 第二次调用（相同 key）：命中幂等回放，不再执行业务闭包
	res2, isReplay2, err2 := ExecuteJSON(ctx, mock, "k1", 5*time.Second, time.Hour, action)
	require.NoError(t, err2)
	assert.True(t, isReplay2)
	assert.Equal(t, 1, callCount) // callCount 未增加
	assert.Equal(t, "ORD_1001", res2.OrderID)
	assert.Equal(t, 99, res2.Amount)
}

func TestExecuteJSON_BizErrorPropagation(t *testing.T) {
	ctx := context.Background()
	mock := &mockGuard{}

	expectedErr := errors.New("insufficient balance")
	_, isReplay, err := ExecuteJSON(ctx, mock, "k2", 5*time.Second, time.Hour, func(c context.Context) (int, error) {
		return 0, expectedErr
	})
	assert.ErrorIs(t, err, expectedErr)
	assert.False(t, isReplay)
}

func TestRedisGuard_DegradeOnConnFailure(t *testing.T) {
	// 指向一个未连通的端口测试降级容灾能力
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:54321", // 不可达端口
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
	})
	defer client.Close()

	guard := NewRedisGuard(client)
	called := false

	res, isReplay, err := guard.Execute(context.Background(), "test-fail-key", 2*time.Second, time.Hour, func(ctx context.Context) (string, error) {
		called = true
		return "fallback_success", nil
	})

	// 断言：Redis 不可用时优雅降级放行执行业务逻辑，不阻塞主流程
	require.NoError(t, err)
	assert.False(t, isReplay)
	assert.True(t, called)
	assert.Equal(t, "fallback_success", res)
}
