// Package kafka 统一封装 Kafka 基础设施：主题常量、生产者/消费者工厂、主题确保与消息收发。
// 全项目所有 Kafka 读写都必须经由本包，基于 twmb/franz-go 纯 Go 客户端实现原生生产者幂等（Idempotent Producer）。
package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"kama_chat_server/internal/common/config"
)

// 主题常量：全项目 Kafka 主题的唯一出处
const (
	TopicDomainEvents = "domain_events"
)

// NewProducer 创建写入指定主题的生产者。
// 默认开启原生生产者幂等机制（Idempotent Producer）：
//   - 内部自动维护 Producer ID (PID)、Epoch 与 Sequence Number
//   - RequiredAcks 默认为 RequireAll (acks=-1 / all)，确保消息写入所有 ISR 副本才返回成功
//   - 网络重试时由 Kafka Broker 端依据 PID+Seq 自动去重，杜绝重复消息
// 保证有序：使用 kgo.StickyKeyPartitioner(nil)，相同业务 Key 的消息严格路由到同一 Partition
func NewProducer(topic string) (*kgo.Client, error) {
	cfg := config.GetConfig().KafkaConfig
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.HostPort),
		kgo.DefaultProduceTopic(topic),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
	}
	if cfg.Timeout > 0 {
		opts = append(opts, kgo.RecordDeliveryTimeout(cfg.Timeout*time.Second))
	}
	return kgo.NewClient(opts...)
}

// Consumer 封装 franz-go 消费端，提供顺畅的按条读取接口和自动位点提交
type Consumer struct {
	client *kgo.Client
	iter   *kgo.FetchesRecordIter
}

// Client 获取底层的 *kgo.Client 实例
func (c *Consumer) Client() *kgo.Client {
	return c.client
}

// ReadRecord 读取下一条消息。内部维护 fetch 批次迭代器，批次耗尽时自动拉取下一批。
// 当 client 关闭或 context 取消时返回相应错误。
func (c *Consumer) ReadRecord(ctx context.Context) (*kgo.Record, error) {
	for {
		if c.iter != nil && !c.iter.Done() {
			return c.iter.Next(), nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil, kgo.ErrClientClosed
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			return nil, errs[0].Err
		}
		c.iter = fetches.RecordIter()
	}
}

// Close 关闭底层 Kafka 客户端
func (c *Consumer) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// IsClosed 判断错误是否由 Kafka 客户端关闭引起
func IsClosed(err error) bool {
	return errors.Is(err, kgo.ErrClientClosed)
}

// NewConsumer 创建消费指定主题的消费者（按消费组负载均衡）
func NewConsumer(topic, groupID string) (*Consumer, error) {
	cfg := config.GetConfig().KafkaConfig
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.HostPort),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
	}
	if cfg.Timeout > 0 {
		opts = append(opts, kgo.AutoCommitInterval(cfg.Timeout*time.Second))
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &Consumer{client: client}, nil
}

// EnsureTopic 确保主题已存在。
// Kafka 数据目录无持久卷，冷启动时主题可能尚未创建；
// 若消费者在主题创建前加入消费组，将被分配 0 个分区且不会自动重平衡。
// CreateTopics 对已存在的主题（kerr.TopicAlreadyExists）幂等，无副作用。
func EnsureTopic(ctx context.Context, topic string) error {
	cfg := config.GetConfig().KafkaConfig
	cl, err := kgo.NewClient(kgo.SeedBrokers(cfg.HostPort))
	if err != nil {
		return err
	}
	adm := kadm.NewClient(cl)
	defer adm.Close()

	resp, err := adm.CreateTopics(ctx, 3, 1, nil, topic)
	if err != nil {
		return err
	}
	for _, r := range resp {
		if r.Err != nil && !errors.Is(r.Err, kerr.TopicAlreadyExists) {
			return r.Err
		}
	}
	return nil
}

// Publish 发送一条消息到指定主题（若 Producer 配置了默认主题，可直接投递）。
// key 用于分区路由（同一 key 的消息进同一分区，保证有序）；
// headers 透传元数据（如 trace context、事件类型）。nil 表示无头。
func Publish(ctx context.Context, cl *kgo.Client, key []byte, payload []byte, headers []kgo.RecordHeader) error {
	record := &kgo.Record{
		Key:     key,
		Value:   payload,
		Headers: headers,
	}
	return cl.ProduceSync(ctx, record).FirstErr()
}

// PublishToTopic 发送一条消息到显式指定的主题
func PublishToTopic(ctx context.Context, cl *kgo.Client, topic string, key []byte, payload []byte, headers []kgo.RecordHeader) error {
	record := &kgo.Record{
		Topic:   topic,
		Key:     key,
		Value:   payload,
		Headers: headers,
	}
	return cl.ProduceSync(ctx, record).FirstErr()
}
