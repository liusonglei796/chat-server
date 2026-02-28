package redisinterface

import (
	"context"
	"time"
)

type CacheService interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	GetOrError(ctx context.Context, key string) (string, error)
	GetByPrefix(ctx context.Context, prefix string) (string, error)
	Delete(ctx context.Context, key string) error
	DeleteByPattern(ctx context.Context, patterns ...string) error
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	AddToSet(ctx context.Context, key string, members ...interface{}) error
	GetSetMembers(ctx context.Context, key string) ([]string, error)
	RemoveFromSet(ctx context.Context, key string, members ...interface{}) error
}

type AsyncCacheService interface {
	CacheService
	SubmitTask(action func())
}
