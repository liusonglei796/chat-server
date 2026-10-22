// Package ops 提供 Redis 命令的轻量封装（集合、字符串、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"context" // 标准库：用于向 Redis 调用传递超时、取消等上下文信号
	"time"    // 标准库：TTL 过期时间类型

	"kama_chat_server/pkg/errorx" // 项目业务错误工具：Wrapf 包装 + CodeCacheError 错误码

	"github.com/redis/go-redis/v9" // Redis Go 客户端 v9
)

// Incr 原子递增键的值（INCR 命令），返回递增后的新值。
// 典型用途：未读消息数、访问量、限流计数等。
// 注意：key 不存在时 INCR 会先创建为 0 再递增为 1，返回 1。
func Incr(client *redis.Client, ctx context.Context, key string) (int64, error) {
	val, err := client.Incr(ctx, key).Result() // INCR key：原子自增 1，返回自增后的值
	if err != nil {                            // 递增出错
		return 0, errorx.Wrapf(err, errorx.CodeCacheError, "redis incr key %s", key) // 返回 0 + 缓存错误
	}
	return val, nil // 成功返回自增后的值
}

// Expire 设置键的过期时间（TTL）。
// 与 Incr 配合的常见模式：先 Incr 计数，再 Expire 设窗口，实现"每周期计数"。
func Expire(client *redis.Client, ctx context.Context, key string, ttl time.Duration) error {
	if err := client.Expire(ctx, key, ttl).Err(); err != nil { // EXPIRE key seconds：设置存活时间
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis expire key %s", key) // 出错 → 缓存错误
	}
	return nil // 成功
}