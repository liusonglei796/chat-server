package redis

import (
	"kama_chat_server/pkg/errorx"
)

// SAdd 向集合添加成员
func SAdd(key string, members ...interface{}) error {
	if err := redisClient.SAdd(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis sadd key %s", key)
	}
	return nil
}

// SMembers 获取集合所有成员
func SMembers(key string) ([]string, error) {
	members, err := redisClient.SMembers(ctx, key).Result()
	if err != nil {
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis smembers key %s", key)
	}
	return members, nil
}

// SIsMember 判断成员是否存在于集合中
func SIsMember(key string, member interface{}) (bool, error) {
	isMember, err := redisClient.SIsMember(ctx, key, member).Result()
	if err != nil {
		return false, errorx.Wrapf(err, errorx.CodeCacheError, "redis sismember key %s", key)
	}
	return isMember, nil
}
