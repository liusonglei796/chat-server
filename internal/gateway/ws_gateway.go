package gateway

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp/websocket"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	metricsinfra "kama_chat_server/internal/infra/metrics"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/interceptor"
)

const (
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	writeWait  = 10 * time.Second
)

// MessageBack 待推送给客户端的消息载体
type MessageBack struct {
	Message []byte
	Uuid    string
}

// UserConn 一个在线用户的 WebSocket 连接抽象 (Reactor 事件驱动模型)
// 底层由 nbio Multi-Reactor 管理网络 I/O 事件，连接自身无需常驻读写 Goroutine。
type UserConn struct {
	Conn        *websocket.Conn
	Uuid        string
	SendBack    chan *MessageBack
	hub         *ConnHub
	cleanupOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc

	closed     atomic.Bool
	lastActive atomic.Int64
}

// SafeSend 向该用户的 WebSocket 下行队列写入消息并以非阻塞方式推送
// 具备应用层背压水线保护，防止慢客户端/弱网无限堆积内存
func (c *UserConn) SafeSend(payload []byte, uuid string) {
	if c.closed.Load() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("send to closed user conn recovered", zap.String("userId", c.Uuid))
		}
	}()

	// 1. 应用层背压限流缓冲保护
	select {
	case c.SendBack <- &MessageBack{
		Message: payload,
		Uuid:    uuid,
	}:
	default:
		zap.L().Warn("user conn send buffer full (backpressure), dropping message",
			zap.String("userId", c.Uuid),
			zap.String("msgUuid", uuid),
		)
		return
	}

	// 2. 刷新最后活跃时间
	c.lastActive.Store(time.Now().Unix())

	// 3. 仅在真实连接存在时执行非阻塞写入并排空缓冲（兼容 Conn 为 nil 的单元测试与离线状态）
	if c.Conn != nil {
		for {
			select {
			case msg, ok := <-c.SendBack:
				if !ok {
					return
				}
				if err := c.Conn.WriteMessage(websocket.TextMessage, msg.Message); err != nil {
					zap.L().Warn("reactor ws write error", zap.String("userId", c.Uuid), zap.Error(err))
					c.cleanup()
					return
				}
			default:
				return
			}
		}
	}
}

// SendPing 发送心跳 Ping 探测帧
func (c *UserConn) SendPing() error {
	if c.closed.Load() || c.Conn == nil {
		return nil
	}
	err := c.Conn.WriteMessage(websocket.PingMessage, nil)
	if err != nil {
		zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
		c.cleanup()
		return err
	}
	if c.hub != nil {
		c.hub.RenewUserLocation(c.Uuid)
	}
	return nil
}

// cleanup 清理连接并注销状态
func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		c.closed.Store(true)
		if c.cancel != nil {
			c.cancel()
		}
		zap.L().Info("ws cleanup start", zap.String("userId", c.Uuid))

		metricsinfra.OnlineConnections.Dec()
		metricsinfra.TotalDisconnections.Inc()

		if c.hub != nil {
			c.hub.UnregisterClient(c)
		}
		close(c.SendBack)

		if c.Conn != nil {
			_ = c.Conn.Close()
		}

		zap.L().Info("ws cleanup done", zap.String("userId", c.Uuid))
	})
}

// HandleUserMessageTask 异步业务协程池任务：解耦 Reactor 事件循环与耗时 gRPC 调用
func HandleUserMessageTask(c *UserConn, jsonMessage []byte, cmdHeader *struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	TargetId string `json:"target_id"`
	AfterSeq int64  `json:"after_seq"`
	Limit    int32  `json:"limit"`
}) {
	if cmdHeader != nil && (cmdHeader.Type == "sync" || cmdHeader.Action == "sync") {
		limit := cmdHeader.Limit
		if limit <= 0 {
			limit = 50
		}
		ctx := context.WithValue(context.Background(), interceptor.UserIDKey, c.Uuid)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		syncResp, syncErr := grpcinfra.GetMessages(ctx, &messagepb.GetMessagesRequest{
			TargetId:    cmdHeader.TargetId,
			Cursor:      "after_seq:" + strconv.FormatInt(cmdHeader.AfterSeq, 10),
			PageSize:    limit,
			RequesterId: c.Uuid,
		})
		cancel()
		if syncErr != nil {
			zap.L().Warn("ws sync messages failed", zap.String("userId", c.Uuid), zap.Error(syncErr))
			errMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "error",
				"message": syncErr.Error(),
			})
			c.SafeSend(errMsg, "")
			return
		}
		respPayload, _ := json.Marshal(map[string]interface{}{
			"type":      "sync_resp",
			"target_id": cmdHeader.TargetId,
			"messages":  syncResp.List,
			"has_more":  syncResp.HasMore,
		})
		c.SafeSend(respPayload, "")
		return
	}

	var req messagepb.SendMessageRequest
	if err := json.Unmarshal(jsonMessage, &req); err != nil {
		zap.L().Error("unmarshal ws message error", zap.String("userId", c.Uuid), zap.Error(err))
		return
	}
	req.SendId = c.Uuid

	// 将连接身份写入 gRPC context，message 服务会校验请求体与鉴权上下文
	ctx := context.WithValue(context.Background(), interceptor.UserIDKey, c.Uuid)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	resp, err := grpcinfra.SendMessage(ctx, &req)
	cancel()
	if err != nil {
		zap.L().Warn("send message failed", zap.String("userId", c.Uuid), zap.Error(err))
		errMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "error",
			"message": err.Error(),
		})
		c.SafeSend(errMsg, "")
		return
	}

	ackMsg, _ := json.Marshal(map[string]interface{}{
		"type":          "ack",
		"message_uuid":  resp.MessageUuid,
		"created_at":    resp.CreatedAt,
		"client_msg_id": req.ClientMsgId,
	})
	c.SafeSend(ackMsg, resp.MessageUuid)
}

// InitReactorClient 当客户端通过 Reactor 成功握手时调用，完成会话绑定与在线登记
func InitReactorClient(conn *websocket.Conn, clientId string, hub *ConnHub) *UserConn {
	ctx, cancel := context.WithCancel(context.Background())
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		hub:      hub,
		ctx:      ctx,
		cancel:   cancel,
	}
	client.lastActive.Store(time.Now().Unix())

	if conn != nil {
		conn.SetSession(client)
	}

	if hub != nil {
		hub.RegisterClient(client)
	}

	metricsinfra.OnlineConnections.Inc()
	metricsinfra.TotalConnections.Inc()

	zap.L().Info("Reactor ws 连接成功", zap.String("userId", clientId))
	return client
}

// NewClientInit 保持历史工厂方法签名兼容
func NewClientInit(conn *websocket.Conn, clientId string, hub *ConnHub) *UserConn {
	return InitReactorClient(conn, clientId, hub)
}

// ClientLogout 当收到前端登出请求时调用
func ClientLogout(clientId string, hub *ConnHub) error {
	client := hub.GetClient(clientId)
	if client != nil {
		client.cleanup()
	}
	return nil
}
