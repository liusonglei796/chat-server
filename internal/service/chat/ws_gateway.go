// Package chat 实现了聊天系统的核心服务层
// ws_gateway.go
// 核心职责：WebSocket 连接生命周期管理
// 1. 建立 WebSocket 连接 (Upgrade)
// 2. 封装 Client 对象，管理读写协程 (Read/Write Loop)
// 3. 通过 MessageBroker 接口解耦消息投递逻辑
package chat

import (
	"context"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MessageBack 用于回传消息给前端
type MessageBack struct {
	Message []byte
	Uuid    int64
}

// UserConn 表示一个 WebSocket 客户端连接
// 代表的是你的后端服务器和用户浏览器之间的一条那根网线。
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

// Read 从 WebSocket 读取消息并通过 Broker 发布
func (c *UserConn) Read() {
	zap.L().Info("ws read goroutine start")
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			zap.L().Error(err.Error())
			return
		}
		log.Println("接受到消息为: ", string(jsonMessage))
		// 通过接口发布消息，不关心具体实现
		if err := c.broker.Publish(ctx, jsonMessage); err != nil {
			zap.L().Error(err.Error())
		}
	}
}

// Write 从 SendBack 通道读取消息并发送给 WebSocket
func (c *UserConn) Write() {
	zap.L().Info("ws write goroutine start")
	//只要传送带上有消息，我就拿出来发送；如果没有消息，我就在这里等着。
	for messageBack := range c.SendBack {
		err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message)
		if err != nil {
			zap.L().Error(err.Error())
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
		zap.L().Error(err.Error())
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
			zap.L().Error(err.Error())
			return err
		}
		close(client.SendTo)
		close(client.SendBack)
	}
	return nil
}
