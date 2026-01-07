// Package redis 提供 Redis 缓存操作的封装
// 包含 String 类型的基础操作以及模式匹配删除等高级功能
// 使用 github.com/redis/go-redis/v9 作为底层客户端
package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"kama_chat_server/internal/config"
	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

// redisClient 全局 Redis 客户端实例
var redisClient *redis.Client

// ctx 全局上下文，用于 Redis 操作
var ctx = context.Background()

// Init 初始化 Redis 连接
// 从配置文件读取连接参数并创建客户端实例
func Init() {
	conf := config.GetConfig()
	host := conf.RedisConfig.Host         // Redis 服务器地址
	port := conf.RedisConfig.Port         // Redis 端口
	password := conf.RedisConfig.Password // 密码，无密码留空
	db := conf.Db                         // 数据库编号

	// 拼接地址：host:port
	addr := host + ":" + strconv.Itoa(port)

	// 创建 Redis 客户端
	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// 连接池配置
		PoolSize:     50, // 最大连接数
		MinIdleConns: 15, // 最小空闲连接，与 Worker 数量匹配
	})

	// 初始化缓存更新 Worker Pool
	// 启动 15 个 Worker，缓冲区大小 3000，适用于多 Service 共享
	InitCacheWorker(15, 3000)
}

// ==================== 基础 String 操作 ====================

// SetKeyEx 设置键值对并指定过期时间
// key: 键名
// value: 值
// timeout: 过期时间
// 返回: 操作错误（已包装）
func SetKeyEx(key string, value string, timeout time.Duration) error {
	if err := redisClient.Set(ctx, key, value, timeout).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

// GetKey 获取键对应的值
// 如果键不存在，返回空字符串和 nil（不视为错误）
// key: 键名
// 返回: 值和错误
func GetKey(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil // 键不存在，返回空但不报错
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

// GetKeyNilIsErr 获取键对应的值（键不存在视为错误）
// 与 GetKey 的区别：如果键不存在，返回 CodeNotFound 错误
// key: 键名
// 返回: 值和错误
func GetKeyNilIsErr(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", errorx.Wrapf(err, errorx.CodeNotFound, "redis key %s not found", key)
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

// ==================== 模式匹配查询 ====================

// GetKeyWithPrefixNilIsErr 通过前缀查找唯一键
// 使用 SCAN 命令遍历，避免阻塞 Redis
// prefix: 键前缀
// 返回: 匹配的键名（期望唯一）和错误
// 注意: 如果找到多个键会返回错误
func GetKeyWithPrefixNilIsErr(prefix string) (string, error) {
	var cursor uint64      // 游标，用于分批扫描
	var foundKeys []string // 收集找到的键

	for {
		var keys []string
		var err error

		// 使用 SCAN 命令分批扫描，每次最多返回 100 个
		// 相比 KEYS 命令，SCAN 不会阻塞 Redis
		keys, cursor, err = redisClient.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}

		foundKeys = append(foundKeys, keys...)

		// 如果找到超过1个键，直接报错（期望唯一）
		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan prefix %s: found %d keys, expected 1", prefix, len(foundKeys))
		}

		// cursor 为 0 表示扫描完成
		if cursor == 0 {
			break
		}
	}

	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis prefix %s not found", prefix)
	}

	return foundKeys[0], nil
}

// GetKeyWithSuffixNilIsErr 通过后缀查找唯一键
// 逻辑同 GetKeyWithPrefixNilIsErr，使用 *suffix 模式
func GetKeyWithSuffixNilIsErr(suffix string) (string, error) {
	var cursor uint64
	var foundKeys []string

	for {
		var keys []string
		var err error

		// 使用 *suffix 模式匹配后缀
		keys, cursor, err = redisClient.Scan(ctx, cursor, "*"+suffix, 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan suffix %s", suffix)
		}

		foundKeys = append(foundKeys, keys...)

		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan suffix %s: found %d keys, expected 1", suffix, len(foundKeys))
		}

		if cursor == 0 {
			break
		}
	}

	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis suffix %s not found", suffix)
	}

	return foundKeys[0], nil
}

// ==================== 删除操作 ====================

// DelKeyIfExists 删除键（如果存在）
// 先检查键是否存在，存在则删除
// key: 键名
// 返回: 操作错误
func DelKeyIfExists(key string) error {
	// 检查键是否存在
	exists, err := redisClient.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 { // 键存在
		if err := redisClient.Del(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete key %s", key)
		}
	}
	// 无论键是否存在，都返回成功
	return nil
}

// DelKeysWithPattern 删除匹配模式的所有键
// 使用 SCAN 分批扫描 + UNLINK 异步删除，避免阻塞 Redis
// pattern: 匹配模式，如 "user_*"
// 返回: 操作错误
func DelKeysWithPattern(pattern string) error {
	var cursor uint64
	for {
		var keys []string
		var err error

		// 每次扫描 500 条，减少循环次数
		keys, cursor, err = redisClient.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
		}

		if len(keys) > 0 {
			// 使用 UNLINK 而非 DEL，实现非阻塞异步删除
			// UNLINK 会在后台线程释放内存，不阻塞主线程
			if err := redisClient.Unlink(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

// DelKeysWithPatterns 批量删除多个模式匹配的键
// patterns: 模式数组，如 ["user_*", "session_*"]
// 返回: 操作错误
func DelKeysWithPatterns(patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}

	// 遍历每一个 pattern
	for _, pattern := range patterns {
		var cursor uint64
		for {
			// 分批扫描：每次扫描 500 个键
			keys, newCursor, err := redisClient.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
			}

			// 立即删除：扫到一批，就删一批
			if len(keys) > 0 {
				// 使用 UNLINK 替代 DEL，实现异步删除
				if err := redisClient.Unlink(ctx, keys...).Err(); err != nil {
					return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
				}
			}

			cursor = newCursor
			if cursor == 0 {
				break
			}
		}
	}

	return nil
}

// DelKeysWithPrefix 删除指定前缀的所有键
// prefix: 键前缀
// 返回: 操作错误
func DelKeysWithPrefix(prefix string) error {
	var cursor uint64
	for {
		var keys []string
		var err error

		// 使用 prefix* 模式匹配
		keys, cursor, err = redisClient.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}

		if len(keys) > 0 {
			if err := redisClient.Del(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete keys with prefix %s", prefix)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

// DelKeysWithSuffix 删除指定后缀的所有键
// suffix: 键后缀
// 返回: 操作错误
func DelKeysWithSuffix(suffix string) error {
	var cursor uint64
	for {
		var keys []string
		var err error

		// 使用 *suffix 模式匹配
		keys, cursor, err = redisClient.Scan(ctx, cursor, "*"+suffix, 100).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan suffix %s", suffix)
		}

		if len(keys) > 0 {
			if err := redisClient.Del(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete keys with suffix %s", suffix)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

// DeleteAllRedisKeys 删除当前数据库中的所有键
// 警告：此操作会清空整个数据库，谨慎使用
// 通常用于服务器关闭时的清理操作
// 返回: 操作错误
func DeleteAllRedisKeys() error {
	var cursor uint64 = 0
	for {
		// 扫描所有键
		keys, nextCursor, err := redisClient.Scan(ctx, cursor, "*", 0).Result()
		if err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "redis scan all keys")
		}
		cursor = nextCursor

		if len(keys) > 0 {
			// 批量删除找到的键
			if _, err := redisClient.Del(ctx, keys...).Result(); err != nil {
				return errorx.Wrap(err, errorx.CodeCacheError, "redis delete all keys")
			}
		}

		// cursor 为 0 表示扫描完成
		if cursor == 0 {
			break
		}
	}
	return nil
}
