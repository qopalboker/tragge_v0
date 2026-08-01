package circuitbreaker

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for circuit breakers.
//
// These metrics are registered with the default Prometheus registerer via promauto
// at package init time. This is standard for library-level metrics in Go.
// When using a custom registry (e.g., in tests), use RegisterMetrics() which
// handles AlreadyRegisteredError gracefully.
var (
	// circuitState tracks the current state of each circuit breaker.
	// 0 = closed, 1 = open, 2 = half-open
	circuitState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "state",
			Help:      "Current state of circuit breaker (0=closed, 1=open, 2=half-open)",
		},
		[]string{"name"},
	)

	// circuitRequests tracks total requests to each circuit breaker.
	circuitRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "requests_total",
			Help:      "Total number of requests to circuit breaker",
		},
		[]string{"name"},
	)

	// circuitSuccesses tracks successful requests through each circuit breaker.
	circuitSuccesses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "successes_total",
			Help:      "Total number of successful requests",
		},
		[]string{"name"},
	)

	// circuitFailures tracks failed requests through each circuit breaker.
	circuitFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "failures_total",
			Help:      "Total number of failures",
		},
		[]string{"name"},
	)

	// circuitRejections tracks requests rejected due to open circuit.
	circuitRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "rejections_total",
			Help:      "Total requests rejected due to open circuit",
		},
		[]string{"name"},
	)

	// circuitTimeouts tracks requests that timed out.
	circuitTimeouts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "timeouts_total",
			Help:      "Total requests that timed out",
		},
		[]string{"name"},
	)

	// circuitStateChanges tracks state transitions.
	circuitStateChanges = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "state_changes_total",
			Help:      "Total number of state changes",
		},
		[]string{"name", "to_state"},
	)

	// circuitLatency tracks the latency of calls through the circuit breaker.
	circuitLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "call_duration_seconds",
			Help:      "Latency of circuit breaker calls",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"name", "result"},
	)

	// circuitOpenDuration tracks how long circuits stay open.
	circuitOpenDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "open_duration_seconds",
			Help:      "Duration circuit breaker stayed open before recovery",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"name"},
	)

	// circuitFailureRate is a gauge showing the current failure rate.
	circuitFailureRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "tragge",
			Subsystem: "circuit_breaker",
			Name:      "failure_rate",
			Help:      "Current failure rate (failures / total requests in window)",
		},
		[]string{"name"},
	)

	// Track circuit open start times for duration metrics
	circuitOpenStartTimes   = make(map[string]time.Time)
	circuitOpenStartTimesMu sync.Mutex

	// Track if metrics have been initialized for a circuit
	initializedMetrics   = make(map[string]bool)
	initializedMetricsMu sync.Mutex
)

// initMetrics initializes the Prometheus metrics for a circuit breaker.
// This sets the initial state to closed and initializes counters to 0.
func initMetrics(name string) {
	initializedMetricsMu.Lock()
	defer initializedMetricsMu.Unlock()

	if initializedMetrics[name] {
		return
	}

	// Initialize state to closed
	circuitState.WithLabelValues(name).Set(float64(StateClosed))

	// Initialize counters (they start at 0 by default, but explicitly adding labels)
	circuitRequests.WithLabelValues(name)
	circuitSuccesses.WithLabelValues(name)
	circuitFailures.WithLabelValues(name)
	circuitRejections.WithLabelValues(name)
	circuitTimeouts.WithLabelValues(name)
	circuitFailureRate.WithLabelValues(name).Set(0)

	initializedMetrics[name] = true
}

// recordRequest increments the request counter for a circuit.
func recordRequest(name string) {
	circuitRequests.WithLabelValues(name).Inc()
}

// recordSuccess increments the success counter for a circuit.
func recordSuccess(name string) {
	circuitSuccesses.WithLabelValues(name).Inc()
}

// recordFailure increments the failure counter for a circuit.
func recordFailure(name string) {
	circuitFailures.WithLabelValues(name).Inc()
}

// recordRejection increments the rejection counter for a circuit.
func recordRejection(name string) {
	circuitRejections.WithLabelValues(name).Inc()
}

// recordTimeout increments the timeout counter for a circuit.
func recordTimeout(name string) {
	circuitTimeouts.WithLabelValues(name).Inc()
}

// recordStateChange records a state change event.
func recordStateChange(name string, newState State) {
	circuitState.WithLabelValues(name).Set(float64(newState))
	circuitStateChanges.WithLabelValues(name, newState.String()).Inc()

	circuitOpenStartTimesMu.Lock()
	defer circuitOpenStartTimesMu.Unlock()

	switch newState {
	case StateOpen:
		// Record when circuit opened
		circuitOpenStartTimes[name] = time.Now()
	case StateClosed:
		// Record how long circuit was open
		if start, ok := circuitOpenStartTimes[name]; ok {
			duration := time.Since(start).Seconds()
			circuitOpenDuration.WithLabelValues(name).Observe(duration)
			delete(circuitOpenStartTimes, name)
		}
	}
}

// recordCallDuration records the duration of a call through the circuit.
func recordCallDuration(name, result string, duration time.Duration) {
	circuitLatency.WithLabelValues(name, result).Observe(duration.Seconds())
}

// UpdateFailureRate updates the failure rate gauge for a circuit.
// Called by the circuit breaker with current success/failure counts.
func UpdateFailureRate(name string, successes, failures int64) {
	total := successes + failures
	if total > 0 {
		rate := float64(failures) / float64(total)
		circuitFailureRate.WithLabelValues(name).Set(rate)
	}
}

// MetricsCollector returns a slice of all circuit breaker Prometheus collectors.
// Use this to register metrics with a custom registry.
func MetricsCollector() []prometheus.Collector {
	return []prometheus.Collector{
		circuitState,
		circuitRequests,
		circuitSuccesses,
		circuitFailures,
		circuitRejections,
		circuitTimeouts,
		circuitStateChanges,
		circuitLatency,
		circuitOpenDuration,
		circuitFailureRate,
	}
}

// RegisterMetrics registers all circuit breaker metrics with the given registry.
// This is useful when using a custom Prometheus registry instead of the default.
func RegisterMetrics(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		circuitState,
		circuitRequests,
		circuitSuccesses,
		circuitFailures,
		circuitRejections,
		circuitTimeouts,
		circuitStateChanges,
		circuitLatency,
		circuitOpenDuration,
		circuitFailureRate,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			// Already registered is not an error
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}
	return nil
}
