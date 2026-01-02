package redis

import (
	"context"
	"errors"
	"kama_chat_server/internal/config"
	"kama_chat_server/pkg/errorx"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

var redisClient *redis.Client
var ctx = context.Background()

// Init 初始化 Redis 连接
func Init() {
	conf := config.GetConfig()
	host := conf.RedisConfig.Host
	port := conf.RedisConfig.Port
	password := conf.RedisConfig.Password
	db := conf.Db
	addr := host + ":" + strconv.Itoa(port)

	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}

func SetKeyEx(key string, value string, timeout time.Duration) error {
	if err := redisClient.Set(ctx, key, value, timeout).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

func GetKey(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

func GetKeyNilIsErr(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", errorx.Wrapf(err, errorx.CodeNotFound, "redis key %s not found", key)
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

func GetKeyWithPrefixNilIsErr(prefix string) (string, error) {
	var cursor uint64
	var foundKeys []string
	for {
		var keys []string
		var err error
		// 使用 Scan 替代 Keys，逐步查找
		keys, cursor, err = redisClient.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}

		foundKeys = append(foundKeys, keys...)

		// 如果找到超过1个，直接报错返回，无需继续
		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan prefix %s: found %d keys, expected 1", prefix, len(foundKeys))
		}

		if cursor == 0 {
			break
		}
	}

	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis prefix %s not found", prefix)
	}

	return foundKeys[0], nil
}

func GetKeyWithSuffixNilIsErr(suffix string) (string, error) {
	var cursor uint64
	var foundKeys []string
	for {
		var keys []string
		var err error
		// 使用 Scan 替代 Keys，逐步查找
		keys, cursor, err = redisClient.Scan(ctx, cursor, "*"+suffix, 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan suffix %s", suffix)
		}

		foundKeys = append(foundKeys, keys...)

		// 如果找到超过1个，直接报错返回，无需继续
		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "redis scan suffix %s: found %d keys, expected 1", suffix, len(foundKeys))
		}

		if cursor == 0 {
			break
		}
	}

	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis suffix %s not found", suffix)
	}

	return foundKeys[0], nil
}

func DelKeyIfExists(key string) error {
	exists, err := redisClient.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 { // 键存在
		if err := redisClient.Del(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete key %s", key)
		}
	}
	// 无论键是否存在，都不返回错误
	return nil
}

func DelKeysWithPattern(pattern string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		// 每次扫描 500 条，减少循环次数
		keys, cursor, err = redisClient.Scan(ctx,cursor, pattern, 500).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
		}

		if len(keys) > 0 {
			// 使用 Unlink 进行非阻塞异步删除
			if err := redisClient.Unlink(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

// DelKeysWithPatterns 批量删除多个模式匹配的 key (使用 Pipeline)
func DelKeysWithPatterns(patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}

	// 遍历每一个 pattern
	for _, pattern := range patterns {
		var cursor uint64
		for {
			// 1. 分批扫描：每次扫描一部分 keys
			// 建议将 count 设为 500 或 1000，减少网络交互次数
			keys, newCursor, err := redisClient.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
			}

			// 2. 立即删除：扫到一批，就删一批
			if len(keys) > 0 {
				// 使用 Unlink 替代 Del，实现异步删除，不阻塞 Redis 主线程
				// keys... 语法会将切片展开，一次性传给 Redis，本身就是批量操作
				if err := redisClient.Unlink(ctx, keys...).Err(); err != nil {
					return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys with pattern %s", pattern)
				}
			}

			cursor = newCursor
			if cursor == 0 {
				break
			}
		}
	}

	return nil
}

func DelKeysWithPrefix(prefix string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		// 使用 Scan 替代 Keys
		keys, cursor, err = redisClient.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}

		if len(keys) > 0 {
			if err := redisClient.Del(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete keys with prefix %s", prefix)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

func DelKeysWithSuffix(suffix string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		// 使用 Scan 替代 Keys
		keys, cursor, err = redisClient.Scan(ctx, cursor, "*"+suffix, 100).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan suffix %s", suffix)
		}

		if len(keys) > 0 {
			if err := redisClient.Del(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete keys with suffix %s", suffix)
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

func DeleteAllRedisKeys() error {
	var cursor uint64 = 0
	for {
		keys, nextCursor, err := redisClient.Scan(ctx, cursor, "*", 0).Result()
		if err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "redis scan all keys")
		}
		cursor = nextCursor

		if len(keys) > 0 {
			if _, err := redisClient.Del(ctx, keys...).Result(); err != nil {
				return errorx.Wrap(err, errorx.CodeCacheError, "redis delete all keys")
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}
