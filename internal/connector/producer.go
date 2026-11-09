package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	messagepb "kama_chat_server/api/gen/message"
	kafkainfra "kama_chat_server/internal/infra/kafka"
	otelx "kama_chat_server/pkg/otel"
)

// IncomingMessageProducer 上行聊天消息异步削峰生产者接口
type IncomingMessageProducer interface {
	PublishIncomingMessage(ctx context.Context, req *messagepb.SendMessageRequest) error
	Close()
}

// ResolveSessionPartitionKey 计算会话级物理分区路由键（与 message.ResolveSessionId 严格对齐）：
// 1. 群聊：统一使用 groupId（G开头），确保同一群内所有成员发言进入同一 Partition；
// 2. 单聊：按字典序组合双方 UUID 为 P_{min}_{max}，确保双方双向对话严格落入同一 Partition 顺序消费。
func ResolveSessionPartitionKey(sendId, receiveId string) string {
	if strings.HasPrefix(receiveId, "G") {
		return receiveId
	}
	if sendId > receiveId {
		sendId, receiveId = receiveId, sendId
	}
	return fmt.Sprintf("P_%s_%s", sendId, receiveId)
}

// KafkaIncomingMessageProducer 基于 franz-go 幂等生产者实现的上行聊天消息投递器
type KafkaIncomingMessageProducer struct {
	client *kgo.Client
}

// NewKafkaIncomingMessageProducer 初始化写入 im_chat_messages_incoming 主题的 Kafka 生产者
func NewKafkaIncomingMessageProducer() *KafkaIncomingMessageProducer {
	client, err := kafkainfra.NewProducer(kafkainfra.TopicChatMessagesIncoming)
	if err != nil {
		zap.L().Fatal("failed to init kafka incoming message producer", zap.Error(err))
	}
	return &KafkaIncomingMessageProducer{client: client}
}

// PublishIncomingMessage 将上行聊天消息序列化为 Protobuf 并同步等待 Kafka ISR 副本确认（ProduceSync）。
// 若 Kafka 投递超时或失败则快速返回错误（Fail-fast），杜绝流量旁路穿透压垮数据库。
func (p *KafkaIncomingMessageProducer) PublishIncomingMessage(ctx context.Context, req *messagepb.SendMessageRequest) error {
	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	partitionKey := ResolveSessionPartitionKey(req.SendId, req.ReceiveId)
	var headers []kgo.RecordHeader
	otelx.InjectTraceContext(ctx, &headers)
	return kafkainfra.Publish(ctx, p.client, []byte(partitionKey), data, headers)
}

// Close 关闭底层 Kafka 生产者客户端
func (p *KafkaIncomingMessageProducer) Close() {
	if p.client != nil {
		p.client.Close()
	}
}
