package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"kama_chat_server/internal/common/config"
	"kama_chat_server/internal/common/dao/redis/ops"
	"kama_chat_server/internal/common/domain/repository"

	"github.com/panjf2000/ants/v2"
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
	// ants 池：15 个 Worker，适用于多 Service 共享
	return NewRedisCache(client, 15)
}

// RedisCache Redis 缓存实现
// 该结构体同时实现了 CacheService（基础同步读写）和 AsyncCacheService（异步任务）两个接口。
type RedisCache struct {
	client *redis.Client
	pool   *ants.Pool // ants goroutine 池，负责异步任务调度
}

// NewRedisCache 创建 Redis 缓存实例
// poolSize: ants 池的 Worker 数量
func NewRedisCache(client *redis.Client, poolSize int) *RedisCache {
	// WithNonblocking: Submit 永不阻塞调用方，池满时返回 ErrPoolOverload，由调用方降级
	// WithPanicHandler: 任务 panic 时记录日志，不影响 Worker 复用
	pool, err := ants.NewPool(poolSize,
		ants.WithNonblocking(true),
		ants.WithPanicHandler(func(p any) {
			zap.L().Error("Redis cache task panic", zap.Any("panic", p))
		}),
	)
	if err != nil {
		zap.L().Fatal("create ants pool failed", zap.Error(err))
	}
	zap.L().Info("Redis cache ants pool started", zap.Int("workers", poolSize))
	return &RedisCache{client: client, pool: pool}
}

// 确保 RedisCache 实现了 AsyncCacheService 接口
var _ repository.AsyncCacheService = (*RedisCache)(nil)

// SetNX 设置键值对，仅当键不存在时生效（原子操作）
// 返回 bool 表示是否设置成功（true=新键，false=键已存在）
func (r *RedisCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return ops.SetNX(r.client, ctx, key, value, ttl)
}

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
	if err := r.pool.Submit(action); err != nil {
		if errors.Is(err, ants.ErrPoolOverload) {
			// 降级：同步执行，宁可慢也不丢
			zap.L().Warn("Redis cache pool overloaded, executing synchronously")
			action()
		}
		// ErrPoolClosed: 进程正在退出，任务丢弃
	}
}

// Release 优雅关闭池：等待已提交任务执行完毕后停止 Worker
// 应在进程收到退出信号时调用
func (r *RedisCache) Release() {
	r.pool.Release()
}

// WrapRedisError 包装 Redis 错误
func WrapRedisError(err error, format string, args ...any) error {
	return ops.WrapRedisError(err, format, args...)
}
