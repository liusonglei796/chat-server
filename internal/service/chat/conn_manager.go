// Package chat 实现了聊天系统的核心服务层
// conn_manager.go
// 核心职责：WebSocket 连接生命周期管理
// 1. 建立 WebSocket 连接 (Upgrade)
// 2. 封装 Client 对象，管理读写协程 (Read/Write Loop)
// 3. 作为“一线柜员”，直接对接前端：接收消息 -> 投递到后端(Kafka/Channel)；从后端接收 -> 推送给前端
package chat

import (
	"context"
	"encoding/json"
	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type MessageBack struct {
	Message []byte
	Uuid    int64
}

type UserConn struct {
	Conn     *websocket.Conn
	Uuid     string
	SendTo   chan []byte       // 给server端
	SendBack chan *MessageBack // 给前端
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	// 检查连接的Origin头
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var ctx = context.Background()

var messageMode = config.GetConfig().KafkaConfig.MessageMode

// 读取websocket消息并发送给send通道
// Kafka Producer (生产者)前端用户发来消息 -> 读协程收到 -> 调用 GlobalKafkaClient.WriteMessage -> 发给 Kafka。
func (c *UserConn) Read() {
	zap.L().Info("ws read goroutine start")
	for {
		// 阻塞有一定隐患，因为下面要处理缓冲的逻辑，但是可以先不做优化，问题不大
		_, jsonMessage, err := c.Conn.ReadMessage() // 阻塞状态
		if err != nil {
			zap.L().Error(err.Error())
			return // 直接断开websocket
		} else {
			var message = request.ChatMessageRequest{}
			if err := json.Unmarshal(jsonMessage, &message); err != nil {
				zap.L().Error(err.Error())
			}
			log.Println("接受到消息为: ", jsonMessage)
			if messageMode == "channel" {
				// 如果server的转发channel没满，先把sendto中的给transmit
				for len(GlobalStandaloneServer.Transmit) < constants.CHANNEL_SIZE && len(c.SendTo) > 0 {
					sendToMessage := <-c.SendTo
					GlobalStandaloneServer.SendMessageToTransmit(sendToMessage)
				}
				// 如果server没满，sendto空了，直接给server的transmit
				if len(GlobalStandaloneServer.Transmit) < constants.CHANNEL_SIZE {
					GlobalStandaloneServer.SendMessageToTransmit(jsonMessage)
				} else if len(c.SendTo) < constants.CHANNEL_SIZE {
					// 如果server满了，直接塞sendto
					c.SendTo <- jsonMessage
				} else {
					// 否则考虑加宽channel size，或者使用kafka
					if err := c.Conn.WriteMessage(websocket.TextMessage, []byte("由于目前同一时间过多用户发送消息，消息发送失败，请稍后重试")); err != nil {
						zap.L().Error(err.Error())
					}
				}
			} else {
				// 强耦合：直接使用 chat.GlobalKafkaClient
				key := []byte(strconv.Itoa(config.GetConfig().KafkaConfig.Partition))
				if err := GlobalKafkaClient.WriteMessage(ctx, key, jsonMessage); err != nil {
					zap.L().Error(err.Error())
				}
				zap.L().Info("已发送消息：" + string(jsonMessage))
			}
		}
	}
}

// 从send通道读取消息发送给websocket
func (c *UserConn) Write() {
	zap.L().Info("ws write goroutine start")
	for messageBack := range c.SendBack { // 阻塞状态
		// 通过 WebSocket 发送消息
		err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message)
		if err != nil {
			zap.L().Error(err.Error())
			return // 直接断开websocket
		}
		// log.Println("已发送消息：", messageBack.Message)
		// 说明顺利发送，修改状态为已发送
		if res := dao.GormDB.Model(&model.Message{}).Where("uuid = ?", messageBack.Uuid).Update("status", message_status_enum.Sent); res.Error != nil {
			zap.L().Error(res.Error.Error())
		}
	}
}

// NewClientInit 当接受到前端有登录消息时，会调用该函数
func NewClientInit(c *gin.Context, clientId string) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error(err.Error())
	}
	client := &UserConn{
		Conn:     conn,
		Uuid:     clientId,
		SendTo:   make(chan []byte, constants.CHANNEL_SIZE),
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE),
	}
	// 强耦合：根据 messageMode 直接调用对应的 Server
	if messageMode == "kafka" {
		GlobalMsgConsumer.SendClientToLogin(client)
	} else {
		GlobalStandaloneServer.SendClientToLogin(client)
	}
	go client.Read()
	go client.Write()
	zap.L().Info("ws连接成功")
}

// ClientLogout 当接受到前端有登出消息时，会调用该函数
func ClientLogout(clientId string) error {
	// 强耦合：根据 messageMode 直接调用对应的 Server
	if messageMode == "kafka" {
		client := GlobalMsgConsumer.GetClient(clientId)
		if client != nil {
			GlobalMsgConsumer.SendClientToLogout(client)
			if err := client.Conn.Close(); err != nil {
				zap.L().Error(err.Error())
				return err
			}
			close(client.SendTo)
			close(client.SendBack)
		}
	} else {
		client := GlobalStandaloneServer.GetClient(clientId)
		if client != nil {
			GlobalStandaloneServer.SendClientToLogout(client)
			if err := client.Conn.Close(); err != nil {
				zap.L().Error(err.Error())
				return err
			}
			close(client.SendTo)
			close(client.SendBack)
		}
	}
	return nil
}
