// Package ops 提供 Redis 命令的轻量封装（集合、字符串、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"context" // 标准库：用于向 Redis 调用传递超时、取消等上下文信号

	"kama_chat_server/pkg/errorx" // 项目业务错误工具：Wrapf 包装 + CodeCacheError 错误码

	"github.com/redis/go-redis/v9" // Redis Go 客户端 v9
)

// Delete 删除单个 Redis 键（如果存在）。
// 先查询是否存在，存在才执行删除，避免对不存在的 key 做无谓操作。
// 使用 UNLINK（异步、非阻塞）而非 DEL，防止大 key 阻塞服务器。
// 参数：
//   - client: Redis 客户端实例
//   - ctx:    上下文
//   - key:    要删除的 key
func Delete(client *redis.Client, ctx context.Context, key string) error {
	exists, err := client.Exists(ctx, key).Result() // EXISTS 命令：返回 0=不存在 / 1=存在
	if err != nil {                                  // 查询出错
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key) // 包装为缓存错误，附带 key 便于定位
	}
	if exists == 1 { // key 存在才删除
		if err := client.Unlink(ctx, key).Err(); err != nil { // UNLINK：异步删除，比 DEL 快、非阻塞
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink key %s", key) // 删除出错 → 缓存错误
		}
	}
	return nil // 存在且删除成功（或本来就不存在）都返回 nil
}

// DeleteByPattern 删除匹配模式的所有键（支持多个 pattern，批量删除）。
// 基于 SCAN 游标遍历（不入阻塞服务端，安全替代 KEYS），分批 UNLINK。
// 参数：
//   - client:   Redis 客户端
//   - ctx:      上下文
//   - patterns: 一个或多个通配符模式，如 "msg:*"
func DeleteByPattern(client *redis.Client, ctx context.Context, patterns ...string) error {
	if len(patterns) == 0 { // 没有提供任何模式
		return nil // 无操作直接返回
	}

	for _, pattern := range patterns { // 遍历每个待删除的模式
		var cursor uint64                 // SCAN 游标，从 0 开始
		for {                             // 循环直到游标归零（一轮 SCAN 遍历完成）
			var keys []string // 本次迭代找到的 key
			var err error
			keys, cursor, err = client.Scan(ctx, cursor, pattern, 500).Result() // SCAN 每次最多取 500 个匹配 key
			if err != nil {                                                     // 扫描出错
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern) // 包装为缓存错误
			}
			if len(keys) > 0 { // 本次迭代确有命中
				if err := client.Unlink(ctx, keys...).Err(); err != nil { // 批量 UNLINK 删除这批 key
					return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern) // 删除出错
				}
			}
			if cursor == 0 { // 游标归零表示该 pattern 已扫完
				break // 退出内层循环，处理下一个 pattern
			}
		}
	}
	return nil // 所有模式处理完毕，返回成功
}