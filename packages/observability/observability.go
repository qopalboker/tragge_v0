// Package observability provides logging, metrics, and tracing for Go services.
//
// The package integrates:
//   - Structured JSON logging with zap
//   - Prometheus metrics with http_request_duration_seconds and http_requests_total
//   - OpenTelemetry tracing with OTLP export to Tempo/Jaeger
//   - HTTP middleware that combines all three
//
// Basic usage:
//
//	obs, err := observability.New(observability.Config{
//	    Service: "my-service",
//	    Env:     "production",
//	    Version: "1.0.0",
//	    OTLPEndpoint: "localhost:4317",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer obs.Shutdown(context.Background())
//
//	r := chi.NewRouter()
//	r.Use(obs.Middleware.Middleware)
//	r.Get("/metrics", obs.MetricsHandler())
package observability

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
)

// Config holds configuration for all observability components.
type Config struct {
	// Service is the name of the service (required)
	Service string

	// Env is the environment (e.g., "production", "staging", "development")
	// Default: value of ENVIRONMENT or GO_ENV env var, or "development"
	Env string

	// Version is the service version
	// Default: value of VERSION env var, or "unknown"
	Version string

	// LogLevel is the minimum log level (default: "info")
	// Valid values: "debug", "info", "warn", "error"
	LogLevel string

	// Development enables development mode (pretty logging, stack traces)
	// Default: true if Env is "development" or "local"
	Development bool

	// OTLPEndpoint is the OTLP gRPC endpoint for tracing
	// If empty, tracing is disabled
	// Default: value of OTEL_EXPORTER_OTLP_ENDPOINT env var
	OTLPEndpoint string

	// OTLPInsecure disables TLS for the OTLP connection
	// Default: true
	OTLPInsecure bool

	// SampleRatio is the tracing sample ratio (0.0 to 1.0)
	// Default: 1.0 (sample all traces)
	SampleRatio float64

	// EnableGoMetrics enables Go runtime metrics.
	// Must be set explicitly; defaults to false if omitted.
	EnableGoMetrics bool

	// EnableProcessMetrics enables process metrics.
	// Must be set explicitly; defaults to false if omitted.
	EnableProcessMetrics bool
}

// Observability holds all observability components.
type Observability struct {
	Logger     *Logger
	Metrics    *Metrics
	Tracer     *Tracer
	Middleware *HTTPMiddleware

	config Config
}

// New creates a new Observability instance with all components initialized.
func New(ctx context.Context, cfg Config) (*Observability, error) {
	InstallStandardLoggerRedaction()
	// Apply defaults
	cfg = applyDefaults(cfg)

	// Initialize logger
	logger, err := NewLogger(LogConfig{
		Service:     cfg.Service,
		Env:         cfg.Env,
		Version:     cfg.Version,
		Level:       cfg.LogLevel,
		Development: cfg.Development,
	})
	if err != nil {
		return nil, err
	}

	// Initialize metrics
	metrics, err := NewMetrics(MetricsConfig{
		Service:              sanitizeServiceName(cfg.Service),
		EnableGoMetrics:      cfg.EnableGoMetrics,
		EnableProcessMetrics: cfg.EnableProcessMetrics,
	})
	if err != nil {
		return nil, err
	}

	// Initialize tracing
	tracer, err := InitTracing(ctx, TracingConfig{
		Service:      cfg.Service,
		Env:          cfg.Env,
		Version:      cfg.Version,
		OTLPEndpoint: cfg.OTLPEndpoint,
		OTLPInsecure: cfg.OTLPInsecure,
		SampleRatio:  cfg.SampleRatio,
	})
	if err != nil {
		return nil, err
	}

	// Create middleware
	middleware := NewHTTPMiddleware(logger, metrics, tracer, cfg.Service)

	return &Observability{
		Logger:     logger,
		Metrics:    metrics,
		Tracer:     tracer,
		Middleware: middleware,
		config:     cfg,
	}, nil
}

// Shutdown gracefully shuts down all components.
func (o *Observability) Shutdown(ctx context.Context) error {
	var errs []error

	if o.Logger != nil {
		if err := o.Logger.Sync(); err != nil {
			// Ignore sync errors for stdout/stderr
			if err.Error() != "sync /dev/stdout: invalid argument" &&
				err.Error() != "sync /dev/stderr: invalid argument" {
				errs = append(errs, err)
			}
		}
	}

	if o.Tracer != nil {
		if err := o.Tracer.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// MetricsHandler returns an http.Handler for the /metrics endpoint.
func (o *Observability) MetricsHandler() http.Handler {
	return o.Metrics.Handler()
}

// applyDefaults applies default values to the config.
func applyDefaults(cfg Config) Config {
	// Environment
	if cfg.Env == "" {
		cfg.Env = os.Getenv("ENVIRONMENT")
		if cfg.Env == "" {
			cfg.Env = os.Getenv("GO_ENV")
		}
		if cfg.Env == "" {
			cfg.Env = "development"
		}
	}

	// Version
	if cfg.Version == "" {
		cfg.Version = os.Getenv("VERSION")
		if cfg.Version == "" {
			cfg.Version = "unknown"
		}
	}

	// Log level
	if cfg.LogLevel == "" {
		cfg.LogLevel = os.Getenv("LOG_LEVEL")
		if cfg.LogLevel == "" {
			cfg.LogLevel = "info"
		}
	}

	// Development mode
	if cfg.Env == "development" || cfg.Env == "local" {
		cfg.Development = true
	}

	// OTLP endpoint
	if cfg.OTLPEndpoint == "" {
		cfg.OTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	// OTLP insecure (default to true only for local development endpoints)
	if cfg.OTLPEndpoint != "" && !cfg.OTLPInsecure {
		if strings.HasPrefix(cfg.OTLPEndpoint, "localhost") || strings.HasPrefix(cfg.OTLPEndpoint, "127.0.0.1") {
			cfg.OTLPInsecure = true
		}
	}

	// Sample ratio
	if cfg.SampleRatio == 0 {
		cfg.SampleRatio = 1.0
	}

	return cfg
}

// sanitizeServiceName converts a service name to a valid Prometheus namespace.
func sanitizeServiceName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else if c == '-' || c == '.' || c == ' ' {
			result = append(result, '_')
		}
	}
	return string(result)
}
