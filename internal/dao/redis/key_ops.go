// Package redis 提供 Redis 缓存操作的封装
// 本文件包含键的删除操作

package redis

import (
	"context"
	"kama_chat_server/pkg/errorx"
)

// ==================== 删除操作 ====================

// DelKeyIfExists 删除键（如果存在）
// 先检查键是否存在，存在则删除
// key: 键名
// 返回: 操作错误
func DelKeyIfExists(ctx context.Context, key string) error {
	// 检查键是否存在
	exists, err := redisClient.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 { // 键存在
		if err := redisClient.Unlink(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink key %s", key)
		}
	}
	// 无论键是否存在，都返回成功
	return nil
}

// DelKeysWithPattern 删除匹配模式的所有键
// 使用 SCAN 分批扫描 + UNLINK 异步删除，避免阻塞 Redis
// pattern: 匹配模式，如 "user_*"
// 返回: 操作错误
func DelKeysWithPattern(ctx context.Context, pattern string) error {
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
func DelKeysWithPatterns(ctx context.Context, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}

	// 遍历每一个 pattern
	for _, pattern := range patterns {
		var cursor uint64
		for {
			// 分批扫描：每次扫描 500 个键
			keys, cursor, err := redisClient.Scan(ctx, cursor, pattern, 500).Result()
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
			if cursor == 0 {
				break
			}
		}
	}

	return nil
}
// DeleteAllRedisKeys 删除当前数据库中的所有键
// 警告：此操作会清空整个数据库，谨慎使用
// 通常用于服务器关闭时的清理操作
// 返回: 操作错误
func DeleteAllRedisKeys(ctx context.Context) error {
	var cursor uint64 = 0
	for {
		// 扫描所有键
		keys, cursor, err := redisClient.Scan(ctx, cursor, "*", 0).Result()
		if err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "redis scan all keys")
		}

		if len(keys) > 0 {
			// 批量删除找到的键
			if _, err := redisClient.Unlink(ctx, keys...).Result(); err != nil {
				return errorx.Wrap(err, errorx.CodeCacheError, "redis unlink all keys")
			}
		}

		// cursor 为 0 表示扫描完成
		if cursor == 0 {
			break
		}
	}
	return nil
}
