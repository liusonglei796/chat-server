// Package ops 提供 Redis 命令的轻量封装（集合、字符串、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"errors" // 标准库：errors.Is 用于在错误链中做哨兵错误判断

	"kama_chat_server/pkg/errorx" // 项目业务错误工具：提供 Wrapf 包装与 CodeNotFound/CodeCacheError 错误码

	"github.com/redis/go-redis/v9" // Redis Go 客户端 v9（redis.Nil 表示 key 不存在）
)

// WrapRedisError 把 Redis 命令返回的错误包装为业务错误。
// 关键区分：key 不存在（redis.Nil）包装为 CodeNotFound；其余真实错误包装为 CodeCacheError。
// 参数：
//   - err:    原始错误（可为 nil）
//   - format: 日志消息模板（含 %s 等占位）
//   - args:   模板占位参数
func WrapRedisError(err error, format string, args ...any) error {
	if err == nil { // 无错误
		return nil // 直接返回 nil，调用方走成功分支
	}
	if errors.Is(err, redis.Nil) { // errors.Is 穿透包装链，判断是否为 key 不存在哨兵错误
		// key 不存在：包装为 CodeNotFound，并把模板后追加 " not found" 便于日志识别
		return errorx.Wrapf(err, errorx.CodeNotFound, format+" not found", args...)
	}
	// 其它（网络、连接、类型错误等）：包装为 CodeCacheError（缓存异常），保留原始错误链
	return errorx.Wrapf(err, errorx.CodeCacheError, format, args...)
}