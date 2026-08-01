package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for a service.
type Metrics struct {
	registry *prometheus.Registry

	// HTTP metrics
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestsTotal   *prometheus.CounterVec

	// Custom metrics can be added by services
	customMetrics []prometheus.Collector
}

// MetricsConfig holds configuration for metrics.
type MetricsConfig struct {
	// Service is the name of the service (used as namespace)
	Service string
	// EnableGoMetrics enables Go runtime metrics (default: true)
	EnableGoMetrics bool
	// EnableProcessMetrics enables process metrics (default: true)
	EnableProcessMetrics bool
	// DurationBuckets are the buckets for request duration histogram
	// Default: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	DurationBuckets []float64
}

// DefaultMetricsConfig returns a MetricsConfig with sensible defaults.
func DefaultMetricsConfig(service string) MetricsConfig {
	return MetricsConfig{
		Service:              service,
		EnableGoMetrics:      true,
		EnableProcessMetrics: true,
		DurationBuckets:      []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}
}

// NewMetrics creates a new Metrics instance with standard HTTP metrics.
func NewMetrics(cfg MetricsConfig) (*Metrics, error) {
	registry := prometheus.NewRegistry()

	// Set default buckets if not specified
	if len(cfg.DurationBuckets) == 0 {
		cfg.DurationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	}

	// Create HTTP request duration histogram
	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Service,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   cfg.DurationBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// Create HTTP requests total counter
	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Service,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// Register HTTP metrics
	if err := registry.Register(httpRequestDuration); err != nil {
		return nil, err
	}
	if err := registry.Register(httpRequestsTotal); err != nil {
		return nil, err
	}

	// Register Go runtime metrics if enabled
	if cfg.EnableGoMetrics {
		if err := registry.Register(collectors.NewGoCollector()); err != nil {
			return nil, err
		}
	}

	// Register process metrics if enabled
	if cfg.EnableProcessMetrics {
		if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
			return nil, err
		}
	}

	return &Metrics{
		registry:            registry,
		HTTPRequestDuration: httpRequestDuration,
		HTTPRequestsTotal:   httpRequestsTotal,
	}, nil
}

// Register adds a custom metric to the registry.
func (m *Metrics) Register(collector prometheus.Collector) error {
	m.customMetrics = append(m.customMetrics, collector)
	return m.registry.Register(collector)
}

// MustRegister adds a custom metric to the registry and panics on error.
func (m *Metrics) MustRegister(collectors ...prometheus.Collector) {
	for _, c := range collectors {
		m.customMetrics = append(m.customMetrics, c)
	}
	m.registry.MustRegister(collectors...)
}

// Handler returns an http.Handler for the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry returns the underlying Prometheus registry for external registration.
// This is useful when you need to register custom metrics from other packages.
func (m *Metrics) Registry() prometheus.Registerer {
	return m.registry
}

// RecordRequest records metrics for an HTTP request.
func (m *Metrics) RecordRequest(method, path, status string, durationSeconds float64) {
	m.HTTPRequestDuration.WithLabelValues(method, path, status).Observe(durationSeconds)
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
}

// NewCounter creates and registers a new counter metric.
func (m *Metrics) NewCounter(opts prometheus.CounterOpts) prometheus.Counter {
	counter := prometheus.NewCounter(opts)
	m.registry.MustRegister(counter)
	return counter
}

// NewCounterVec creates and registers a new counter vector metric.
func (m *Metrics) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(opts, labelNames)
	m.registry.MustRegister(counter)
	return counter
}

// NewGauge creates and registers a new gauge metric.
func (m *Metrics) NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	gauge := prometheus.NewGauge(opts)
	m.registry.MustRegister(gauge)
	return gauge
}

// NewGaugeVec creates and registers a new gauge vector metric.
func (m *Metrics) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(opts, labelNames)
	m.registry.MustRegister(gauge)
	return gauge
}

// NewHistogram creates and registers a new histogram metric.
func (m *Metrics) NewHistogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	histogram := prometheus.NewHistogram(opts)
	m.registry.MustRegister(histogram)
	return histogram
}

// NewHistogramVec creates and registers a new histogram vector metric.
func (m *Metrics) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	histogram := prometheus.NewHistogramVec(opts, labelNames)
	m.registry.MustRegister(histogram)
	return histogram
}

// NewSummary creates and registers a new summary metric.
func (m *Metrics) NewSummary(opts prometheus.SummaryOpts) prometheus.Summary {
	summary := prometheus.NewSummary(opts)
	m.registry.MustRegister(summary)
	return summary
}

// NewSummaryVec creates and registers a new summary vector metric.
func (m *Metrics) NewSummaryVec(opts prometheus.SummaryOpts, labelNames []string) *prometheus.SummaryVec {
	summary := prometheus.NewSummaryVec(opts, labelNames)
	m.registry.MustRegister(summary)
	return summary
}
