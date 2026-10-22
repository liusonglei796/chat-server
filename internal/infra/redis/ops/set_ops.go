// Package ops 提供 Redis 命令的轻量封装（集合、字符串、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"context" // 标准库：用于向 Redis 调用传递超时、取消等上下文信号

	"kama_chat_server/pkg/errorx" // 项目业务错误工具：Wrapf 包装 + CodeCacheError 错误码

	"github.com/redis/go-redis/v9" // Redis Go 客户端 v9
)

// AddToSet 向 Redis Set（集合）添加一个或多个成员（SADD）。
// 集合用于去重性数据（如好友 ID 列表、在线用户集合）。
// 参数：
//   - client:  Redis 客户端
//   - ctx:     上下文
//   - key:     集合 key
//   - members: 可变参数，要加入的成员（1 个或多个）
func AddToSet(client *redis.Client, ctx context.Context, key string, members ...interface{}) error {
	if err := client.SAdd(ctx, key, members...).Err(); err != nil { // SADD key member [member ...]
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis sadd key %s", key) // 出错 → 缓存错误，附 key
	}
	return nil // 成功
}

// GetSetMembers 获取集合的全部成员（SMEMBERS），返回成员切片。
// 空集合返回空切片且不报错（Set 类型无 key 不存在一说，SMEMBERS 返回空结果）。
func GetSetMembers(client *redis.Client, ctx context.Context, key string) ([]string, error) {
	members, err := client.SMembers(ctx, key).Result() // SMEMBERS key：返回全部成员
	if err != nil {                                    // 出错
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis smembers key %s", key) // nil + 缓存错误
	}
	return members, nil // 成功返回成员列表
}

// RemoveFromSet 从集合中移除一个或多个成员（SREM）。
func RemoveFromSet(client *redis.Client, ctx context.Context, key string, members ...interface{}) error {
	if err := client.SRem(ctx, key, members...).Err(); err != nil { // SREM key member [member ...]
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis srem key %s", key) // 出错 → 缓存错误
	}
	return nil // 成功
}