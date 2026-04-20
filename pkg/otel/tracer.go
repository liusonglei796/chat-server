// Package otel 提供 OpenTelemetry 追踪初始化功能
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// InitTracer 初始化 OpenTelemetry TracerProvider，使用 OTLP gRPC 导出器
// endpoint: OTLP gRPC 端点地址，如 "localhost:4317"
// serviceName: 服务名称，用于 OTel 资源属性
// 返回 shutdown 函数和可能的错误。调用方应在程序退出时调用 shutdown 函数
func InitTracer(ctx context.Context, endpoint string, serviceName string) (func(context.Context) error, error) {
	logger, _ := zap.NewProduction()

	// 创建 OTLP gRPC 导出器
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// 创建资源，包含服务名称和服务版本属性
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 创建 TracerProvider，使用 BatchSpanProcessor 批量导出 span
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局传播器，支持 TraceContext 和 Baggage
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("OpenTelemetry tracer initialized",
		zap.String("endpoint", endpoint),
		zap.String("serviceName", serviceName),
	)

	// 返回 shutdown 函数，用于优雅关闭 TracerProvider
	shutdown := func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			logger.Error("failed to shutdown TracerProvider", zap.Error(err))
			return fmt.Errorf("failed to shutdown TracerProvider: %w", err)
		}
		logger.Info("OpenTelemetry tracer shutdown successfully")
		return nil
	}

	return shutdown, nil
}
