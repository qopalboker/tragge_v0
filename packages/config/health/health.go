// Package health provides standardized health check responses and handlers
// for Kubernetes liveness, readiness, and startup probes across all services.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Status represents the overall health status of a service.
type Status string

const (
	// StatusOK indicates the service is fully healthy.
	StatusOK Status = "ok"
	// StatusDegraded indicates the service is running but with reduced functionality.
	StatusDegraded Status = "degraded"
	// StatusUnavailable indicates the service cannot handle requests.
	StatusUnavailable Status = "unavailable"
)

// DependencyStatus represents the health status of a single dependency.
type DependencyStatus struct {
	// Name is the dependency name (e.g., "database", "redis", "kafka").
	Name string `json:"name"`
	// Status is the health status of the dependency.
	Status Status `json:"status"`
	// Latency is the time taken to check the dependency (in milliseconds).
	Latency int64 `json:"latency_ms,omitempty"`
	// Message provides additional context about the status.
	Message string `json:"message,omitempty"`
	// Details contains dependency-specific information.
	Details map[string]interface{} `json:"details,omitempty"`
}

// HealthResponse is the standardized response format for all health endpoints.
type HealthResponse struct {
	// Status is the overall health status.
	Status Status `json:"status"`
	// Service is the name of the service.
	Service string `json:"service"`
	// Version is the service version (from build info or environment).
	Version string `json:"version,omitempty"`
	// Timestamp is when the health check was performed.
	Timestamp time.Time `json:"timestamp"`
	// Dependencies lists the status of each dependency.
	Dependencies []DependencyStatus `json:"dependencies,omitempty"`
	// Message provides additional context about the overall status.
	Message string `json:"message,omitempty"`
}

// CheckFunc is a function that checks a dependency's health.
// It should return nil if healthy, or an error describing the problem.
type CheckFunc func(ctx context.Context) error

// Dependency represents a health check dependency.
type Dependency struct {
	// Name is the dependency identifier.
	Name string
	// Critical indicates whether this dependency is required for the service to be ready.
	// If a critical dependency fails, the service returns 503 Service Unavailable.
	// Non-critical dependencies failing will set status to "degraded" but still return 200.
	Critical bool
	// Check is the function to call to check health.
	Check CheckFunc
}

// Checker manages health checks for a service.
type Checker struct {
	serviceName  string
	version      string
	dependencies []Dependency
	mu           sync.RWMutex
	timeout      time.Duration

	// Prometheus metrics
	checkDuration *prometheus.HistogramVec
	checkStatus   *prometheus.GaugeVec
}

// Package-level metrics registered once to avoid duplicate registration panics.
var (
	healthMetricsOnce   sync.Once
	healthCheckDuration *prometheus.HistogramVec
	healthCheckStatus   *prometheus.GaugeVec
)

func initHealthMetrics() {
	healthCheckDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "health_check_duration_seconds",
		Help:    "Duration of health check calls",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	}, []string{"service", "dependency"})
	healthCheckStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_check_status",
		Help: "Health check status (1=ok, 0.5=degraded, 0=unavailable)",
	}, []string{"service", "dependency"})
}

// Option configures a Checker.
type Option func(*Checker)

// WithTimeout sets the timeout for individual health checks.
func WithTimeout(d time.Duration) Option {
	return func(c *Checker) {
		c.timeout = d
	}
}

// WithVersion sets the service version.
func WithVersion(v string) Option {
	return func(c *Checker) {
		c.version = v
	}
}

// NewChecker creates a new health checker for a service.
func NewChecker(serviceName string, opts ...Option) *Checker {
	healthMetricsOnce.Do(initHealthMetrics)

	c := &Checker{
		serviceName:   serviceName,
		dependencies:  make([]Dependency, 0),
		timeout:       2 * time.Second,
		checkDuration: healthCheckDuration,
		checkStatus:   healthCheckStatus,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// AddDependency adds a dependency to check.
func (c *Checker) AddDependency(name string, critical bool, check CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dependencies = append(c.dependencies, Dependency{
		Name:     name,
		Critical: critical,
		Check:    check,
	})
}

// Check performs all health checks and returns the overall status.
func (c *Checker) Check(ctx context.Context) *HealthResponse {
	c.mu.RLock()
	deps := make([]Dependency, len(c.dependencies))
	copy(deps, c.dependencies)
	c.mu.RUnlock()

	response := &HealthResponse{
		Status:       StatusOK,
		Service:      c.serviceName,
		Version:      c.version,
		Timestamp:    time.Now().UTC(),
		Dependencies: make([]DependencyStatus, 0, len(deps)),
	}

	// Run checks concurrently
	type checkResult struct {
		status   DependencyStatus
		critical bool
	}
	results := make(chan checkResult, len(deps))

	var wg sync.WaitGroup
	for _, dep := range deps {
		wg.Add(1)
		go func(d Dependency) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- checkResult{
						status: DependencyStatus{
							Name:    d.Name,
							Status:  StatusUnavailable,
							Message: fmt.Sprintf("panic: %s", observability.RedactPanic(r)),
						},
						critical: d.Critical,
					}
				}
			}()

			checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			start := time.Now()
			err := d.Check(checkCtx)
			latency := time.Since(start)

			status := DependencyStatus{
				Name:    d.Name,
				Latency: latency.Milliseconds(),
			}

			// Record metrics
			c.checkDuration.WithLabelValues(c.serviceName, d.Name).Observe(latency.Seconds())

			if err != nil {
				status.Status = StatusUnavailable
				status.Message = err.Error()
				c.checkStatus.WithLabelValues(c.serviceName, d.Name).Set(0)
			} else {
				status.Status = StatusOK
				c.checkStatus.WithLabelValues(c.serviceName, d.Name).Set(1)
			}

			results <- checkResult{status: status, critical: d.Critical}
		}(dep)
	}

	// Close results channel when all checks complete
	go func() {
		defer func() { recover() }()
		wg.Wait()
		close(results)
	}()

	// Collect results
	var criticalFailure bool
	var nonCriticalFailure bool
	var failedDeps []string

	for result := range results {
		response.Dependencies = append(response.Dependencies, result.status)
		if result.status.Status != StatusOK {
			if result.critical {
				criticalFailure = true
				failedDeps = append(failedDeps, result.status.Name)
			} else {
				nonCriticalFailure = true
			}
		}
	}

	// Determine overall status
	if criticalFailure {
		response.Status = StatusUnavailable
		response.Message = fmt.Sprintf("critical dependencies unavailable: %s", strings.Join(failedDeps, ", "))
	} else if nonCriticalFailure {
		response.Status = StatusDegraded
		response.Message = "non-critical dependencies unavailable"
	}

	// Update overall status metric
	switch response.Status {
	case StatusOK:
		c.checkStatus.WithLabelValues(c.serviceName, "overall").Set(1)
	case StatusDegraded:
		c.checkStatus.WithLabelValues(c.serviceName, "overall").Set(0.5)
	case StatusUnavailable:
		c.checkStatus.WithLabelValues(c.serviceName, "overall").Set(0)
	}

	return response
}

// LivenessHandler returns an HTTP handler for the /healthz liveness probe.
// This should return 200 if the process is running, regardless of dependencies.
func (c *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := &HealthResponse{
			Status:    StatusOK,
			Service:   c.serviceName,
			Version:   c.version,
			Timestamp: time.Now().UTC(),
		}
		writeJSON(w, http.StatusOK, response)
	}
}

// ReadinessHandler returns an HTTP handler for the /readyz readiness probe.
// This checks all dependencies and returns 200 only if all critical dependencies are healthy.
func (c *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		response := c.Check(ctx)

		var httpStatus int
		switch response.Status {
		case StatusOK:
			httpStatus = http.StatusOK
		case StatusDegraded:
			httpStatus = http.StatusOK // Degraded is still "ready" to accept traffic
		case StatusUnavailable:
			httpStatus = http.StatusServiceUnavailable
		default:
			httpStatus = http.StatusInternalServerError
		}

		writeJSON(w, httpStatus, response)
	}
}

// StartupHandler returns an HTTP handler for the /startupz startup probe.
// This is similar to readiness but may have different thresholds during startup.
func (c *Checker) StartupHandler() http.HandlerFunc {
	return c.ReadinessHandler()
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
