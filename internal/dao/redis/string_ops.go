// Package redis 提供 Redis 缓存操作的封装
// 本文件包含 String 类型的基础操作

package redis

import (
	"context"
	"errors"
	"time"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

// ==================== 基础 String 操作 ====================

// SetKeyEx 设置键值对并指定过期时间
// key: 键名
// value: 值
// timeout: 过期时间
// 返回: 操作错误（已包装）
func SetKeyEx(ctx context.Context, key string, value string, timeout time.Duration) error {
	if err := redisClient.Set(ctx, key, value, timeout).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

// GetKey 获取键对应的值
// 如果键不存在，返回空字符串和 nil（不视为错误）
// key: 键名
// 返回: 值和错误
func GetKey(ctx context.Context, key string) (string, error) {
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
func GetKeyNilIsErr(ctx context.Context, key string) (string, error) {
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
func GetKeyWithPrefixNilIsErr(ctx context.Context, prefix string) (string, error) {
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


