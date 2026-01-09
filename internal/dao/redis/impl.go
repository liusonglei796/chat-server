// Package redis 提供 CacheService 接口的 Redis 实现
package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"kama_chat_server/pkg/errorx"
)

// RedisCache Redis 缓存实现
// 该结构体同时实现了 CacheService（基础同步读写）和 AsyncCacheService（异步任务）两个接口。
// 这种设计允许不同模块根据需求声明依赖最小的接口：
// 1. SmsService 只需要 CacheService，因此它无法访问 SubmitTask 方法，保证了安全性。
// 2. ChatServer 需要异步队列，因此它依赖 AsyncCacheService。
// 从而实现了“同一个实现类，不同的视图限制（接口隔离）”。
type RedisCache struct {
	client       *redis.Client
	taskChan     chan func()
	workerNum    int
	taskChanSize int
}

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(client *redis.Client, workerNum, taskChanSize int) *RedisCache {
	rc := &RedisCache{
		client:       client,
		taskChan:     make(chan func(), taskChanSize),
		workerNum:    workerNum,
		taskChanSize: taskChanSize,
	}
	// 启动 Worker Pool
	for i := 0; i < workerNum; i++ {
		go rc.startWorker()
	}
	zap.L().Info("Redis Cache Workers started", zap.Int("workers", workerNum), zap.Int("buffer", taskChanSize))
	return rc
}

// startWorker 启动单个 Worker 消费循环
func (r *RedisCache) startWorker() {
	defer func() {
		if rec := recover(); rec != nil {
			zap.L().Error("Redis Worker panic", zap.Any("recover", rec))
			go r.startWorker() // 重启
		}
	}()

	for task := range r.taskChan {
		if task != nil {
			task()
		}
	}
}

// ==================== String 操作 ====================

// Set 设置键值对并指定过期时间
func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

// Get 获取键对应的值（键不存在返回空字符串和 nil）
func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

// GetOrError 获取键对应的值（键不存在返回错误）
func (r *RedisCache) GetOrError(ctx context.Context, key string) (string, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", errorx.Wrapf(err, errorx.CodeNotFound, "redis key %s not found", key)
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

// GetByPrefix 通过前缀查找唯一键的值
func (r *RedisCache) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	var cursor uint64
	var foundKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, prefix+"*", 100).Result()
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

// ==================== Key 操作 ====================

// Delete 删除键（如果存在）
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 {
		if err := r.client.Unlink(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink key %s", key)
		}
	}
	return nil
}

// DeleteByPattern 删除匹配模式的所有键
func (r *RedisCache) DeleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
		}
		if len(keys) > 0 {
			if err := r.client.Unlink(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
			}
		}
		if cursor == 0 {
			break
		}
	}
	return nil
}

// DeleteByPatterns 批量删除多个模式匹配的键
func (r *RedisCache) DeleteByPatterns(ctx context.Context, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	for _, pattern := range patterns {
		if err := r.DeleteByPattern(ctx, pattern); err != nil {
			return err
		}
	}
	return nil
}

// ==================== Set 集合操作 ====================

// AddToSet 向集合添加成员
func (r *RedisCache) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	if err := r.client.SAdd(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis sadd key %s", key)
	}
	return nil
}

// GetSetMembers 获取集合中的所有成员
func (r *RedisCache) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	members, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis smembers key %s", key)
	}
	return members, nil
}

// RemoveFromSet 从集合中移除成员
func (r *RedisCache) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	if err := r.client.SRem(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis srem key %s", key)
	}
	return nil
}

// ==================== 异步任务 ====================

// SubmitTask 提交异步缓存任务
func (r *RedisCache) SubmitTask(action func()) {
	select {
	case r.taskChan <- action:
		// 成功放入
	default:
		// 降级：同步执行
		zap.L().Warn("Redis cache task channel full, executing synchronously")
		action()
	}
}

// 确保 RedisCache 实现了 AsyncCacheService 接口
var _ AsyncCacheService = (*RedisCache)(nil)
