package ops

import (
	"context"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

func Delete(client *redis.Client, ctx context.Context, key string) error {
	exists, err := client.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 {
		if err := client.Unlink(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink key %s", key)
		}
	}
	return nil
}

func DeleteByPattern(client *redis.Client, ctx context.Context, patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}

	for _, pattern := range patterns {
		var cursor uint64
		for {
			var keys []string
			var err error
			keys, cursor, err = client.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
			}
			if len(keys) > 0 {
				if err := client.Unlink(ctx, keys...).Err(); err != nil {
					return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
				}
			}
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}
