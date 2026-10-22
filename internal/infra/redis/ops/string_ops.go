// Package ops 提供 Redis 命令的轻量封装（集合、字符串、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"context" // 标准库：用于向 Redis 调用传递超时、取消等上下文信号
	"errors"  // 标准库：errors.Is 判断哨兵错误（redis.Nil）
	"time"    // 标准库：TTL 过期时间类型

	"kama_chat_server/pkg/errorx" // 项目业务错误工具：Wrapf/Newf + CodeNotFound/CodeCacheError 错误码

	"github.com/redis/go-redis/v9" // Redis Go 客户端 v9（redis.Nil = key 不存在）
)

// Set 设置字符串键值并指定过期时间（SET key value EX ttl）。
func Set(client *redis.Client, ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := client.Set(ctx, key, value, ttl).Err(); err != nil { // SET 命令
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key) // 出错 → 缓存错误
	}
	return nil // 成功
}

// SetNX 仅当键不存在时设置键值（SETNX，原子操作），返回是否设置成功。
// true=键原本不存在、本次写入成功；false=键已存在、未写入。
// 典型用途：幂等去重（如消息 client_msg_id 去重）、分布式锁的抢占。
func SetNX(client *redis.Client, ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	ok, err := client.SetNX(ctx, key, value, ttl).Result() // SETNX key value：原子"不存在才写"
	if err != nil {                                        // 出错
		return false, errorx.Wrapf(err, errorx.CodeCacheError, "redis setnx key %s", key) // false + 缓存错误
	}
	return ok, nil // 成功：返回是否写入
}

// Get 获取键值。**key 不存在时返回 ("", nil)**——缓存读场景下 miss 不算错误。
func Get(client *redis.Client, ctx context.Context, key string) (string, error) {
	value, err := client.Get(ctx, key).Result() // GET key
	if err != nil {
		if errors.Is(err, redis.Nil) { // redis.Nil：key 不存在
			return "", nil // 按"无缓存"处理：空字符串 + nil，不算错误
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key) // 真实错误 → 缓存错误
	}
	return value, nil // 成功返回值
}

// GetOrError 获取键值。**key 不存在时返回错误（CodeNotFound）**——适合"必须存在"的强制读。
// 与 Get 的区别：Get 把 miss 当正常，GetOrError 把 miss 当异常。
func GetOrError(client *redis.Client, ctx context.Context, key string) (string, error) {
	value, err := client.Get(ctx, key).Result() // GET key
	if err != nil {
		return "", WrapRedisError(err, "redis get key %s", key) // 统一包装：Nil→NotFound，其它→CacheError
	}
	return value, nil // 成功返回值
}

// GetByPrefix 按前缀查找**唯一** key（返回 key 名，而非值）。
// 基于 SCAN 游标扫描（安全替代 KEYS），要求前缀下最多只能有 1 个 key：
//   - 找到 0 个 → CodeNotFound
//   - 找到 >1 个 → 报错（违背"唯一"契约）
//   - 正好 1 个 → 返回该 key 名
func GetByPrefix(client *redis.Client, ctx context.Context, prefix string) (string, error) {
	var cursor uint64        // SCAN 游标
	var foundKeys []string   // 累计命中的 key

	for { // 游标循环直到扫完
		var keys []string // 本次迭代命中的 key
		var err error
		keys, cursor, err = client.Scan(ctx, cursor, prefix+"*", 100).Result() // SCAN cursor MATCH prefix* COUNT 100
		if err != nil {                                                        // 扫描出错
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix) // 缓存错误
		}
		foundKeys = append(foundKeys, keys...) // 累积本次结果
		if len(foundKeys) > 1 {                // 已超过 1 个 → 违反唯一性
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan prefix %s: found %d keys, expected 1", prefix, len(foundKeys))
		}
		if cursor == 0 { // 游标归零 → 扫描完成
			break // 退出循环
		}
	}
	if len(foundKeys) == 0 { // 一个都没匹配到
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis prefix %s not found", prefix) // 包装为 NotFound
	}
	return foundKeys[0], nil // 返回唯一匹配的 key 名
}