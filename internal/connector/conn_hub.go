package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/constants"
)

// ConnHub 连接服务长连接生命周期中枢
// 负责维护本连接服务节点在线连接，以及在 Redis 动态路由表中登记与注销用户在线路由
// 基于底层 ConnManager 32 分段锁实现无通道锁化高并发接入
type ConnHub struct {
	Clients          *ConnManager
	cache            cacheinfra.AsyncCacheService
	connGrpcAddr     string
	incomingProducer IncomingMessageProducer
	stopCh           chan struct{}
	closeOnce        sync.Once
}

// SetIncomingProducer 注入上行聊天消息 Kafka 生产者
func (h *ConnHub) SetIncomingProducer(p IncomingMessageProducer) {
	h.incomingProducer = p
}

// IncomingProducer 获取上行聊天消息 Kafka 生产者
func (h *ConnHub) IncomingProducer() IncomingMessageProducer {
	if h == nil {
		return nil
	}
	return h.incomingProducer
}

func NewConnHub(cache cacheinfra.AsyncCacheService, connGrpcAddr string) *ConnHub {
	return &ConnHub{
		Clients:      NewConnManager(),
		cache:        cache,
		connGrpcAddr: connGrpcAddr,
		stopCh:       make(chan struct{}),
	}
}

// RenewUserLocation 续期用户在 Redis 中的连接服务路由 TTL
func (h *ConnHub) RenewUserLocation(userId string) {
	if h.cache != nil && h.connGrpcAddr != "" {
		h.cache.SubmitTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.cache.Expire(ctx, constants.CacheKeyUserConn+userId, 120*time.Second)
		})
	}
}

// Start 启动连接服务中枢与全局心跳巡检循环
func (h *ConnHub) Start() {
	zap.L().Info("ConnHub started")
	go h.startHeartbeatLoop()
}

// startHeartbeatLoop 升级为 Reactor 架构下的空闲超时惰性巡检：
// 告别旧模型下全局单协程周期性遍历几十万连接下发 Ping 帧的“惊群”与 CPU 尖刺；
// 改由客户端主动心跳刷新 lastActive，服务端低频（每 30s）巡检并精准回收超时僵尸连接。
func (h *ConnHub) startHeartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			now := time.Now().Unix()
			timeoutThreshold := int64(pongWait / time.Second)
			h.Clients.Range(func(userId string, conn *UserConn) bool {
				last := conn.lastActive.Load()
				if last > 0 && (now-last) > timeoutThreshold {
					zap.L().Warn("reactor idle timeout, closing dead connection",
						zap.String("userId", userId),
						zap.Int64("idleSec", now-last),
					)
					conn.cleanup()
				}
				return true
			})
		}
	}
}

// Close 关闭连接服务中枢
func (h *ConnHub) Close() {
	h.closeOnce.Do(func() {
		close(h.stopCh)
		zap.L().Info("ConnHub closed")
	})
}

func (h *ConnHub) GetClient(userId string) *UserConn {
	return h.Clients.Get(userId)
}

func (h *ConnHub) GetOnlineCount() int64 {
	return h.Clients.Count()
}

// RegisterClient 注册长连接，挂载对称 OnClose 注销回调，并同步向 Redis 登记连接服务路由。
// 路由必须在下发连接成功前可见，否则刚建连就可能推送不到；断连删除仍走异步。
func (h *ConnHub) RegisterClient(client *UserConn) {
	if client == nil {
		return
	}
	// 挂载对称生命周期回调：连接销毁时自动触发 UnregisterClient
	client.OnClose = func(c *UserConn) {
		h.UnregisterClient(c)
	}
	h.Clients.Store(client.Uuid, client)
	if h.cache != nil && h.connGrpcAddr != "" {
		userId := client.Uuid
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := h.cache.Set(ctx, constants.CacheKeyUserConn+userId, h.connGrpcAddr, 120*time.Second)
		cancel()
		if err != nil {
			zap.L().Warn("failed to register user conn location in redis", zap.String("userId", userId), zap.Error(err))
		}
	}
	zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
	welcomeMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "system",
		"content": "欢迎来到kama聊天服务器",
	})
	client.SafeSend(welcomeMsg, "")
}

// UnregisterClient 注销长连接，并异步清理 Redis 中的连接服务路由
func (h *ConnHub) UnregisterClient(client *UserConn) {
	h.Clients.Delete(client.Uuid)
	if h.cache != nil {
		userId := client.Uuid
		h.cache.SubmitTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.cache.Delete(ctx, constants.CacheKeyUserConn+userId); err != nil {
				zap.L().Warn("failed to delete user conn location from redis", zap.String("userId", userId), zap.Error(err))
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
