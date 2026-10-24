package infra

import (
	"fmt"
	"time"

	etcdregistry "github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/jwt"
	redisinfra "kama_chat_server/internal/infra/redis"
	"kama_chat_server/pkg/discovery"
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

// ProvideRegistrar 初始化 Etcd 服务注册与旧版服务发现兼容注册
func ProvideRegistrar(serviceName string, port int) (registry.Registrar, func(), error) {
	etcdEndpoints := []string{"etcd:2379", "127.0.0.1:2379"}
	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect etcd: %w", err)
	}

	kratosRegistrar := etcdregistry.New(etcdCli)

	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   serviceName,
		Addr:   fmt.Sprintf("%s:%d", serviceName, port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	}

	cleanup := func() {
		if legacyRegister != nil {
			legacyRegister.Stop()
		}
		_ = etcdCli.Close()
	}

	return kratosRegistrar, cleanup, nil
}

// InitJWT 初始化 JWT 配置
func InitJWT(secret string, accessExpiryMinutes, refreshExpiryHours int) {
	jwt.Init(secret, accessExpiryMinutes, refreshExpiryHours)
}
