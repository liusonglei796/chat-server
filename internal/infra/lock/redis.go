package lock

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Distributed 分布式锁统一接口
type Distributed interface {
	Acquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, token string) error
}

// Redis 基于 Redis SETNX + Lua 脚本安全释放的分布式锁实现
type Redis struct {
	client redis.UniversalClient
}

// NewRedis 创建 Redis 分布式锁
func NewRedis(client redis.UniversalClient) Distributed {
	return &Redis{client: client}
}

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

func (r *Redis) Acquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if r.client == nil {
		return false, errors.New("redis client is nil")
	}
	return r.client.SetNX(ctx, key, token, ttl).Result()
}

func (r *Redis) Release(ctx context.Context, key, token string) error {
	if r.client == nil {
		return errors.New("redis client is nil")
	}
	_, err := releaseScript.Run(ctx, r.client, []string{key}, token).Result()
	return err
}
