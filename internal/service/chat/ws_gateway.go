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
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"kama_chat_server/pkg/errorx"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
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
//	有了 WebSocket： 用户 B 和服务器之间建立了一根“长管子”。服务器一旦收到 A 发给 B 的消息，不需要等 B 询问，直接顺着这根管子把消息“推”到 B 的屏幕上。
type UserConn struct {
	Conn     *websocket.Conn
	Uuid     string
	SendTo   chan []byte       // 缓冲通道（Channel 模式备用）
	SendBack chan *MessageBack // 给前端
	broker   MessageBroker     // 注入的消息代理
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

var ctx = context.Background()

// Read 从 WebSocket 读取消息
// 安全: 服务端会用连接时认证的用户 ID 覆盖消息中的 SendId，防止 IDOR 攻击
func (c *UserConn) Read() {
	zap.L().Info("ws read goroutine start")
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return
		}
		log.Println("接受到消息为: ", string(jsonMessage))

		// 安全: 注入真实的用户 ID，覆盖客户端传入的 send_id（防止 IDOR）
		securedMessage := injectRealSenderId(jsonMessage, c.Uuid)

		// 通过接口发布消息，不关心具体实现
		if err := c.broker.Publish(ctx, securedMessage); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	}
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

// Write 从 SendBack 通道读取消息并发送给 WebSocket
func (c *UserConn) Write() {
	zap.L().Info("ws write goroutine start")
	//只要传送带上有消息，我就拿出来发送；如果没有消息，我就在这里等着。
	for messageBack := range c.SendBack {
		err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return
		}
		// 通过 Repository 接口更新消息状态（遵循依赖倒置原则）
		if repo := c.broker.GetMessageRepo(); repo != nil {
			if err := repo.UpdateStatus(messageBack.Uuid, message_status_enum.Sent); err != nil {
				zap.L().Error("更新消息状态失败", zap.Error(err))
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
// broker: 消息代理实例，通过依赖注入传入
func ClientLogout(clientId string, broker MessageBroker) error {
	client := broker.GetClient(clientId)
	if client != nil {
		broker.UnregisterClient(client)
		if err := client.Conn.Close(); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		close(client.SendTo)
		close(client.SendBack)
	}
	return nil
}
