// Package redis 提供 Redis 缓存操作的封装
// 本文件仅包含 Redis 连接初始化逻辑
// 使用 github.com/redis/go-redis/v9 作为底层客户端
package redis

import (
	"strconv"

	"kama_chat_server/internal/config"

	"github.com/redis/go-redis/v9"
)

// Init 初始化 Redis 连接并返回缓存服务
// 从配置文件读取连接参数并创建客户端实例
// 返回 AsyncCacheService 接口，供 Service 层依赖注入使用
func Init() AsyncCacheService {
	conf := config.GetConfig()
	host := conf.RedisConfig.Host         // Redis 服务器地址
	port := conf.RedisConfig.Port         // Redis 端口
	password := conf.RedisConfig.Password // 密码，无密码留空
	db := conf.RedisConfig.Db             // 数据库编号

	// 拼接地址：host:port
	addr := host + ":" + strconv.Itoa(port)

	// 创建 Redis 客户端
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		// 连接池配置
		PoolSize:     50, // 最大连接数
		MinIdleConns: 15, // 最小空闲连接，与 Worker 数量匹配
	})

	// 创建并返回缓存服务实例
	// 启动 15 个 Worker，缓冲区大小 3000，适用于多 Service 共享
	return NewRedisCache(client, 15, 3000)
}
