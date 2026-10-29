package infra

import (
	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/debounce"
	"kama_chat_server/internal/infra/lock"
	redisinfra "kama_chat_server/internal/infra/redis"
)

// ProvideCache 初始化 Redis 异步服务并导出
func ProvideCache() (cacheinfra.AsyncCacheService, func(), error) {
	cacheService := redisinfra.Init()
	cleanup := func() {
		if rc, ok := cacheService.(*redisinfra.RedisCache); ok {
			rc.Release()
		}
	}
	return cacheService, cleanup, nil
}

// ProvideLock 提供 Redis 分布式互斥锁
func ProvideLock() lock.Distributed {
	return lock.NewRedis(redisinfra.NewClient())
}

// ProvideDebounceStore 提供防抖底层存储
func ProvideDebounceStore() debounce.Store {
	return debounce.NewRedisStore(redisinfra.NewClient())
}
