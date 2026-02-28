package ops

import (
	"context"
	"time"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

func Incr(client *redis.Client, ctx context.Context, key string) (int64, error) {
	val, err := client.Incr(ctx, key).Result()
	if err != nil {
		return 0, errorx.Wrapf(err, errorx.CodeCacheError, "redis incr key %s", key)
	}
	return val, nil
}

func Expire(client *redis.Client, ctx context.Context, key string, ttl time.Duration) error {
	if err := client.Expire(ctx, key, ttl).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis expire key %s", key)
	}
	return nil
}
