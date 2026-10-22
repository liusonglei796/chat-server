// kafka_propagation.go
// Kafka trace context propagation utilities for OpenTelemetry.
//
// Provides functions to inject and extract distributed trace context
// into/from Kafka message headers, enabling end-to-end tracing across
// the Kafka message pipeline.
package otel

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// W3C Trace Context 标准 header 名
const (
	TraceParentKey = "traceparent"
	TraceStateKey  = "tracestate"
)

// KafkaHeaderCarrier implements propagation.TextMapCarrier for Kafka message headers.
// It wraps a pointer to a slice of kgo.RecordHeader so that the Inject operation
// can append new headers to the original slice.
type KafkaHeaderCarrier struct {
	Headers *[]kgo.RecordHeader
}

// Get returns the value for a given header key.
// Implements propagation.TextMapCarrier interface.
func (c KafkaHeaderCarrier) Get(key string) string {
	if c.Headers == nil {
		return ""
	}
	for _, h := range *c.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// Set sets the value for a given header key, appending a new header if not present.
// Implements propagation.TextMapCarrier interface.
func (c KafkaHeaderCarrier) Set(key string, value string) {
	if c.Headers == nil {
		return
	}
	*c.Headers = append(*c.Headers, kgo.RecordHeader{
		Key:   key,
		Value: []byte(value),
	})
}

// Keys returns all header keys currently present.
// Implements propagation.TextMapCarrier interface.
func (c KafkaHeaderCarrier) Keys() []string {
	if c.Headers == nil {
		return nil
	}
	keys := make([]string, 0, len(*c.Headers))
	for _, h := range *c.Headers {
		keys = append(keys, h.Key)
	}
	return keys
}

// InjectTraceContext injects the current trace context from ctx into Kafka message headers.
//
// Usage:
//
//	var headers []kgo.RecordHeader
//	otel.InjectTraceContext(ctx, &headers)
//	err := kafkainfra.Publish(ctx, producer, key, value, headers)
func InjectTraceContext(ctx context.Context, headers *[]kgo.RecordHeader) {
	otel.GetTextMapPropagator().Inject(ctx, KafkaHeaderCarrier{Headers: headers})
}

// ExtractTraceContext extracts trace context from Kafka message headers into a context.
//
// Usage:
//
//	ctx := otel.ExtractTraceContext(context.Background(), kafkaRecord.Headers)
//	// use ctx for downstream processing with propagated trace context
func ExtractTraceContext(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, KafkaHeaderCarrier{Headers: &headers})
}

// mapCarrier 是 map 形态的 propagation.TextMapCarrier，用于把 trace context 序列化为字符串
type mapCarrier map[string]string

// Get 实现 propagation.TextMapCarrier
func (c mapCarrier) Get(key string) string { return c[key] }

// Set 实现 propagation.TextMapCarrier
func (c mapCarrier) Set(key, value string) { c[key] = value }

// Keys 实现 propagation.TextMapCarrier
func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// CaptureTraceContext 从 ctx 中捕获 W3C trace context（traceparent / tracestate）。
//
// 用于随领域事件一起落库：事件先写 outbox 表（同一业务事务），再由 CDC 投递 Kafka，
// 消费侧据此把 span 挂回原始业务请求的 trace 上，实现跨 MQ 的端到端链路追踪。
//
// ctx 中没有有效 trace 时返回两个空字符串。
func CaptureTraceContext(ctx context.Context) (traceParent, traceState string) {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return "", ""
	}
	carrier := mapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier[TraceParentKey], carrier[TraceStateKey]
}

// RestoreTraceContext 把落库的 trace context 还原到 ctx 中，作为后续 span 的父上下文。
// traceParent 为空（事件写入时无有效 trace）时原样返回 ctx。
func RestoreTraceContext(ctx context.Context, traceParent, traceState string) context.Context {
	if traceParent == "" {
		return ctx
	}
	carrier := mapCarrier{TraceParentKey: traceParent}
	if traceState != "" {
		carrier[TraceStateKey] = traceState
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// StartKafkaProducerSpan 创建 producer span，并把 trace context 注入到消息头。
// 调用方负责调用 span.End()。
func StartKafkaProducerSpan(ctx context.Context, headers *[]kgo.RecordHeader, operation string) (context.Context, trace.Span) {
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	ctx, span := tracer.Start(ctx, operation, trace.WithSpanKind(trace.SpanKindProducer))
	InjectTraceContext(ctx, headers)
	return ctx, span
}

// StartKafkaConsumerSpan 从消息头提取上游 trace context，并以此为父创建 consumer span。
// 消息头里没有 trace context 时，ctx 退化为一条无父的新 trace。
// 调用方负责调用 span.End()。
func StartKafkaConsumerSpan(ctx context.Context, headers []kgo.RecordHeader, operation string) (context.Context, trace.Span) {
	ctx = ExtractTraceContext(ctx, headers)
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	return tracer.Start(ctx, operation, trace.WithSpanKind(trace.SpanKindConsumer))
}

// EndSpan 结束 span；err 非 nil 时同时记录错误并把 span 状态置为 Error。
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
