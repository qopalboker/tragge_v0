package resilience

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Prometheus metrics for bulkheads

	// bulkheadInUse tracks the current number of concurrent requests in each bulkhead.
	bulkheadInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "tragge",
			Subsystem: "bulkhead",
			Name:      "in_use",
			Help:      "Current number of concurrent requests in the bulkhead",
		},
		[]string{"name"},
	)

	// bulkheadMax tracks the maximum capacity of each bulkhead.
	bulkheadMax = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "tragge",
			Subsystem: "bulkhead",
			Name:      "max",
			Help:      "Maximum capacity of the bulkhead",
		},
		[]string{"name"},
	)

	// bulkheadRejections tracks the total number of rejected requests due to full bulkhead.
	bulkheadRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "bulkhead",
			Name:      "rejections_total",
			Help:      "Total number of requests rejected due to full bulkhead",
		},
		[]string{"name"},
	)

	// bulkheadWaitTime tracks the time spent waiting for bulkhead capacity.
	bulkheadWaitTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "tragge",
			Subsystem: "bulkhead",
			Name:      "wait_seconds",
			Help:      "Time spent waiting for bulkhead capacity",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"name"},
	)
)

// initBulkheadMetrics initializes metrics for a bulkhead.
func initBulkheadMetrics(name string, maxSize int) {
	bulkheadInUse.WithLabelValues(name).Set(0)
	bulkheadMax.WithLabelValues(name).Set(float64(maxSize))
}

// recordBulkheadAcquire records when a bulkhead slot is acquired.
func recordBulkheadAcquire(name string) {
	bulkheadInUse.WithLabelValues(name).Inc()
}

// recordBulkheadRelease records when a bulkhead slot is released.
func recordBulkheadRelease(name string) {
	bulkheadInUse.WithLabelValues(name).Dec()
}

// recordBulkheadRejection records when a request is rejected due to full bulkhead.
func recordBulkheadRejection(name string) {
	bulkheadRejections.WithLabelValues(name).Inc()
}

// MetricsCollector returns a slice of all bulkhead Prometheus collectors.
// Use this to register metrics with a custom registry.
func MetricsCollector() []prometheus.Collector {
	return []prometheus.Collector{
		bulkheadInUse,
		bulkheadMax,
		bulkheadRejections,
		bulkheadWaitTime,
	}
}

// RegisterMetrics registers all bulkhead metrics with the given registry.
func RegisterMetrics(reg prometheus.Registerer) error {
	collectors := MetricsCollector()

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
