// Package cache 提供缓存相关的工具函数
package cache

import (
	"math/rand"
	"time"
)

// RandomizedTTL 返回带随机偏移的过期时间，用于防止缓存雪崩
func RandomizedTTL(baseTTL time.Duration) time.Duration {
	return TTLWithJitter(baseTTL, 10)
}

// TTLWithJitter 返回带随机抖动的过期时间
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
