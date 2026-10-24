package infra

import (
	cacheinfra "kama_chat_server/internal/infra/cache"
	redisinfra "kama_chat_server/internal/infra/redis"
)

// ProvideCache 初始化 Redis 异步缓存服务并导出清理函数
func ProvideCache() (cacheinfra.AsyncCacheService, func(), error) {
	cacheService := redisinfra.Init()
	cleanup := func() {
		if rc, ok := cacheService.(*redisinfra.RedisCache); ok {
			rc.Release()
		}
	}
	return cacheService, cleanup, nil
}
