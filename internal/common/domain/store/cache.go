package store

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
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	AddToSet(ctx context.Context, key string, members ...interface{}) error
	GetSetMembers(ctx context.Context, key string) ([]string, error)
	RemoveFromSet(ctx context.Context, key string, members ...interface{}) error
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRangeByScore(ctx context.Context, key string, max, min string, offset, count int64) ([]string, error)
	ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error
	ZRem(ctx context.Context, key string, members ...interface{}) error
	ZCard(ctx context.Context, key string) (int64, error)
	ZScore(ctx context.Context, key string, member string) (float64, error)
}

type AsyncCacheService interface {
	CacheService
	SubmitTask(action func())
}
