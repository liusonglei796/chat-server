package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"kama_chat_server/internal/config"
	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ==================== 接口定义 ====================

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
	// DeleteByPattern 删除匹配模式的所有键（支持批量模式）
	DeleteByPattern(ctx context.Context, patterns ...string) error

	// ==================== 计数器操作 ====================

	// Incr 原子递增，返回递增后的值
	Incr(ctx context.Context, key string) (int64, error)
	// Expire 设置键过期时间
	Expire(ctx context.Context, key string, ttl time.Duration) error

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

// ==================== 初始化逻辑 ====================

// Init 初始化 Redis 连接并返回缓存服务
// 从配置文件读取连接参数并创建客户端实例
// 返回 AsyncCacheService 接口，供 Service 层依赖注入使用
func Init() AsyncCacheService {
	conf := config.GetConfig()
	host := conf.RedisConfig.Host         // Redis 服务器地址
	port := conf.RedisConfig.Port         // Redis 端口
	password := conf.RedisConfig.Password // 密码，无密码留空
	db := conf.RedisConfig.Db             // 数据库编号

	// 拼接地址：host:port
	addr := host + ":" + strconv.Itoa(port)

	// 创建 Redis 客户端
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		// 连接池配置
		PoolSize:     50, // 最大连接数
		MinIdleConns: 15, // 最小空闲连接，与 Worker 数量匹配
	})

	// 创建并返回缓存服务实例
	// 启动 15 个 Worker，缓冲区大小 3000，适用于多 Service 共享
	return NewRedisCache(client, 15, 3000)
}

// ==================== 实现逻辑 ====================

// RedisCache Redis 缓存实现
// 该结构体同时实现了 CacheService（基础同步读写）和 AsyncCacheService（异步任务）两个接口。
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
		return "", WrapRedisError(err, "redis get key %s", key)
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

// DeleteByPattern 删除匹配模式的所有键（支持批量）
func (r *RedisCache) DeleteByPattern(ctx context.Context, patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}

	for _, pattern := range patterns {
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
	}
	return nil
}

// ==================== 计数器操作 ====================

// Incr 原子递增键的值，返回递增后的值
func (r *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, errorx.Wrapf(err, errorx.CodeCacheError, "redis incr key %s", key)
	}
	return val, nil
}

// Expire 设置键过期时间
func (r *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := r.client.Expire(ctx, key, ttl).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis expire key %s", key)
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

// WrapRedisError 包装 Redis 错误，统一处理 redis.Nil 和其他错误
//   - redis.Nil -> CodeNotFound
//   - 其他错误 -> CodeCacheError
//
// 用法：return WrapRedisError(err, "redis get key %s", key)
func WrapRedisError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format+" not found", args...)
	}
	return errorx.Wrapf(err, errorx.CodeCacheError, format, args...)
}
