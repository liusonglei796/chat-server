package chat

import (
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	kafkainfra "kama_chat_server/internal/common/infrastructure/kafka"
)

type KafkaClient struct {
	Producer *kafka.Writer
	Consumer *kafka.Reader
}

func NewKafkaClient() *KafkaClient {
	return &KafkaClient{}
}

func (k *KafkaClient) KafkaInit() {
	// 生产者：写入上行消息到 chat_upstream
	k.Producer = kafkainfra.NewProducer(kafkainfra.TopicChatUpstream)

	// 消费者：从 chat_downstream 读取下行消息
	k.Consumer = kafkainfra.NewConsumer(kafkainfra.TopicChatDownstream, "chat_server")
}

func (k *KafkaClient) KafkaClose() {
	if err := k.Producer.Close(); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	if err := k.Consumer.Close(); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
}
