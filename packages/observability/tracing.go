package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig holds configuration for OpenTelemetry tracing.
type TracingConfig struct {
	// Service is the name of the service
	Service string
	// Env is the environment (e.g., "production", "staging", "development")
	Env string
	// Version is the service version
	Version string
	// OTLPEndpoint is the OTLP gRPC endpoint (e.g., "localhost:4317" for Tempo/Jaeger)
	// If empty, tracing is disabled
	OTLPEndpoint string
	// OTLPInsecure disables TLS for the OTLP connection (default: true for local dev)
	OTLPInsecure bool
	// SampleRatio is the sampling ratio (0.0 to 1.0, default: 1.0 for all traces)
	SampleRatio float64
	// BatchTimeout is the maximum time to wait before exporting a batch (default: 5s)
	BatchTimeout time.Duration
	// MaxExportBatchSize is the maximum number of spans to export in a single batch (default: 512)
	MaxExportBatchSize int
}

// DefaultTracingConfig returns a TracingConfig with sensible defaults.
func DefaultTracingConfig(service string) TracingConfig {
	return TracingConfig{
		Service:            service,
		OTLPEndpoint:       "",
		OTLPInsecure:       true,
		SampleRatio:        1.0,
		BatchTimeout:       5 * time.Second,
		MaxExportBatchSize: 512,
	}
}

// Tracer wraps the OpenTelemetry tracer provider and provides convenience methods.
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	service  string
}

// InitTracing initializes OpenTelemetry tracing and returns a Tracer.
// Call Shutdown() when done to flush any pending spans.
func InitTracing(ctx context.Context, cfg TracingConfig) (*Tracer, error) {
	// If no endpoint is configured, return a no-op tracer
	if cfg.OTLPEndpoint == "" {
		return &Tracer{
			tracer:  otel.Tracer(cfg.Service),
			service: cfg.Service,
		}, nil
	}

	// Create OTLP exporter options
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.OTLPInsecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	// Create OTLP exporter
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service info
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.Service),
	}
	if cfg.Env != "" {
		attrs = append(attrs, semconv.DeploymentEnvironment(cfg.Env))
	}
	if cfg.Version != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.Version))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler based on sample ratio
	var sampler sdktrace.Sampler
	if cfg.SampleRatio >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRatio <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRatio)
	}

	// Set batch options
	batchOpts := []sdktrace.BatchSpanProcessorOption{}
	if cfg.BatchTimeout > 0 {
		batchOpts = append(batchOpts, sdktrace.WithBatchTimeout(cfg.BatchTimeout))
	}
	if cfg.MaxExportBatchSize > 0 {
		batchOpts = append(batchOpts, sdktrace.WithMaxExportBatchSize(cfg.MaxExportBatchSize))
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, batchOpts...),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global trace provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		provider: provider,
		tracer:   provider.Tracer(cfg.Service),
		service:  cfg.Service,
	}, nil
}

// Shutdown flushes any pending spans and shuts down the tracer.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
}

// Tracer returns the underlying OpenTelemetry tracer.
func (t *Tracer) Tracer() trace.Tracer {
	return t.tracer
}

// StartSpan starts a new span with the given name.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// StartSpanWithAttributes starts a new span with the given name and attributes.
func (t *Tracer) StartSpanWithAttributes(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// SpanFromContext returns the current span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext returns the trace ID from the current span context.
func TraceIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}
	return ""
}

// SpanIDFromContext returns the span ID from the current span context.
func SpanIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasSpanID() {
		return spanCtx.SpanID().String()
	}
	return ""
}

// AddSpanError records an error on the current span.
func AddSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
	}
}

// SetSpanAttributes sets attributes on the current span.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// Common attribute helpers

// UserIDAttr returns an attribute for user ID.
func UserIDAttr(userID string) attribute.KeyValue {
	return attribute.String("user.id", userID)
}

// ContestIDAttr returns an attribute for contest ID.
func ContestIDAttr(contestID string) attribute.KeyValue {
	return attribute.String("contest.id", contestID)
}

// OrderIDAttr returns an attribute for order ID.
func OrderIDAttr(orderID string) attribute.KeyValue {
	return attribute.String("order.id", orderID)
}

// SymbolAttr returns an attribute for symbol.
func SymbolAttr(symbol string) attribute.KeyValue {
	return attribute.String("symbol", symbol)
}

// HTTPMethodAttr returns an attribute for HTTP method.
func HTTPMethodAttr(method string) attribute.KeyValue {
	return attribute.String("http.method", method)
}

// HTTPPathAttr returns an attribute for HTTP path.
func HTTPPathAttr(path string) attribute.KeyValue {
	return attribute.String("http.path", path)
}

// HTTPStatusCodeAttr returns an attribute for HTTP status code.
func HTTPStatusCodeAttr(code int) attribute.KeyValue {
	return attribute.Int("http.status_code", code)
}
