// Package redis 定义缓存服务接口
// 遵循依赖倒置原则，Service 层依赖此接口而非具体 Redis 实现
package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheService 缓存服务接口
// 抽象缓存操作，支持 Redis、Memcached、本地缓存等多种实现
type CacheService interface {
	// ==================== String 操作 ====================

	// Set 设置键值对并指定过期时间
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	// Get 获取键对应的值（键不存在返回空字符串和 nil）
	Get(ctx context.Context, key string) (string, error)
	// GetOrError 获取键对应的值（键不存在返回错误）
	GetOrError(ctx context.Context, key string) (string, error)
	// GetByPrefix 通过前缀查找唯一键的值
	GetByPrefix(ctx context.Context, prefix string) (string, error)

	// ==================== Key 操作 ====================

	// Delete 删除键（如果存在）
	Delete(ctx context.Context, key string) error
	// DeleteByPattern 删除匹配模式的所有键
	DeleteByPattern(ctx context.Context, pattern string) error
	// DeleteByPatterns 批量删除多个模式匹配的键
	DeleteByPatterns(ctx context.Context, patterns []string) error

	// ==================== Set 集合操作 ====================

	// AddToSet 向集合添加成员
	AddToSet(ctx context.Context, key string, members ...interface{}) error
	// GetSetMembers 获取集合中的所有成员
	GetSetMembers(ctx context.Context, key string) ([]string, error)
	// RemoveFromSet 从集合中移除成员
	RemoveFromSet(ctx context.Context, key string, members ...interface{}) error
}

// AsyncCacheService 异步缓存服务接口
// 提供异步任务提交能力，用于非阻塞缓存更新
type AsyncCacheService interface {
	CacheService
	// SubmitTask 提交异步缓存任务
	SubmitTask(action func())
}

// NewRedisCache 创建 Redis 缓存实例
// 将构造函数与接口定义放在同一文件中，便于统一查看依赖入口。
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
