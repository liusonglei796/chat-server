// Package cache 提供缓存相关的工具函数
package cache

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// RandomizedTTL 返回带随机偏移的过期时间，用于防止缓存雪崩
// baseTTL: 基础过期时间
// 返回值: baseTTL ± 10% 的随机过期时间
func RandomizedTTL(baseTTL time.Duration) time.Duration {
	if baseTTL <= 0 {
		return baseTTL
	}
	// 计算 ±10% 的抖动范围
	jitterRange := int64(baseTTL) / 10
	if jitterRange == 0 {
		return baseTTL
	}
	// 生成 -jitterRange 到 +jitterRange 之间的随机偏移
	jitter := time.Duration(rand.Int63n(jitterRange*2) - jitterRange)
	return baseTTL + jitter
}

// TTLWithJitter 返回带自定义抖动百分比的过期时间
// baseTTL: 基础过期时间
// jitterPercent: 抖动百分比 (例如 10 表示 ±10%)
func TTLWithJitter(baseTTL time.Duration, jitterPercent int) time.Duration {
	if baseTTL <= 0 || jitterPercent <= 0 {
		return baseTTL
	}
	jitterRange := int64(baseTTL) * int64(jitterPercent) / 100
	if jitterRange == 0 {
		return baseTTL
	}
	jitter := time.Duration(rand.Int63n(jitterRange*2) - jitterRange)
	return baseTTL + jitter
}
