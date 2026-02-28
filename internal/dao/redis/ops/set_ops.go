package ops

import (
	"context"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

func AddToSet(client *redis.Client, ctx context.Context, key string, members ...interface{}) error {
	if err := client.SAdd(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis sadd key %s", key)
	}
	return nil
}

func GetSetMembers(client *redis.Client, ctx context.Context, key string) ([]string, error) {
	members, err := client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis smembers key %s", key)
	}
	return members, nil
}

func RemoveFromSet(client *redis.Client, ctx context.Context, key string, members ...interface{}) error {
	if err := client.SRem(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis srem key %s", key)
	}
	return nil
}
