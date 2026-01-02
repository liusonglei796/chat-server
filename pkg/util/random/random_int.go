package random

import (
	"crypto/rand"
	"math/big"
	"time"
)

// GetRandomInt 生成指定位数的安全随机数字（用于验证码）
func GetRandomInt(length int) int {
	// 计算范围：例如 length=6 时，范围是 100000-999999
	min := int64(1)
	for i := 1; i < length; i++ {
		min *= 10
	}
	max := min * 10

	// 生成 [min, max) 范围的随机数
	rangeSize := big.NewInt(max - min)
	n, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return int(min) // fallback
	}
	return int(n.Int64() + min)
}

// GetNowAndLenRandomString 生成带时间戳前缀的随机字符串（用于 UUID）
// 格式: YYMMDD + 字母数字混合
// 示例: 241230AbCdE1234567
func GetNowAndLenRandomString(length int) string {
	result := make([]byte, length)
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range result {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			result[i] = 'x'
			continue
		}
		result[i] = charset[n.Int64()]
	}
	return time.Now().Format("060102") + string(result)
}
