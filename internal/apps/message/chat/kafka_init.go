package chat

import (
	myconfig "kama_chat_server/internal/common/config"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type KafkaClient struct {
	Producer *kafka.Writer
	Consumer *kafka.Reader
}

func NewKafkaClient() *KafkaClient {
	return &KafkaClient{}
}

func (k *KafkaClient) KafkaInit() {
	kafkaConfig := myconfig.GetConfig().KafkaConfig

	// 生产者：写入上行消息到 chat_upstream
	k.Producer = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  "chat_upstream",
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: true,
	}

	// 消费者：从 chat_downstream 读取下行消息
	k.Consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          "chat_downstream",
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "chat_server",
		StartOffset:    kafka.LastOffset,
	})
}

func (k *KafkaClient) KafkaClose() {
	if err := k.Producer.Close(); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	if err := k.Consumer.Close(); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
}
