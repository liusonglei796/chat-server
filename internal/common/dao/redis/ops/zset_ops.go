// Package ops 提供 Redis 命令的轻量封装（集合、字符串、有序集合、计数器、Key 删除等），
// 并把原生 Redis 错误统一包装为带业务错误码（errorx）的错误。
package ops

import (
	"context"
	"errors"

	"kama_chat_server/pkg/errorx"

	"github.com/redis/go-redis/v9"
)

// ZAdd 向有序集合添加成员或更新已有成员的 score。
func ZAdd(client *redis.Client, ctx context.Context, key string, score float64, member string) error {
	if err := client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis zadd key %s", key)
	}
	return nil
}

// ZRevRangeByScore 按分数从大到小获取有序集合成员（支持偏移与数量限制）。
// max/min 可为 "+inf", "-inf" 或具体分数字符串（支持 "(" 前缀表示开区间）。
func ZRevRangeByScore(client *redis.Client, ctx context.Context, key string, max, min string, offset, count int64) ([]string, error) {
	members, err := client.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Max:    max,
		Min:    min,
		Offset: offset,
		Count:  count,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, errorx.Wrapf(err, errorx.CodeCacheError, "redis zrevrangebyscore key %s", key)
	}
	return members, nil
}

// ZRemRangeByRank 移除有序集合中指定排名区间的成员（排名从 0 开始，0 为最低分）。
// 例如移除最早的数据：start=0, stop=-201。
func ZRemRangeByRank(client *redis.Client, ctx context.Context, key string, start, stop int64) error {
	if err := client.ZRemRangeByRank(ctx, key, start, stop).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis zremrangebyrank key %s", key)
	}
	return nil
}

// ZRem 移除有序集合中的一个或多个成员。
func ZRem(client *redis.Client, ctx context.Context, key string, members ...interface{}) error {
	if err := client.ZRem(ctx, key, members...).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis zrem key %s", key)
	}
	return nil
}

// ZCard 获取有序集合的基数（元素总数）。Key 不存在时返回 0。
func ZCard(client *redis.Client, ctx context.Context, key string) (int64, error) {
	count, err := client.ZCard(ctx, key).Result()
	if err != nil {
		return 0, errorx.Wrapf(err, errorx.CodeCacheError, "redis zcard key %s", key)
	}
	return count, nil
}

// ZScore 获取有序集合中指定成员的 score。若成员不存在返回 0。
func ZScore(client *redis.Client, ctx context.Context, key string, member string) (float64, error) {
	score, err := client.ZScore(ctx, key, member).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, errorx.Wrapf(err, errorx.CodeCacheError, "redis zscore key %s", key)
	}
	return score, nil
}
