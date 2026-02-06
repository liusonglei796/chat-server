// ws_gateway.go
// 核心职责：WebSocket 网关 (Gateway)
//
// 架构角色：
// 1. **连接维持**: 它是所有用户设备 (手机/网页) 与后端服务器的**唯一**长连接入口。
// 2. **协议升级**: 负责把 HTTP 协议升级为 WebSocket 协议 (Upgrade)。
// 3. **读写分离**: 每个连接启动两个协程 (ReadLoop/WriteLoop) 处理收发数据。
//
// 关键协作：
// - **收消息**: 收到用户消息后，调用 `broker.Publish` (不关心是发给 Kafka 还是直接转发)。
// - **发消息**: `kafka_broker` 最终会调用这里的 `WriteMessage` 把消息推给用户。
package chat

import (
	"context"
	"encoding/json"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// pongWait 等待客户端 pong 响应的超时时间
	pongWait = 60 * time.Second
	// pingPeriod 发送 ping 的间隔（必须小于 pongWait）
	pingPeriod = (pongWait * 9) / 10
	// writeWait 写操作的超时时间
	writeWait = 10 * time.Second
)

// MessageBack 用于向 WebSocket 客户端推送消息
// 包含实际的 JSON 消息体和用于状态更新的消息 ID
// 场景：
// 1. 推送给接收者（Let users see new messages）
// 2. 回显给发送者（Let sender confirm message sent）
type MessageBack struct {
	Message []byte
	Uuid    string
}

// UserConn 表示一个 WebSocket 客户端连接
// 代表的是你的后端服务器和用户浏览器之间的一条那根网线。
//
//	有了 WebSocket： 用户 B 和服务器之间建立了一根"长管子"。服务器一旦收到 A 发给 B 的消息，不需要等 B 询问，直接顺着这根管子把消息"推"到 B 的屏幕上。
type UserConn struct {
	Conn        *websocket.Conn
	Uuid        string
	SendTo      chan []byte       // 缓冲通道（Channel 模式备用）
	SendBack    chan *MessageBack // 给前端
	broker      MessageBroker     // 注入的消息代理
	cleanupOnce sync.Once         // 确保 cleanup 只执行一次（Read 退出 和 ClientLogout 可能并发触发）
}

//  gorilla/websocket 默认的安全机制会拦截跨域请求。

// 比如：你的 Go 后端运行在 localhost:8080，但你的 Vue/React 前端运行在 localhost:3000。如果不写这段代码，默认会连接失败（报 403 Forbidden 错误）。
// return true 就是为了解决跨域问题，允许任何来源的连接。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Read 从 WebSocket 读取消息
// 安全: 服务端会用连接时认证的用户 ID 覆盖消息中的 SendId，防止 IDOR 攻击
// 退出时通过 defer 执行 cleanup：注销客户端 → 关闭通道 → 关闭连接
func (c *UserConn) Read() {
	// cleanup: Read 退出时必须释放资源，防止内存泄漏和幽灵连接
	defer c.cleanup()

	// 设置心跳：初始读超时 + pong 回调续期
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	zap.L().Info("ws read goroutine start", zap.String("userId", c.Uuid))
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			// 正常关闭（客户端主动断开）不记录 Error 级别日志
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				zap.L().Error("ws read error", zap.String("userId", c.Uuid), zap.Error(err))
			} else {
				zap.L().Info("ws connection closed", zap.String("userId", c.Uuid), zap.Error(err))
			}
			return
		}

		zap.L().Debug("ws received message", zap.String("userId", c.Uuid), zap.String("message", string(jsonMessage)))

		// 安全: 注入真实的用户 ID，覆盖客户端传入的 send_id（防止 IDOR）
		securedMessage := injectRealSenderId(jsonMessage, c.Uuid)

		// 通过接口发布消息，不关心具体实现
		if err := c.broker.Publish(context.Background(), securedMessage); err != nil {
			zap.L().Error("publish message error", zap.String("userId", c.Uuid), zap.Error(err))
		}
	}
}

// cleanup 释放 UserConn 占用的资源（通过 sync.Once 确保只执行一次）
// 调用顺序：注销客户端 → 安全关闭通道 → 关闭 WebSocket 连接
func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		zap.L().Info("ws cleanup start", zap.String("userId", c.Uuid))

		// 1. 从 broker 注销，防止新消息写入 SendBack
		c.broker.UnregisterClient(c)

		// 2. 安全关闭 SendBack 通道（让 Write goroutine 退出 range 循环）
		safeCloseMessageBackChan(c.SendBack)

		// 3. 安全关闭 SendTo 通道
		safeCloseBytesChan(c.SendTo)

		// 4. 关闭 WebSocket 连接
		if err := c.Conn.Close(); err != nil {
			zap.L().Warn("ws close connection error", zap.String("userId", c.Uuid), zap.Error(err))
		}

		zap.L().Info("ws cleanup done", zap.String("userId", c.Uuid))
	})
}

// safeCloseMessageBackChan 安全关闭 *MessageBack 通道，防止 double-close panic
func safeCloseMessageBackChan(ch chan *MessageBack) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("SendBack channel already closed", zap.Any("recover", r))
		}
	}()
	close(ch)
}

// safeCloseBytesChan 安全关闭 []byte 通道，防止 double-close panic
func safeCloseBytesChan(ch chan []byte) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("SendTo channel already closed", zap.Any("recover", r))
		}
	}()
	close(ch)
}

// injectRealSenderId 将真实的用户 ID 注入消息，覆盖客户端传入的 send_id
// 这是防止 WebSocket 消息 IDOR 攻击的关键安全措施
func injectRealSenderId(jsonMessage []byte, realUserId string) []byte {
	var msg map[string]interface{}
	if err := json.Unmarshal(jsonMessage, &msg); err != nil {
		zap.L().Error("Failed to unmarshal message for security injection", zap.Error(err))
		return jsonMessage // 如果解析失败，原样返回（后续会被校验拦截）
	}

	// 覆盖 send_id 字段
	msg["send_id"] = realUserId

	securedMessage, err := json.Marshal(msg)
	if err != nil {
		zap.L().Error("Failed to marshal secured message", zap.Error(err))
		return jsonMessage
	}

	return securedMessage
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket 客户端
// 同时定期发送 Ping 帧维持心跳，检测静默断连的客户端
func (c *UserConn) Write() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	zap.L().Info("ws write goroutine start", zap.String("userId", c.Uuid))
	for {
		select {
		case messageBack, ok := <-c.SendBack:
			if !ok {
				// SendBack 通道已关闭（由 cleanup 触发），发送 Close 帧后退出
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

			// 通过 Repository 接口更新消息状态（遵循依赖倒置原则）
			if repo := c.broker.GetMessageRepo(); repo != nil {
				if err := repo.UpdateStatus(messageBack.Uuid, message_status.Sent); err != nil {
					zap.L().Error("更新消息状态失败", zap.Error(err))
				}
			}

		case <-ticker.C:
			// 定期发送 Ping 帧，检测客户端是否还在线
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}
		}
	}
}

// NewClientInit 当接受到前端有登录消息时，会调用该函数
// broker: 消息代理实例，通过依赖注入传入
func NewClientInit(c *gin.Context, clientId string, broker MessageBroker) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return
	}
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendTo:   make(chan []byte, constants.CHANNEL_SIZE),
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		broker:   broker,
	}
	// 通过接口注册websocket客户端
	broker.RegisterClient(client)
	go client.Read()
	go client.Write()
	zap.L().Info("ws连接成功")
}

// ClientLogout 当接受到前端有登出消息时，会调用该函数
// 通过 cleanup 统一释放资源（sync.Once 保证幂等）
func ClientLogout(clientId string, broker MessageBroker) error {
	client := broker.GetClient(clientId)
	if client != nil {
		client.cleanup()
	}
	return nil
}
