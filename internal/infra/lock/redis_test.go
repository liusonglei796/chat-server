package lock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRedisLock_NilClient(t *testing.T) {
	l := NewRedis(nil)
	ctx := context.Background()

	acquired, err := l.Acquire(ctx, "test_key", "token123", time.Second)
	assert.Error(t, err)
	assert.False(t, acquired)

	err = l.Release(ctx, "test_key", "token123")
	assert.Error(t, err)
}
