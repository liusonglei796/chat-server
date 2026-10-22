package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/constants"
)

// ConnHub 网关长连接生命周期中心
// 负责维护本网关节点在线连接，以及在 Redis 动态路由表中登记与注销用户路由
// 基于底层 ConnManager 32 分段锁实现无通道锁化高并发接入
type ConnHub struct {
	Clients         *ConnManager
	cache           cacheinfra.AsyncCacheService
	gatewayGrpcAddr string
}

func NewConnHub(cache cacheinfra.AsyncCacheService, gatewayGrpcAddr string) *ConnHub {
	return &ConnHub{
		Clients:         NewConnManager(),
		cache:           cache,
		gatewayGrpcAddr: gatewayGrpcAddr,
	}
}

// RenewUserLocation 续期用户在 Redis 中的网关路由 TTL
func (h *ConnHub) RenewUserLocation(userId string) {
	if h.cache != nil && h.gatewayGrpcAddr != "" {
		h.cache.SubmitTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.cache.Expire(ctx, constants.CacheKeyUserGateway+userId, 120*time.Second)
		})
	}
}

// Start 启动网关连接中心（无锁化架构下由各连接协程并发处理，无须单协程事件循环）
func (h *ConnHub) Start() {
	zap.L().Info("ConnHub started")
}

// Close 关闭网关连接中心
func (h *ConnHub) Close() {
	zap.L().Info("ConnHub closed")
}

func (h *ConnHub) GetClient(userId string) *UserConn {
	return h.Clients.Get(userId)
}

func (h *ConnHub) GetOnlineCount() int64 {
	return h.Clients.Count()
}

// RegisterClient 注册长连接，并异步向 Redis 登记网关路由
func (h *ConnHub) RegisterClient(client *UserConn) {
	h.Clients.Store(client.Uuid, client)
	if h.cache != nil && h.gatewayGrpcAddr != "" {
		userId := client.Uuid
		h.cache.SubmitTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.cache.Set(ctx, constants.CacheKeyUserGateway+userId, h.gatewayGrpcAddr, 120*time.Second); err != nil {
				zap.L().Warn("failed to register user gateway in redis", zap.String("userId", userId), zap.Error(err))
			}
		})
	}
	zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
	welcomeMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "system",
		"content": "欢迎来到kama聊天服务器",
	})
	client.SafeSend(welcomeMsg, "")
}

// UnregisterClient 注销长连接，并异步清理 Redis 中的网关路由
func (h *ConnHub) UnregisterClient(client *UserConn) {
	h.Clients.Delete(client.Uuid)
	if h.cache != nil {
		userId := client.Uuid
		h.cache.SubmitTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.cache.Delete(ctx, constants.CacheKeyUserGateway+userId); err != nil {
				zap.L().Warn("failed to delete user gateway from redis", zap.String("userId", userId), zap.Error(err))
			}
		})
	}
	zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
	logoutMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "system",
		"content": "已退出登录",
	})
	client.SafeSend(logoutMsg, "")
}
