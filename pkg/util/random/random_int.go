// Package random 提供安全随机相关工具
package random

import (
	"crypto/rand" // 安全随机源
	"math/big"    // 大整数运算
	// 时间处理
)

// GetRandomInt 生成指定位数的安全随机数字（用于验证码）
func GetRandomInt(length int) int {
	// 计算范围：例如 length=6 时，范围是 100000-999999
	min := int64(1)               // 最小值起点
	for i := 1; i < length; i++ { // 按位扩展
		min *= 10 // 每次扩大 10 倍
	}
	max := min * 10 // 最大值（不含上界）

	// 生成 [min, max) 范围的随机数
	rangeSize := big.NewInt(max - min)         // 计算区间大小
	n, err := rand.Int(rand.Reader, rangeSize) // 生成安全随机数
	if err != nil {                            // 出错时兜底
		return int(min) // fallback
	}
	return int(n.Int64() + min) // 平移到 [min, max)
}
