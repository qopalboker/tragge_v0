package ratelimit

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics provides Prometheus metrics for rate limiting.
type Metrics struct {
	// requestsTotal counts total rate limit checks.
	requestsTotal *prometheus.CounterVec

	// exceededTotal counts rate limit exceeded events.
	exceededTotal *prometheus.CounterVec

	// allowedTotal counts allowed requests.
	allowedTotal *prometheus.CounterVec

	// errorsTotal counts rate limiter errors.
	errorsTotal *prometheus.CounterVec

	// activeUsers tracks currently tracked users (gauge).
	activeUsers *prometheus.GaugeVec

	// latency measures rate limit check duration.
	latency *prometheus.HistogramVec

	// registered tracks whether metrics are registered.
	registered bool
	mu         sync.Mutex
}

// MetricsConfig configures the metrics collector.
type MetricsConfig struct {
	// Namespace for all metrics (default: "tragge").
	Namespace string

	// Subsystem for all metrics (default: "ratelimit").
	Subsystem string

	// ConstLabels are added to all metrics.
	ConstLabels prometheus.Labels

	// Registerer is the prometheus registerer to use.
	// If nil, prometheus.DefaultRegisterer is used.
	Registerer prometheus.Registerer
}

// DefaultMetricsConfig returns the default metrics configuration.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Namespace:  "tragge",
		Subsystem:  "ratelimit",
		Registerer: prometheus.DefaultRegisterer,
	}
}

// NewMetrics creates a new metrics collector.
func NewMetrics(cfg MetricsConfig) *Metrics {
	if cfg.Namespace == "" {
		cfg.Namespace = "tragge"
	}
	if cfg.Subsystem == "" {
		cfg.Subsystem = "ratelimit"
	}
	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "requests_total",
				Help:        "Total number of rate limit checks",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"limiter", "result"},
		),

		exceededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "exceeded_total",
				Help:        "Total number of rate limit exceeded events",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"limiter"},
		),

		allowedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "allowed_total",
				Help:        "Total number of allowed requests",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"limiter"},
		),

		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "errors_total",
				Help:        "Total number of rate limiter errors",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"limiter", "error_type"},
		),

		activeUsers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "active_users",
				Help:        "Number of users currently being rate limited",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"limiter"},
		),

		latency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   cfg.Namespace,
				Subsystem:   cfg.Subsystem,
				Name:        "check_duration_seconds",
				Help:        "Duration of rate limit checks in seconds",
				ConstLabels: cfg.ConstLabels,
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"limiter"},
		),
	}

	return m
}

// Register registers all metrics with the provided registerer.
// It's safe to call multiple times.
func (m *Metrics) Register(registerer prometheus.Registerer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered {
		return nil
	}

	collectors := []prometheus.Collector{
		m.requestsTotal,
		m.exceededTotal,
		m.allowedTotal,
		m.errorsTotal,
		m.activeUsers,
		m.latency,
	}

	for _, c := range collectors {
		if err := registerer.Register(c); err != nil {
			// Check if already registered
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	m.registered = true
	return nil
}

// MustRegister registers all metrics and panics on error.
func (m *Metrics) MustRegister(registerer prometheus.Registerer) {
	if err := m.Register(registerer); err != nil {
		panic(err)
	}
}

// Describe implements prometheus.Collector.
func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	m.requestsTotal.Describe(ch)
	m.exceededTotal.Describe(ch)
	m.allowedTotal.Describe(ch)
	m.errorsTotal.Describe(ch)
	m.activeUsers.Describe(ch)
	m.latency.Describe(ch)
}

// Collect implements prometheus.Collector.
func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.requestsTotal.Collect(ch)
	m.exceededTotal.Collect(ch)
	m.allowedTotal.Collect(ch)
	m.errorsTotal.Collect(ch)
	m.activeUsers.Collect(ch)
	m.latency.Collect(ch)
}

// RecordAllowed records an allowed request.
func (m *Metrics) RecordAllowed(limiter, key string) {
	m.requestsTotal.WithLabelValues(limiter, "allowed").Inc()
	m.allowedTotal.WithLabelValues(limiter).Inc()
}

// RecordExceeded records a rate limit exceeded event.
func (m *Metrics) RecordExceeded(limiter, key string) {
	m.requestsTotal.WithLabelValues(limiter, "exceeded").Inc()
	m.exceededTotal.WithLabelValues(limiter).Inc()
}

// RecordError records a rate limiter error.
func (m *Metrics) RecordError(limiter, errorType string) {
	m.errorsTotal.WithLabelValues(limiter, errorType).Inc()
}

// SetActiveUsers sets the number of active users for a limiter.
func (m *Metrics) SetActiveUsers(limiter string, count int) {
	m.activeUsers.WithLabelValues(limiter).Set(float64(count))
}

// ObserveLatency records the duration of a rate limit check.
func (m *Metrics) ObserveLatency(limiter string, seconds float64) {
	m.latency.WithLabelValues(limiter).Observe(seconds)
}

// Timer returns a function that records the duration when called.
func (m *Metrics) Timer(limiter string) func() {
	timer := prometheus.NewTimer(m.latency.WithLabelValues(limiter))
	return func() {
		timer.ObserveDuration()
	}
}

// Global metrics instance for convenience.
var (
	globalMetrics     *Metrics
	globalMetricsOnce sync.Once
)

// GlobalMetrics returns the global metrics instance, creating it if necessary.
func GlobalMetrics() *Metrics {
	globalMetricsOnce.Do(func() {
		globalMetrics = NewMetrics(DefaultMetricsConfig())
		globalMetrics.MustRegister(prometheus.DefaultRegisterer)
	})
	return globalMetrics
}

// InitGlobalMetrics initializes the global metrics with custom config.
func InitGlobalMetrics(cfg MetricsConfig) *Metrics {
	globalMetricsOnce.Do(func() {
		globalMetrics = NewMetrics(cfg)
		if cfg.Registerer != nil {
			globalMetrics.MustRegister(cfg.Registerer)
		} else {
			globalMetrics.MustRegister(prometheus.DefaultRegisterer)
		}
	})
	return globalMetrics
}

// MetricsMiddleware wraps a limiter to record metrics on each check.
type MetricsMiddleware struct {
	limiter RateLimiter
	metrics *Metrics
	name    string
}

// NewMetricsMiddleware wraps a limiter with metrics recording.
func NewMetricsMiddleware(limiter RateLimiter, metrics *Metrics, name string) *MetricsMiddleware {
	return &MetricsMiddleware{
		limiter: limiter,
		metrics: metrics,
		name:    name,
	}
}

// Allow checks if a request is allowed and records metrics.
func (mm *MetricsMiddleware) Allow(key string) bool {
	done := mm.metrics.Timer(mm.name)
	defer done()

	allowed := mm.limiter.Allow(key)
	if allowed {
		mm.metrics.RecordAllowed(mm.name, key)
	} else {
		mm.metrics.RecordExceeded(mm.name, key)
	}
	return allowed
}

// AllowN checks if n requests are allowed and records metrics.
func (mm *MetricsMiddleware) AllowN(key string, n int) bool {
	done := mm.metrics.Timer(mm.name)
	defer done()

	allowed := mm.limiter.AllowN(key, n)
	if allowed {
		mm.metrics.RecordAllowed(mm.name, key)
	} else {
		mm.metrics.RecordExceeded(mm.name, key)
	}
	return allowed
}

// Reset resets the limiter for the key.
func (mm *MetricsMiddleware) Reset(key string) {
	mm.limiter.Reset(key)
}

// Remaining returns remaining requests.
func (mm *MetricsMiddleware) Remaining(key string) int {
	return mm.limiter.Remaining(key)
}

// RetryAfter returns the retry duration.
func (mm *MetricsMiddleware) RetryAfter(key string) time.Duration {
	return mm.limiter.RetryAfter(key)
}

// ActiveUsersCollector periodically updates active user metrics.
type ActiveUsersCollector struct {
	limiters map[string]interface {
		ActiveUsers() int
	}
	metrics *Metrics
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewActiveUsersCollector creates a collector for active user metrics.
func NewActiveUsersCollector(metrics *Metrics) *ActiveUsersCollector {
	return &ActiveUsersCollector{
		limiters: make(map[string]interface{ ActiveUsers() int }),
		metrics:  metrics,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// AddLimiter adds a limiter to track.
func (c *ActiveUsersCollector) AddLimiter(name string, limiter interface{ ActiveUsers() int }) {
	c.limiters[name] = limiter
}

// Start begins periodic collection of active user metrics at the given interval.
func (c *ActiveUsersCollector) Start(interval time.Duration) {
	go func() {
		defer close(c.doneCh)
		defer func() {
			if r := recover(); r != nil {
				// Active users metrics collection panic should not crash the process
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.Collect()
			}
		}
	}()
}

// Collect updates all active user metrics.
func (c *ActiveUsersCollector) Collect() {
	for name, limiter := range c.limiters {
		c.metrics.SetActiveUsers(name, limiter.ActiveUsers())
	}
}

// Stop stops the collector and waits for the background goroutine to finish.
func (c *ActiveUsersCollector) Stop() {
	close(c.stopCh)
	<-c.doneCh
}
