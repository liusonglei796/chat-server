// Package cache 提供缓存工具的单元测试
package cache

import (
	"testing"
	"time"
)

func TestRandomizedTTL(t *testing.T) {
	baseTTL := 10 * time.Minute

	for i := 0; i < 100; i++ {
		result := RandomizedTTL(baseTTL)
		// 结果应该在 baseTTL ± 10% 范围内
		minTTL := baseTTL - (baseTTL / 10)
		maxTTL := baseTTL + (baseTTL / 10)

		if result < minTTL || result > maxTTL {
			t.Errorf("RandomizedTTL(%v) = %v, want in range [%v, %v]", baseTTL, result, minTTL, maxTTL)
		}
	}
}

func TestRandomizedTTL_ZeroValue(t *testing.T) {
	result := RandomizedTTL(0)
	if result != 0 {
		t.Errorf("RandomizedTTL(0) = %v, want 0", result)
	}
}

func TestRandomizedTTL_SmallValue(t *testing.T) {
	// 非常小的 TTL，抖动范围可能为 0
	result := RandomizedTTL(1 * time.Nanosecond)
	if result != 1*time.Nanosecond {
		t.Errorf("RandomizedTTL(1ns) = %v, want 1ns", result)
	}
}

func TestTTLWithJitter(t *testing.T) {
	baseTTL := 1 * time.Hour
	jitterPercent := 20

	for i := 0; i < 100; i++ {
		result := TTLWithJitter(baseTTL, jitterPercent)
		minTTL := baseTTL - (baseTTL * time.Duration(jitterPercent) / 100)
		maxTTL := baseTTL + (baseTTL * time.Duration(jitterPercent) / 100)

		if result < minTTL || result > maxTTL {
			t.Errorf("TTLWithJitter(%v, %d) = %v, want in range [%v, %v]",
				baseTTL, jitterPercent, result, minTTL, maxTTL)
		}
	}
}

func TestTTLWithJitter_ZeroJitter(t *testing.T) {
	baseTTL := 1 * time.Hour
	result := TTLWithJitter(baseTTL, 0)
	if result != baseTTL {
		t.Errorf("TTLWithJitter with 0%% jitter should return baseTTL")
	}
}
