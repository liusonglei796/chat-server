package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/pkg/constants"
)

// ClientHub 网关长连接生命周期中心
// 负责维护本网关节点在线连接，以及在 Redis 动态路由表中登记与注销用户路由
type ClientHub struct {
	Clients         *ConnManager
	Login           chan *UserConn
	Logout          chan *UserConn
	closeOnce       sync.Once
	quit            chan os.Signal
	cache           store.AsyncCacheService
	gatewayGrpcAddr string
}

func NewClientHub(cache store.AsyncCacheService, gatewayGrpcAddr string) *ClientHub {
	return &ClientHub{
		Clients:         NewConnManager(),
		Login:           make(chan *UserConn, 1024),
		Logout:          make(chan *UserConn, 1024),
		quit:            make(chan os.Signal, 1),
		cache:           cache,
		gatewayGrpcAddr: gatewayGrpcAddr,
	}
}

// RenewUserLocation 续期用户在 Redis 中的网关路由 TTL
func (h *ClientHub) RenewUserLocation(userId string) {
	if h.cache != nil && h.gatewayGrpcAddr != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = h.cache.Expire(ctx, constants.CacheKeyUserGateway+userId, 120*time.Second)
		cancel()
	}
}

func (h *ClientHub) Start() {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error(fmt.Sprintf("chat server client hub panic: %v", r))
		}
		h.closeOnce.Do(func() {
			close(h.Login)
			close(h.Logout)
		})
	}()

	for {
		select {
		case client := <-h.Login:
			h.Clients.Store(client.Uuid, client)
			if h.cache != nil && h.gatewayGrpcAddr != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := h.cache.Set(ctx, constants.CacheKeyUserGateway+client.Uuid, h.gatewayGrpcAddr, 120*time.Second); err != nil {
					zap.L().Warn("failed to register user gateway in redis", zap.String("userId", client.Uuid), zap.Error(err))
				}
				cancel()
			}
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			welcomeMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "system",
				"content": "欢迎来到kama聊天服务器",
			})
			client.SafeSend(welcomeMsg, "")
		case client := <-h.Logout:
			h.Clients.Delete(client.Uuid)
			if h.cache != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := h.cache.Delete(ctx, constants.CacheKeyUserGateway+client.Uuid); err != nil {
					zap.L().Warn("failed to delete user gateway from redis", zap.String("userId", client.Uuid), zap.Error(err))
				}
				cancel()
			}
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			logoutMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "system",
				"content": "已退出登录",
			})
			client.SafeSend(logoutMsg, "")
		case <-h.quit:
			return
		}
	}
}

func (h *ClientHub) Close() {
	h.closeOnce.Do(func() {
		close(h.Login)
		close(h.Logout)
	})
}

func (h *ClientHub) GetClient(userId string) *UserConn {
	return h.Clients.Get(userId)
}

func (h *ClientHub) GetOnlineCount() int64 {
	return h.Clients.Count()
}

func (h *ClientHub) RegisterClient(client *UserConn) {
	h.Login <- client
}

func (h *ClientHub) UnregisterClient(client *UserConn) {
	h.Logout <- client
}
