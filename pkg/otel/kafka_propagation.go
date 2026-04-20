// kafka_propagation.go
// Kafka trace context propagation utilities for OpenTelemetry.
//
// Provides functions to inject and extract distributed trace context
// into/from Kafka message headers, enabling end-to-end tracing across
// the Kafka message pipeline.
package otel

import (
	"context"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
)

// KafkaHeaderCarrier implements propagation.TextMapCarrier for Kafka message headers.
// It wraps a pointer to a slice of kafka.Header so that the Inject operation
// can append new headers to the original slice.
type KafkaHeaderCarrier struct {
	Headers *[]kafka.Header
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
	*c.Headers = append(*c.Headers, kafka.Header{
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
//	var headers []kafka.Header
//	otel.InjectTraceContext(ctx, &headers)
//	err := producer.WriteMessages(ctx, kafka.Message{
//	    Key:     key,
//	    Value:   value,
//	    Headers: headers,
//	})
func InjectTraceContext(ctx context.Context, headers *[]kafka.Header) {
	otel.GetTextMapPropagator().Inject(ctx, KafkaHeaderCarrier{Headers: headers})
}

// ExtractTraceContext extracts trace context from Kafka message headers into a context.
//
// Usage:
//
//	ctx := otel.ExtractTraceContext(context.Background(), kafkaMessage.Headers)
//	// use ctx for downstream processing with propagated trace context
func ExtractTraceContext(ctx context.Context, headers []kafka.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, KafkaHeaderCarrier{Headers: &headers})
}
