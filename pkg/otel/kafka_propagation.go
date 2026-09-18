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
