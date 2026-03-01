package redis

import (
	"context"
	"strconv"
	"time"

	"kama_chat_server/internal/config"
	"kama_chat_server/internal/dao/redis/ops"
	"kama_chat_server/internal/domain/repository"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ==================== 初始化逻辑 ====================

// Init 初始化 Redis 连接并返回缓存服务
// 从配置文件读取连接参数并创建客户端实例
// 返回 AsyncCacheService 接口，供 Service 层依赖注入使用
func Init() repository.AsyncCacheService {
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

// 确保 RedisCache 实现了 AsyncCacheService 接口
var _ repository.AsyncCacheService = (*RedisCache)(nil)

// Set 设置键值对并指定过期时间
func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return ops.Set(r.client, ctx, key, value, ttl)
}

// Get 获取键对应的值（键不存在返回空字符串和 nil）
func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return ops.Get(r.client, ctx, key)
}

// GetOrError 获取键对应的值（键不存在返回错误）
func (r *RedisCache) GetOrError(ctx context.Context, key string) (string, error) {
	return ops.GetOrError(r.client, ctx, key)
}

// GetByPrefix 通过前缀查找唯一键的值
func (r *RedisCache) GetByPrefix(ctx context.Context, prefix string) (string, error) {
	return ops.GetByPrefix(r.client, ctx, prefix)
}

// Delete 删除键（如果存在）
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return ops.Delete(r.client, ctx, key)
}

// DeleteByPattern 删除匹配模式的所有键（支持批量）
func (r *RedisCache) DeleteByPattern(ctx context.Context, patterns ...string) error {
	return ops.DeleteByPattern(r.client, ctx, patterns...)
}

// Incr 原子递增键的值，返回递增后的值
func (r *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	return ops.Incr(r.client, ctx, key)
}

// Expire 设置键过期时间
func (r *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return ops.Expire(r.client, ctx, key, ttl)
}

// AddToSet 向集合添加成员
func (r *RedisCache) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	return ops.AddToSet(r.client, ctx, key, members...)
}

// GetSetMembers 获取集合中的所有成员
func (r *RedisCache) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	return ops.GetSetMembers(r.client, ctx, key)
}

// RemoveFromSet 从集合中移除成员
func (r *RedisCache) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	return ops.RemoveFromSet(r.client, ctx, key, members...)
}

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

// WrapRedisError 包装 Redis 错误
func WrapRedisError(err error, format string, args ...any) error {
	return ops.WrapRedisError(err, format, args...)
}
