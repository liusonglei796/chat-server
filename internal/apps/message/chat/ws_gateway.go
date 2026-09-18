// ws_gateway.go
// 核心职责：WebSocket 网关 (Gateway)
//
// 架构角色：
// 1. **连接维持**: 它是所有用户设备 (手机/网页) 与后端服务器的**唯一**长连接入口。
// 2. **协议升级**: 负责把 HTTP 协议升级为 WebSocket 协议 (Upgrade)。
// 3. **读写分离**: 每个连接启动两个协程 (ReadLoop/WriteLoop) 处理收发数据。
//
// 关键协作：
// - **收消息**: 收到用户消息后，通过 gRPC 直接调用 message_service 的 SendMessage（同步鉴权、幂等、落库）。
// - **发消息**: 后端微服务通过 gRPC 直推网关 GatewayGrpcServer，将消息放入此连接的 SendBack channel。
package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/metrics"
	"kama_chat_server/pkg/constants"
)

const (
	// pongWait 等待客户端 pong 响应的超时时间
	pongWait = 60 * time.Second
	// pingPeriod 发送 ping 的间隔（必须小于 pongWait）
	pingPeriod = (pongWait * 9) / 10
	// writeWait 写操作的超时时间
	writeWait = 10 * time.Second
)

// MessageBack 待推送给浏览器的消息载体
// 面向对象：一条聊天消息（1 条消息 = 1 个 MessageBack 实例）
// 生命周期：由后端微服务通过 gRPC 直推到网关，写入 UserConn.SendBack channel，由 Write goroutine 取出后推送到浏览器
//
// 字段说明：internal/service/chat/ws_gateway.go
//   - Message: 序列化后的 JSON 响应体（即前端最终收到的完整 JSON）
//   - Uuid:    消息 ID，Write goroutine 推送成功后据此更新 MySQL 消息状态为"已发送"
type MessageBack struct {
	Message []byte // 序列化后的 JSON 响应体（GetMessageListRespond / AVMessageRespond）
	Uuid    string // 消息唯一标识，用于推送成功后更新 message.status = Sent
}

// UserConn 一个在线用户的 WebSocket 连接（1 个在线用户 = 1 个 UserConn 实例）
// 面向对象：用户的网络连接（不是消息本身）
// 生命周期：用户登录时由 NewClientInit 创建，断开/登出时由 cleanup 销毁
type UserConn struct {
	Conn        *websocket.Conn   // 底层 WebSocket 连接（通往用户浏览器的 TCP 管道）
	Uuid        string            // 用户 ID（来自 JWT，可信）
	SendBack    chan *MessageBack // 待推送消息队列：gRPC 直推写入 → Write goroutine 读取并推送到浏览器
	hub         *ClientHub        // 网关长连接生命周期中心，用于心跳续期、注销连接等
	cleanupOnce sync.Once         // 确保 cleanup 只执行一次（Read 退出和 ClientLogout 可能并发触发）
}

// SafeSend 向该用户的 WebSocket 下行队列写入消息（由 Write goroutine 统一执行网络写，防并发写冲突）
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

//  gorilla/websocket 默认的安全机制会拦截跨域请求。

// 比如：你的 Go 后端运行在 localhost:8080，但你的 Vue/React 前端运行在 localhost:3000。如果不写这段代码，默认会连接失败（报 403 Forbidden 错误）。
// return true 就是为了解决跨域问题，允许任何来源的连接。
// [第三方库: github.com/gorilla/websocket] websocket.Upgrader 用于将 HTTP 连接升级为 WebSocket 协议
var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Read 从 WebSocket 读取消息
// 退出时通过 defer 执行 cleanup：注销客户端 → 关闭通道 → 关闭连接
func (c *UserConn) Read() {
	// cleanup: Read 退出时必须释放资源，防止内存泄漏和幽灵连接
	defer c.cleanup()

	// 设置心跳：初始读超时 + pong 回调续期
	// 改进建议：pongWait 定义了等待客户端 pong 响应的超时时间（60秒）
	// 如果客户端在 60 秒内未响应 ping，连接将被断开
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait)) // [第三方库: github.com/gorilla/websocket] Conn.SetReadDeadline 设置读超时
	c.Conn.SetPongHandler(func(string) error {           // [第三方库: github.com/gorilla/websocket] Conn.SetPongHandler 设置 pong 回调
		// 收到 pong 后重置读超时，保持连接活跃并续期 Redis 路由
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		c.hub.RenewUserLocation(c.Uuid)
		return nil
	})

	zap.L().Info("ws read goroutine start", zap.String("userId", c.Uuid)) // [第三方库: go.uber.org/zap] 结构化日志记录
	for {
		_, jsonMessage, err := c.Conn.ReadMessage() // [第三方库: github.com/gorilla/websocket] Conn.ReadMessage 读取 WebSocket 消息
		if err != nil {
			// 正常关闭（客户端主动断开）不记录 Error 级别日志
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) { // [第三方库: github.com/gorilla/websocket] 判断是否为非预期关闭
				zap.L().Error("ws read error", zap.String("userId", c.Uuid), zap.Error(err))
			} else {
				zap.L().Info("ws connection closed", zap.String("userId", c.Uuid), zap.Error(err))
			}
			return
		}
		zap.L().Debug("ws received message", zap.String("userId", c.Uuid), zap.String("message", string(jsonMessage)))
		c.hub.RenewUserLocation(c.Uuid)

		// 单管道架构：上行直调 message_service gRPC（同步鉴权、幂等、落库），失败立即报错回执
		var req messagepb.SendMessageRequest
		if err := json.Unmarshal(jsonMessage, &req); err != nil {
			zap.L().Error("unmarshal ws message error", zap.String("userId", c.Uuid), zap.Error(err))
			continue
		}
		// 强制覆盖 send_id 为当前 JWT 连接身份，防止越权伪造
		req.SendId = c.Uuid

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := grpc_client.SendMessage(ctx, &req)
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

		// 发送成功：通过安全队列返回即时 ACK 确认
		ackMsg, _ := json.Marshal(map[string]interface{}{
			"type":          "ack",
			"message_uuid":  resp.MessageUuid,
			"created_at":    resp.CreatedAt,
			"client_msg_id": req.ClientMsgId,
		})
		c.SafeSend(ackMsg, resp.MessageUuid)
	}
}

// cleanup 释放 UserConn 占用的资源（通过 sync.Once 确保只执行一次）
// 调用顺序：注销客户端 → 安全关闭通道 → 关闭 WebSocket 连接
func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		zap.L().Info("ws cleanup start", zap.String("userId", c.Uuid))

		// 指标埋点：断开连接
		metrics.OnlineConnections.Dec()
		metrics.TotalDisconnections.Inc()

		// 1. 从 Hub 注销，防止新消息写入 SendBack
		c.hub.UnregisterClient(c)

		// 2. 安全关闭 SendBack 通道（让 Write goroutine 退出 range 循环）
		// 因为 cleanup() 被 sync.Once 包裹，它本身就保证了只执行一次，
		// 所以底层必定只会 close(SendBack) 一次，绝对不会触发 "close of closed channel" 的 panic
		// 因此这里不需要任何额外的 recover() 操作了。
		close(c.SendBack)

		// 3. 关闭 WebSocket 连接
		if err := c.Conn.Close(); err != nil {
			zap.L().Warn("ws close connection error", zap.String("userId", c.Uuid), zap.Error(err))
		}

		zap.L().Info("ws cleanup done", zap.String("userId", c.Uuid))
	})
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket 客户端
// 同时定期发送 Ping 帧维持心跳，检测静默断连的客户端
func (c *UserConn) Write() {
	ticker := time.NewTicker(pingPeriod) // [标准库: time] 定时器用于心跳 Ping
	defer ticker.Stop()

	zap.L().Info("ws write goroutine start", zap.String("userId", c.Uuid))
	for {
		select {
		case messageBack, ok := <-c.SendBack:
			if !ok {
				// SendBack 通道已关闭（由 cleanup 触发），发送 Close 帧后退出
				_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{}) // [第三方库: github.com/gorilla/websocket] 发送 Close 帧
				zap.L().Info("ws write goroutine exit: SendBack closed", zap.String("userId", c.Uuid))
				return
			}

			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message); err != nil { // [第三方库: github.com/gorilla/websocket] 发送文本消息帧
				zap.L().Error("ws write error", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}


		case <-ticker.C:
			// 定期发送 Ping 帧，检测客户端是否还在线，并续期 Redis 路由
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil { // [第三方库: github.com/gorilla/websocket] 发送 Ping 心跳帧
				zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}
			c.hub.RenewUserLocation(c.Uuid)
		}
	}
}

// NewClientInit 当接受到前端有登录消息时，会调用该函数
// hub: 网关长连接生命周期中心，通过依赖注入传入
func NewClientInit(c *gin.Context, clientId string, hub *ClientHub) { // [第三方库: github.com/gin-gonic/gin] gin.Context 为 HTTP 请求上下文
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil) // [第三方库: github.com/gorilla/websocket] Upgrader.Upgrade 将 HTTP 升级为 WebSocket
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return
	}
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
		hub:      hub,
	}
	// 注册到网关在线连接池并登记 Redis 路由
	hub.RegisterClient(client)

	// 指标埋点：新连接
	metrics.OnlineConnections.Inc()
	metrics.TotalConnections.Inc()

	go client.Read()
	go client.Write()
	zap.L().Info("ws连接成功")
}

// ClientLogout 当接受到前端有登出消息时，会调用该函数
// 通过 cleanup 统一释放资源（sync.Once 保证幂等）
func ClientLogout(clientId string, hub *ClientHub) error {
	client := hub.GetClient(clientId)
	if client != nil {
		client.cleanup()
	}
	return nil
}
