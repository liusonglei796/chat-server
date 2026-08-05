package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"kama_chat_server/internal/common/dto/event"
	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/metrics"
	"kama_chat_server/pkg/otel"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
	gootel "go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type MsgConsumer struct {
	Clients     sync.Map
	Login       chan *UserConn
	Logout      chan *UserConn
	closeOnce   sync.Once
	kafkaClient *KafkaClient
	quit        chan os.Signal
}

func NewMsgConsumer(kafkaClient *KafkaClient) *MsgConsumer {
	return &MsgConsumer{
		Login:       make(chan *UserConn),
		Logout:      make(chan *UserConn),
		kafkaClient: kafkaClient,
		quit:        make(chan os.Signal, 1),
	}
}

func (k *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
	start := time.Now()
	defer func() {
		metrics.PublishDuration.Observe(time.Since(start).Seconds())
	}()

	// 创建 producer span，作为消息链路在 chat_server 侧的根
	// 使用全局 TracerProvider：WS read 循环传入的 ctx 无 span，SpanFromContext 会返回 noop provider
	tracer := gootel.GetTracerProvider().Tracer("kama_chat_server/internal/apps/message/chat")
	ctx, span := tracer.Start(ctx, "kafka.publish")
	defer span.End()

	key := []byte("0")
	var req struct {
		SendId    string `json:"send_id"`
		ReceiveId string `json:"receive_id"`
		Type      int    `json:"type"`
	}
	if err := json.Unmarshal(msg, &req); err == nil && req.ReceiveId != "" {
		if req.ReceiveId[0] == 'U' {
			id1, id2 := req.SendId, req.ReceiveId
			if id1 > id2 {
				id1, id2 = id2, id1
			}
			key = []byte(id1 + "_" + id2)
		} else if req.ReceiveId[0] == 'G' {
			key = []byte(req.ReceiveId)
		}
	}

	var headers []kafka.Header
	otel.InjectTraceContext(ctx, &headers)

	return kafkainfra.Publish(ctx, k.kafkaClient.Producer, key, msg, headers)
}

func (k *MsgConsumer) Start() {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
		}
		k.closeOnce.Do(func() {
			close(k.Login)
			close(k.Logout)
		})
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("kafka consumer panic: %v", r))
			}
		}()
		for {
			kafkaMessage, err := k.kafkaClient.Consumer.ReadMessage(context.Background())
			if err != nil {
				zap.L().Error("service error", zap.Error(err))
				metrics.KafkaConsumeErrors.Inc()
				continue
			}

			var pushEvent event.PushEvent
			if err := json.Unmarshal(kafkaMessage.Value, &pushEvent); err != nil {
				zap.L().Error("unmarshal push event error", zap.Error(err))
				continue
			}

			messageBack := &MessageBack{
				Message: pushEvent.Payload,
				Uuid:    pushEvent.MessageUuid,
			}

			if value, ok := k.Clients.Load(pushEvent.TargetUserId); ok {
				client := value.(*UserConn)
				trySendBack(client, messageBack)
			}
		}
	}()

	for {
		select {
		case client := <-k.Login:
			k.Clients.Store(client.Uuid, client)
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到kama聊天服务器")); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		case client := <-k.Logout:
			k.Clients.Delete(client.Uuid)
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		case <-k.quit:
			return
		}
	}
}

func (k *MsgConsumer) Close() {
	k.closeOnce.Do(func() {
		close(k.Login)
		close(k.Logout)
	})
}

func (k *MsgConsumer) GetClient(userId string) *UserConn {
	value, ok := k.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}

func (k *MsgConsumer) RegisterClient(client *UserConn) {
	k.Login <- client
}

func (k *MsgConsumer) UnregisterClient(client *UserConn) {
	k.Logout <- client
}

func trySendBack(client *UserConn, msg *MessageBack) {
	client.SendBack <- msg
}
