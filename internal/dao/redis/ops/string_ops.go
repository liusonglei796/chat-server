package ops

import (
	"context"
	"errors"
	"time"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

func Set(client *redis.Client, ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := client.Set(ctx, key, value, ttl).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

func SetNX(client *redis.Client, ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	ok, err := client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, errorx.Wrapf(err, errorx.CodeCacheError, "redis setnx key %s", key)
	}
	return ok, nil
}

func Get(client *redis.Client, ctx context.Context, key string) (string, error) {
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

func GetOrError(client *redis.Client, ctx context.Context, key string) (string, error) {
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		return "", WrapRedisError(err, "redis get key %s", key)
	}
	return value, nil
}

func GetByPrefix(client *redis.Client, ctx context.Context, prefix string) (string, error) {
	var cursor uint64
	var foundKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}
		foundKeys = append(foundKeys, keys...)
		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan prefix %s: found %d keys, expected 1", prefix, len(foundKeys))
		}
		if cursor == 0 {
			break
		}
	}
	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis prefix %s not found", prefix)
	}
	return foundKeys[0], nil
}
