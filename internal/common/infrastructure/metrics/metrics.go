// Package metrics 提供 Prometheus 指标定义和注册
// 核心职责：定义聊天服务器的所有可观测性指标
//
// 指标分类：
// 1. 连接指标：在线 WebSocket 连接数
// 2. 消息指标：消息吞吐量（按类型/方向）、消息处理延迟
// 3. Kafka 指标：消费延迟、生产失败数
// 4. Redis 指标：缓存命中率/未命中率
// 5. HTTP 指标：请求延迟、状态码分布
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// 所有指标变量在 init() 中自动注册到 Prometheus 全局 Registerer

var (
	// ===== 连接指标 =====

	// OnlineConnections 当前在线 WebSocket 连接数（Gauge，实时值）
	OnlineConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "kama_chat",
		Subsystem: "connection",
		Name:      "online_total",
		Help:      "当前在线的 WebSocket 连接数",
	})

	// TotalConnections 累计连接数（Counter，只增不减）
	TotalConnections = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "connection",
		Name:      "total_connections",
		Help:      "累计 WebSocket 连接数（含历史断开重连）",
	})

	// TotalDisconnections 累计断开连接数
	TotalDisconnections = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "connection",
		Name:      "total_disconnections",
		Help:      "累计 WebSocket 断开连接数",
	})

	// ===== 消息指标 =====

	// MessagesPublished 发布到 Kafka 的消息总数，按消息类型分类
	MessagesPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "published_total",
		Help:      "发布到 Kafka 的消息总数",
	}, []string{"type"}) // type: text, file, audio_video

	// MessagesConsumed 从 Kafka 消费的消息总数，按消息类型分类
	MessagesConsumed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "consumed_total",
		Help:      "从 Kafka 消费的消息总数",
	}, []string{"type"}) // type: text, file, audio_video

	// MessagesDuplicated 被去重跳过的消息数
	MessagesDuplicated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "duplicated_total",
		Help:      "被幂等去重跳过的消息数",
	})

	// MessagesDegrade Redis 幂等检查失败降级放行的消息数
	MessagesDegrade = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "degrade_total",
		Help:      "Redis 幂等检查失败降级放行的消息数（依赖 MySQL 唯一索引兜底）",
	})

	// MessagesDispatched 成功分发到客户端的消息数
	MessagesDispatched = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "dispatched_total",
		Help:      "成功分发到 WebSocket 客户端的消息数",
	})

	// MessagesDropped 因 channel 满而丢弃的消息数
	MessagesDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "dropped_total",
		Help:      "因 SendBack channel 满而丢弃的消息数",
	})

	// MessageProcessDuration 消息处理耗时直方图（从消费到分发完成）
	MessageProcessDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "process_duration_seconds",
		Help:      "消息处理耗时（秒）",
		Buckets:   prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
	}, []string{"type"}) // type: text, file, audio_video

	// PublishDuration 消息发布到 Kafka 的耗时
	PublishDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "kama_chat",
		Subsystem: "message",
		Name:      "publish_duration_seconds",
		Help:      "消息发布到 Kafka 的耗时（秒）",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})

	// ===== Kafka 指标 =====

	// KafkaProduceErrors 生产者写入失败数
	KafkaProduceErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "kafka",
		Name:      "produce_errors_total",
		Help:      "Kafka 生产者写入失败次数",
	})

	// KafkaConsumeErrors 消费者读取失败数
	KafkaConsumeErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "kafka",
		Name:      "consume_errors_total",
		Help:      "Kafka 消费者读取失败次数",
	})

	// ===== Redis 指标 =====

	// CacheHits 缓存命中次数
	CacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "cache",
		Name:      "hits_total",
		Help:      "Redis 缓存命中次数",
	})

	// CacheMisses 缓存未命中次数
	CacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "cache",
		Name:      "misses_total",
		Help:      "Redis 缓存未命中次数",
	})

	// CacheErrors 缓存操作失败次数
	CacheErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kama_chat",
		Subsystem: "cache",
		Name:      "errors_total",
		Help:      "Redis 缓存操作失败次数",
	})

	// ===== HTTP 指标 =====
	// 注：HTTP 请求指标由 promhttp.InstrumentHandler 自动收集，无需手动定义
)

// init 自动注册所有指标到 Prometheus 全局 Registerer
func init() {
	prometheus.MustRegister(
		// 连接指标
		OnlineConnections,
		TotalConnections,
		TotalDisconnections,
		// 消息指标
		MessagesPublished,
		MessagesConsumed,
		MessagesDuplicated,
		MessagesDegrade,
		MessagesDispatched,
		MessagesDropped,
		MessageProcessDuration,
		PublishDuration,
		// Kafka 指标
		KafkaProduceErrors,
		KafkaConsumeErrors,
		// Redis 指标
		CacheHits,
		CacheMisses,
		CacheErrors,
	)
}
