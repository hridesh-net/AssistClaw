package tracing

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Config struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
	SampleRatio float64
}

var tracer = otel.Tracer("assistclaw")

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func Init(ctx context.Context, cfg Config, log *zap.Logger) func(context.Context) error {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }
	}

	if cfg.SampleRatio <= 0 {
		cfg.SampleRatio = 0.01
	}
	if cfg.SampleRatio > 1 {
		cfg.SampleRatio = 1
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = "assistclaw"
	}

	exp, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint(strings.TrimSpace(cfg.Endpoint)),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Warn("tracing exporter init failed, continuing without tracing", zap.Error(err))
		return func(context.Context) error { return nil }
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var once sync.Once
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		once.Do(func() {
			log.Warn("tracing exporter reported error", zap.Error(err))
		})
	}))

	return tp.Shutdown
}
