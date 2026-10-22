package otel

import (
	"context"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	testSpanID  = "00f067aa0ba902b7"
)

// setupTracing 显式安装 W3C TraceContext 传播器和真实的 TracerProvider
// （生产环境由 InitTracer 负责设置；默认的 no-op 传播器/no-op provider 会让测试断言失去意义）
func setupTracing(t *testing.T) {
	t.Helper()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
}

// newTraceCtx 构造一个带有效 SpanContext 的 ctx
func newTraceCtx(t *testing.T) context.Context {
	t.Helper()
	tid, err := trace.TraceIDFromHex(testTraceID)
	if err != nil {
		t.Fatalf("parse trace id: %v", err)
	}
	sid, err := trace.SpanIDFromHex(testSpanID)
	if err != nil {
		t.Fatalf("parse span id: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestCaptureTraceContext(t *testing.T) {
	setupTracing(t)

	t.Run("valid span should be captured", func(t *testing.T) {
		tp, ts := CaptureTraceContext(newTraceCtx(t))
		if tp == "" {
			t.Fatal("expected non-empty traceparent")
		}
		if ts != "" {
			t.Errorf("expected empty tracestate, got %s", ts)
		}
		// traceparent 格式: version-traceid-spanid-flags
		if len(tp) < 55 {
			t.Errorf("traceparent too short: %s", tp)
		}
		if !strings.Contains(tp, testTraceID) || !strings.Contains(tp, testSpanID) {
			t.Errorf("traceparent should carry trace/span id, got %s", tp)
		}
	})

	t.Run("context without span should return empty", func(t *testing.T) {
		tp, ts := CaptureTraceContext(context.Background())
		if tp != "" || ts != "" {
			t.Errorf("expected empty trace context, got (%q, %q)", tp, ts)
		}
	})
}

func TestRestoreTraceContext(t *testing.T) {
	setupTracing(t)

	t.Run("restored context should keep the original trace id", func(t *testing.T) {
		tp, _ := CaptureTraceContext(newTraceCtx(t))
		ctx := RestoreTraceContext(context.Background(), tp, "")
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid span context after restore")
		}
		if sc.TraceID().String() != testTraceID {
			t.Errorf("expected trace id %s, got %s", testTraceID, sc.TraceID())
		}
	})

	t.Run("empty traceparent should leave ctx untouched", func(t *testing.T) {
		ctx := RestoreTraceContext(context.Background(), "", "")
		if trace.SpanContextFromContext(ctx).IsValid() {
			t.Error("expected no span context when traceparent is empty")
		}
	})
}

func TestKafkaHeaderRoundTrip(t *testing.T) {
	setupTracing(t)

	origin := newTraceCtx(t)
	var headers []kgo.RecordHeader

	ctx, span := StartKafkaProducerSpan(origin, &headers, "kafka publish group_created")
	span.End()
	if !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("expected producer span to carry a valid span context")
	}

	consumeCtx, consumeSpan := StartKafkaConsumerSpan(context.Background(), headers, "kafka consume group_created")
	consumeSpan.End()

	sc := trace.SpanContextFromContext(consumeCtx)
	if !sc.IsValid() {
		t.Fatal("expected consumer span to carry a valid span context")
	}
	if sc.TraceID().String() != testTraceID {
		t.Errorf("consumer span should belong to the producer trace, got trace id %s", sc.TraceID())
	}
}

func TestEndSpan(t *testing.T) {
	setupTracing(t)
	tracer := otel.GetTracerProvider().Tracer(tracerName)

	_, span := tracer.Start(context.Background(), "ok")
	EndSpan(span, nil)
	if span.IsRecording() {
		t.Error("expected span to be ended")
	}

	_, errSpan := tracer.Start(context.Background(), "boom")
	EndSpan(errSpan, context.Canceled)
	if errSpan.IsRecording() {
		t.Error("expected error span to be ended")
	}
}
