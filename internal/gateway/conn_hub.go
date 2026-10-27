package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
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
	writePool       *ants.Pool
	stopCh          chan struct{}
	closeOnce       sync.Once
}

func NewConnHub(cache cacheinfra.AsyncCacheService, gatewayGrpcAddr string) *ConnHub {
	// 初始化网关写协程池 (容量设为 10000 Worker，按需动态弹性扩缩容)
	writePool, err := ants.NewPool(10000,
		ants.WithNonblocking(true),
		ants.WithPanicHandler(func(p any) {
			zap.L().Error("gateway write pool panic recovered", zap.Any("panic", p))
		}),
	)
	if err != nil {
		zap.L().Warn("create gateway write ants pool failed, fallback to direct goroutine", zap.Error(err))
	}

	return &ConnHub{
		Clients:         NewConnManager(),
		cache:           cache,
		gatewayGrpcAddr: gatewayGrpcAddr,
		writePool:       writePool,
		stopCh:          make(chan struct{}),
	}
}

// SubmitWriteTask 提交待发送任务至网关写协程池
func (h *ConnHub) SubmitWriteTask(task func()) {
	if h.writePool != nil {
		if err := h.writePool.Submit(task); err == nil {
			return
		}
	}
	// 协程池降级策略：若池已满或未初始化，动态起轻量协程执行，防止消息阻塞丢弃
	go task()
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

// Start 启动网关连接中心与全局心跳巡检循环
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

// Close 关闭网关连接中心与协程池
func (h *ConnHub) Close() {
	h.closeOnce.Do(func() {
		close(h.stopCh)
		if h.writePool != nil {
			h.writePool.Release()
		}
		zap.L().Info("ConnHub closed")
	})
}

func (h *ConnHub) GetClient(userId string) *UserConn {
	return h.Clients.Get(userId)
}

func (h *ConnHub) GetOnlineCount() int64 {
	return h.Clients.Count()
}

// RegisterClient 注册长连接，并同步向 Redis 登记网关路由。
// 路由必须在下发连接成功前可见，否则刚建连就可能推送不到；断连删除仍走异步。
func (h *ConnHub) RegisterClient(client *UserConn) {
	h.Clients.Store(client.Uuid, client)
	if h.cache != nil && h.gatewayGrpcAddr != "" {
		userId := client.Uuid
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := h.cache.Set(ctx, constants.CacheKeyUserGateway+userId, h.gatewayGrpcAddr, 120*time.Second)
		cancel()
		if err != nil {
			zap.L().Warn("failed to register user gateway in redis", zap.String("userId", userId), zap.Error(err))
		}
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
