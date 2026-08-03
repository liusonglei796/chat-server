// grpc_propagation.go
// gRPC trace context propagation for OpenTelemetry.
//
// Provides client/server unary interceptors that propagate distributed trace
// context over gRPC metadata (traceparent/baggage), enabling end-to-end
// tracing across chat_server -> microservice gRPC calls without depending on
// the otelgrpc instrumentation package.
package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// tracerName 是所有传播器创建的 span 所使用的 instrumentation scope 名称
const tracerName = "kama_chat_server/pkg/otel"

// grpcMetadataCarrier implements propagation.TextMapCarrier over gRPC metadata.
type grpcMetadataCarrier struct {
	md metadata.MD
}

// Get returns the value for a given metadata key.
func (c grpcMetadataCarrier) Get(key string) string {
	vals := c.md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Set sets the value for a given metadata key.
func (c grpcMetadataCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

// Keys returns all metadata keys currently present.
func (c grpcMetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}

// ClientTraceInterceptor 是 gRPC 客户端 unary 拦截器：
// 创建 client span，并把 trace context 注入到出站 gRPC metadata 中，
// 使下游服务能够把调用链与当前 trace 关联。
// 用法：grpc.WithChainUnaryInterceptor(interceptor.ClientAuthInterceptor(), otel.ClientTraceInterceptor())
func ClientTraceInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 创建 client span（在传播器初始化后自动成为当前 trace 的子 span）
		tracer := otel.GetTracerProvider().Tracer(tracerName)
		ctx, span := tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
		defer span.End()

		// 把 trace context 注入到出站 metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		md = md.Copy()
		otel.GetTextMapPropagator().Inject(ctx, grpcMetadataCarrier{md: md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}
}

// ServerTraceInterceptor 是 gRPC 服务端 unary 拦截器：
// 从入站 metadata 中提取 trace context，并创建 server span。
// 用法：grpc.ChainUnaryInterceptor(interceptor.ServerAuthInterceptor(), otel.ServerTraceInterceptor())
func ServerTraceInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 从入站 metadata 提取父 trace context
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = otel.GetTextMapPropagator().Extract(ctx, grpcMetadataCarrier{md: md})
		}

		// 创建 server span
		tracer := otel.GetTracerProvider().Tracer(tracerName)
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return resp, err
	}
}
