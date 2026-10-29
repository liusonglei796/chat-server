package interceptor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCheckRemainingBudget(t *testing.T) {
	t.Run("no deadline in context should pass", func(t *testing.T) {
		ctx := context.Background()
		err := CheckRemainingBudget(ctx, 100*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("sufficient remaining budget should pass", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := CheckRemainingBudget(ctx, 100*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("insufficient remaining budget should fail with DeadlineExceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		err := CheckRemainingBudget(ctx, 100*time.Millisecond)
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.DeadlineExceeded, st.Code())
	})
}

func TestServerDeadlineBudgetMiddleware(t *testing.T) {
	mw := ServerDeadlineBudgetMiddleware(100 * time.Millisecond)
	handlerCalled := false
	handler := mw(func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	})

	t.Run("passes when budget is sufficient", func(t *testing.T) {
		handlerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		res, err := handler(ctx, "req")
		assert.NoError(t, err)
		assert.Equal(t, "ok", res)
		assert.True(t, handlerCalled)
	})

	t.Run("aborts when budget is insufficient", func(t *testing.T) {
		handlerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		res, err := handler(ctx, "req")
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.False(t, handlerCalled)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.DeadlineExceeded, st.Code())
	})
}

func TestServerDeadlineBudgetInterceptor(t *testing.T) {
	interceptor := ServerDeadlineBudgetInterceptor(100 * time.Millisecond)
	handlerCalled := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "resp", nil
	}

	t.Run("passes when budget is sufficient", func(t *testing.T) {
		handlerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		res, err := interceptor(ctx, "req", &grpc.UnaryServerInfo{}, handler)
		assert.NoError(t, err)
		assert.Equal(t, "resp", res)
		assert.True(t, handlerCalled)
	})

	t.Run("aborts when budget is insufficient", func(t *testing.T) {
		handlerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		res, err := interceptor(ctx, "req", &grpc.UnaryServerInfo{}, handler)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.False(t, handlerCalled)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.DeadlineExceeded, st.Code())
	})
}

func TestRetryBudgetInterceptor(t *testing.T) {
	interceptor := RetryBudgetInterceptor(150 * time.Millisecond)
	invokerCalled := false
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	t.Run("passes when budget is sufficient", func(t *testing.T) {
		invokerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		err := interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
		assert.NoError(t, err)
		assert.True(t, invokerCalled)
	})

	t.Run("aborts when budget is insufficient", func(t *testing.T) {
		invokerCalled = false
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
		assert.Error(t, err)
		assert.False(t, invokerCalled)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.DeadlineExceeded, st.Code())
	})
}
