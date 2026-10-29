package debounce

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store 防抖底层存储接口
type Store interface {
	SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
}

// RedisStore 基于 Redis SETNX 的防抖存储
type RedisStore struct {
	client redis.UniversalClient
}

// NewRedisStore 创建 Redis 防抖存储
func NewRedisStore(client redis.UniversalClient) Store {
	return &RedisStore{client: client}
}

func (s *RedisStore) SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, value, expiration).Result()
}
