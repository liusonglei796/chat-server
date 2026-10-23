package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	metricsinfra "kama_chat_server/internal/infra/metrics"
	"kama_chat_server/pkg/constants"
)

const (
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	writeWait  = 10 * time.Second
)

// MessageBack 待推送给浏览器的消息载体
type MessageBack struct {
	Message []byte
	Uuid    string
}

// UserConn 一个在线用户的 WebSocket 连接
type UserConn struct {
	Conn        *websocket.Conn
	Uuid        string
	SendBack    chan *MessageBack
	hub         *ConnHub
	cleanupOnce sync.Once
}

// SafeSend 向该用户的 WebSocket 下行队列写入消息
func (c *UserConn) SafeSend(payload []byte, uuid string) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("send to closed user conn recovered", zap.String("userId", c.Uuid))
		}
	}()
	c.SendBack <- &MessageBack{
		Message: payload,
		Uuid:    uuid,
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Read 从 WebSocket 读取消息
func (c *UserConn) Read() {
	defer c.cleanup()

	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		c.hub.RenewUserLocation(c.Uuid)
		return nil
	})

	zap.L().Info("ws read goroutine start", zap.String("userId", c.Uuid))
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				zap.L().Error("ws read error", zap.String("userId", c.Uuid), zap.Error(err))
			} else {
				zap.L().Info("ws connection closed", zap.String("userId", c.Uuid), zap.Error(err))
			}
			return
		}
		zap.L().Debug("ws received message", zap.String("userId", c.Uuid), zap.String("message", string(jsonMessage)))
		c.hub.RenewUserLocation(c.Uuid)

		var req messagepb.SendMessageRequest
		if err := json.Unmarshal(jsonMessage, &req); err != nil {
			zap.L().Error("unmarshal ws message error", zap.String("userId", c.Uuid), zap.Error(err))
			continue
		}
		req.SendId = c.Uuid

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := grpcinfra.SendMessage(ctx, &req)
		cancel()
		if err != nil {
			zap.L().Warn("send message failed", zap.String("userId", c.Uuid), zap.Error(err))
			errMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "error",
				"message": err.Error(),
			})
			c.SafeSend(errMsg, "")
			continue
		}

		ackMsg, _ := json.Marshal(map[string]interface{}{
			"type":          "ack",
			"message_uuid":  resp.MessageUuid,
			"created_at":    resp.CreatedAt,
			"client_msg_id": req.ClientMsgId,
		})
		c.SafeSend(ackMsg, resp.MessageUuid)
	}
}

func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		zap.L().Info("ws cleanup start", zap.String("userId", c.Uuid))

		metricsinfra.OnlineConnections.Dec()
		metricsinfra.TotalDisconnections.Inc()

		c.hub.UnregisterClient(c)
		close(c.SendBack)

		if err := c.Conn.Close(); err != nil {
			zap.L().Warn("ws close connection error", zap.String("userId", c.Uuid), zap.Error(err))
		}

		zap.L().Info("ws cleanup done", zap.String("userId", c.Uuid))
	})
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket 客户端
func (c *UserConn) Write() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	zap.L().Info("ws write goroutine start", zap.String("userId", c.Uuid))
	for {
		select {
		case messageBack, ok := <-c.SendBack:
			if !ok {
				_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				zap.L().Info("ws write goroutine exit: SendBack closed", zap.String("userId", c.Uuid))
				return
			}

			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message); err != nil {
				zap.L().Error("ws write error", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}

		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}
			c.hub.RenewUserLocation(c.Uuid)
		}
	}
}

// NewClientInit 当接收到前端建立长连接请求时调用
func NewClientInit(w http.ResponseWriter, r *http.Request, clientId string, hub *ConnHub) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		zap.L().Error("websocket upgrade error", zap.Error(err))
		return
	}
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		hub:      hub,
	}
	hub.RegisterClient(client)

	metricsinfra.OnlineConnections.Inc()
	metricsinfra.TotalConnections.Inc()

	go client.Read()
	go client.Write()
	zap.L().Info("ws连接成功", zap.String("userId", clientId))
}

// ClientLogout 当接受到前端有登出消息时调用
func ClientLogout(clientId string, hub *ConnHub) error {
	client := hub.GetClient(clientId)
	if client != nil {
		client.cleanup()
	}
	return nil
}
