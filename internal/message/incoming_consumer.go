package message

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	messagepb "kama_chat_server/api/gen/message"
	kafkainfra "kama_chat_server/internal/infra/kafka"
	metricsinfra "kama_chat_server/internal/infra/metrics"
	"kama_chat_server/pkg/errorx"
	otelx "kama_chat_server/pkg/otel"
)

const incomingConsumerGroupID = "message_service_incoming_group"

// IncomingMessageConsumer 消费 im_chat_messages_incoming 主题，
// 按会话分区（Key = session_id）严格串行落库并触发方案二双向推流（含回声确认 Echo ACK）。
type IncomingMessageConsumer struct {
	reader  *kafkainfra.Consumer
	service *MessageService
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewIncomingMessageConsumer 构造上行聊天消息 Kafka 消费者
func NewIncomingMessageConsumer(service *MessageService) *IncomingMessageConsumer {
	reader, err := kafkainfra.NewConsumer(kafkainfra.TopicChatMessagesIncoming, incomingConsumerGroupID)
	if err != nil {
		zap.L().Fatal("failed to init kafka incoming message consumer", zap.Error(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &IncomingMessageConsumer{
		reader:  reader,
		service: service,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// isNonRetryableBusinessError 判断是否为不可重试的确定性业务错误（如参数非法、被拉黑拒收、非好友、账号禁用）
func isNonRetryableBusinessError(err error) bool {
	if err == nil {
		return false
	}
	if errorx.IsCode(err, errorx.CodeInvalidParam) ||
		errorx.IsCode(err, errorx.CodeForbidden) ||
		errorx.IsCode(err, errorx.CodeUnauthorized) ||
		errorx.IsCode(err, errorx.CodeNotFound) {
		return true
	}
	if se := kerrors.FromError(err); se != nil {
		if se.Code == 400 || se.Code == 401 || se.Code == 403 || se.Code == 404 {
			return true
		}
	}
	return false
}

// ProcessRecord 处理单条上行聊天消息 Record：
// 1. Protobuf 反序列化为 SendMessageRequest
// 2. 调用 MessageService.SendMessage 串行分配 seq_id、落库并触发双向推流（Echo ACK）
// 3. 若遇确定性业务拒绝（如被拉黑），向发送方回推 error 帧并放行位点；若遇瞬时错误则有限重试
func (c *IncomingMessageConsumer) ProcessRecord(ctx context.Context, msg *kgo.Record) error {
	handleCtx, span := otelx.StartKafkaConsumerSpan(ctx, msg.Headers, "kafka consume incoming_message")

	var pbReq messagepb.SendMessageRequest
	if err := proto.Unmarshal(msg.Value, &pbReq); err != nil {
		otelx.EndSpan(span, err)
		metricsinfra.KafkaConsumeErrors.Inc()
		zap.L().Error("unmarshal incoming chat message proto error", zap.Error(err))
		return nil // 毒丸消息直接跳过，防止阻塞整个会话分区
	}

	chatReq := ChatMessageRequest{
		SessionId:   pbReq.SessionId,
		Type:        int8(pbReq.Type),
		Content:     pbReq.Content,
		Url:         pbReq.Url,
		SendId:      pbReq.SendId,
		SendName:    pbReq.SendName,
		SendAvatar:  pbReq.SendAvatar,
		ReceiveId:   pbReq.ReceiveId,
		FileSize:    pbReq.FileSize,
		FileType:    pbReq.FileType,
		FileName:    pbReq.FileName,
		ClientMsgId: pbReq.ClientMsgId,
	}

	var execErr error
	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		execCtx, cancel := context.WithTimeout(handleCtx, 10*time.Second)
		_, execErr = c.service.SendMessage(execCtx, chatReq)
		cancel()

		if execErr == nil {
			break
		}
		if isNonRetryableBusinessError(execErr) {
			break
		}
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				otelx.EndSpan(span, ctx.Err())
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	otelx.EndSpan(span, execErr)
	if execErr != nil {
		if !isNonRetryableBusinessError(execErr) {
			metricsinfra.KafkaConsumeErrors.Inc()
			zap.L().Error("consume incoming message failed after retries",
				zap.String("sendId", pbReq.SendId),
				zap.String("receiveId", pbReq.ReceiveId),
				zap.String("clientMsgId", pbReq.ClientMsgId),
				zap.Error(execErr),
			)
		} else {
			zap.L().Info("incoming message rejected by business rule",
				zap.String("sendId", pbReq.SendId),
				zap.String("receiveId", pbReq.ReceiveId),
				zap.String("clientMsgId", pbReq.ClientMsgId),
				zap.Error(execErr),
			)
		}
		// 向发送方回推错误通知帧，便于客户端消除转圈或感知拒收原因
		if pbReq.SendId != "" {
			errPayload, _ := json.Marshal(map[string]interface{}{
				"type":          "error",
				"client_msg_id": pbReq.ClientMsgId,
				"message":       execErr.Error(),
			})
			c.service.trySendBack(pbReq.SendId, "", errPayload)
		}
		return execErr
	}
	return nil
}

// Start 启动后台常驻消费协程（单分区内严格串行处理并后置同步提交位点）
func (c *IncomingMessageConsumer) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("incoming message consumer panic: %v", r))
			}
		}()
		for {
			if c.ctx.Err() != nil {
				return
			}
			msg, err := c.reader.ReadRecord(c.ctx)
			if err != nil {
				if kafkainfra.IsClosed(err) || c.ctx.Err() != nil {
					return
				}
				zap.L().Error("read incoming message record error", zap.Error(err))
				continue
			}

			_ = c.ProcessRecord(c.ctx, msg)

			if err := c.reader.CommitRecord(c.ctx, msg); err != nil {
				if kafkainfra.IsClosed(err) || c.ctx.Err() != nil {
					return
				}
				zap.L().Error("commit kafka offset for incoming message error", zap.Error(err))
			}
		}
	}()
}

// Close 优雅停止消费者并等待在途消息处理完毕
func (c *IncomingMessageConsumer) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.reader != nil {
		c.reader.Close()
	}
	c.wg.Wait()
}
