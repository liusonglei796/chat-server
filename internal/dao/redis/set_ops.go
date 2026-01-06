// Package redis 提供 Redis 缓存操作的封装
// 本文件包含 Set（集合）类型的操作

package redis

import (
	"kama_chat_server/pkg/errorx"
)

// ==================== Set 集合操作 ====================

// SAdd 向集合添加一个或多个成员
// Redis 命令: SADD key member [member ...]
// 集合特性：成员唯一，重复添加不会报错但不会增加成员
// key: 集合键名
// members: 要添加的成员（可变参数）
// 返回: 操作错误
func SAdd(key string, members ...interface{}) error {
	if err := redisClient.SAdd(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis sadd key %s", key)
	}
	return nil
}

// SMembers 获取集合中的所有成员
// Redis 命令: SMEMBERS key
// key: 集合键名
// 返回: 成员字符串切片和错误
func SMembers(key string) ([]string, error) {
	members, err := redisClient.SMembers(ctx, key).Result()
	if err != nil {
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis smembers key %s", key)
	}
	return members, nil
}

// SIsMember 判断成员是否存在于集合中
// Redis 命令: SISMEMBER key member
// key: 集合键名
// member: 要检查的成员
// 返回: 是否存在（bool）和错误
func SIsMember(key string, member interface{}) (bool, error) {
	isMember, err := redisClient.SIsMember(ctx, key, member).Result()
	if err != nil {
		return false, errorx.Wrapf(err, errorx.CodeCacheError, "redis sismember key %s", key)
	}
	return isMember, nil
}
