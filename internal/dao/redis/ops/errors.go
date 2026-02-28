package ops

import (
	"errors"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

func WrapRedisError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format+" not found", args...)
	}
	return errorx.Wrapf(err, errorx.CodeCacheError, format, args...)
}
