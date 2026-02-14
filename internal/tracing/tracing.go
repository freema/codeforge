package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration.
type Config struct {
	Enabled      bool
	Endpoint     string
	SamplingRate float64
	ServiceName  string
	Version      string
}

// Setup initializes the OpenTelemetry tracer provider.
// Returns a shutdown function that must be called on application exit.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	opts := []otlptracehttp.Option{}
	if cfg.Endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	opts = append(opts, otlptracehttp.WithInsecure())

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if cfg.SamplingRate > 0 && cfg.SamplingRate < 1 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns a named tracer for the codeforge application.
func Tracer() trace.Tracer {
	return otel.Tracer("codeforge")
}

// TraceIDFromContext extracts the trace ID string from the current span context.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// WithTaskAttributes returns a SpanStartOption with common task attributes.
func WithTaskAttributes(taskID string, iteration int) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String("task.id", taskID),
		attribute.Int("task.iteration", iteration),
	)
}
