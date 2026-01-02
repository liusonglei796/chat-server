package mq

import (
	"context"
	myconfig "kama_chat_server/internal/config"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var ctx = context.Background()

type kafkaService struct {
	ChatWriter *kafka.Writer
	ChatReader *kafka.Reader
	KafkaConn  *kafka.Conn
}

var KafkaService = new(kafkaService)

// KafkaInit 初始化kafka
func (k *kafkaService) KafkaInit() {
	//k.CreateTopic()
	kafkaConfig := myconfig.GetConfig().KafkaConfig
	k.ChatWriter = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  kafkaConfig.ChatTopic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: false,
	}
	k.ChatReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          kafkaConfig.ChatTopic,
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "chat",
		StartOffset:    kafka.LastOffset,
	})
}

func (k *kafkaService) KafkaClose() {
	if err := k.ChatWriter.Close(); err != nil {
		zap.L().Error(err.Error())
	}
	if err := k.ChatReader.Close(); err != nil {
		zap.L().Error(err.Error())
	}
}

// CreateTopic 创建topic
func (k *kafkaService) CreateTopic() {
	// 如果已经有topic了，就不创建了
	kafkaConfig := myconfig.GetConfig().KafkaConfig

	chatTopic := kafkaConfig.ChatTopic

	// 连接至任意kafka节点
	var err error
	k.KafkaConn, err = kafka.Dial("tcp", kafkaConfig.HostPort)
	if err != nil {
		zap.L().Error(err.Error())
	}

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             chatTopic,
			NumPartitions:     kafkaConfig.Partition,
			ReplicationFactor: 1,
		},
	}

	// 创建topic
	if err = k.KafkaConn.CreateTopics(topicConfigs...); err != nil {
		zap.L().Error(err.Error())
	}

}

// WriteMessage 实现 websocket.MessageWriter 接口
// 用于向 Kafka 发送消息
func (k *kafkaService) WriteMessage(ctx context.Context, key, value []byte) error {
	return k.ChatWriter.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}
