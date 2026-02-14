package errorx

import (
	"errors"

	"github.com/redis/go-redis/v9"
)

// WrapRedisError 包装 Redis 错误，统一处理 redis.Nil 和其他错误
//   - redis.Nil -> CodeNotFound
//   - 其他错误 -> CodeCacheError
//
// 用法：return errorx.WrapRedisError(err, "redis get key %s", key)
func WrapRedisError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return Wrapf(err, CodeNotFound, format+" not found", args...)
	}
	return Wrapf(err, CodeCacheError, format, args...)
}
