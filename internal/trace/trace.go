package trace

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc" // gRPC экспортер для трейсов
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracerProvider(ctx context.Context, otelEndpoint string) (func(), error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(otelEndpoint),
	}

	// Создаём gRPC Exporter (порт 4317)
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return func() {}, fmt.Errorf("failed to create OTLP trace exporter: %v", err)
	}

	// Метаинформация (Resource)
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return func() {}, fmt.Errorf("failed to create resource: %v", err)
	}

	// Инициализируем TracerProvider
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// Используем BatchSpanProcessor для эффективности (отправляет пачками)
		sdktrace.WithBatcher(exporter),
		// Sampler: AlwaysSample пишет 100% запросов (хорошо для дебага)
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Регистрируем глобальный провайдер
	otel.SetTracerProvider(tracerProvider)

	// Настраиваем propagation контекста.
	// Это позволяет передавать TraceID в заголовках запросов к другим сервисам.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Возвращаем функцию для корректного завершения (flush данных перед выходом)
	return func() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := tracerProvider.Shutdown(ctx); err != nil {
			otel.Handle(err)
		}
	}, nil
}
