package server

import "github.com/prometheus/client_golang/prometheus"

// SettlementMetrics holds Prometheus metrics for the settlement service.
type SettlementMetrics struct {
	SettlementsStarted   *prometheus.CounterVec
	SettlementsCompleted *prometheus.CounterVec
	SettlementDuration   *prometheus.HistogramVec
	PrizesDistributed    *prometheus.CounterVec
	PrizesTotalAmount    prometheus.Counter
	ActiveSettlements    prometheus.Gauge
	StuckSettlementsDetected  prometheus.Counter
	OrphanedSettlingDetected  prometheus.Counter
	FailedSettlingDetected    prometheus.Counter
}

// NewSettlementMetrics creates and registers all settlement metrics.
func NewSettlementMetrics(registry prometheus.Registerer) *SettlementMetrics {
	m := &SettlementMetrics{
		SettlementsStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "started_total",
			Help:      "Total number of settlements started",
		}, []string{"source"}),

		SettlementsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "completed_total",
			Help:      "Total number of settlements completed",
		}, []string{"status"}),

		SettlementDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "settlement",
			Name:      "duration_seconds",
			Help:      "Duration of settlement processing in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"status"}),

		PrizesDistributed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "prizes_distributed_total",
			Help:      "Total number of prizes distributed",
		}, []string{"status"}),

		PrizesTotalAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "prizes_amount_cents_total",
			Help:      "Total amount of prizes distributed in cents",
		}),

		ActiveSettlements: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "settlement",
			Name:      "active_count",
			Help:      "Number of currently active settlements",
		}),

		StuckSettlementsDetected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "stuck_detected_total",
			Help:      "Total number of stuck settlements detected and retried",
		}),

		OrphanedSettlingDetected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "orphaned_settling_detected_total",
			Help:      "Total number of orphaned settling contests detected (settling status but no settlement record)",
		}),

		FailedSettlingDetected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "settlement",
			Name:      "failed_settling_detected_total",
			Help:      "Total number of contests with settling status but failed settlement record",
		}),
	}

	registry.MustRegister(
		m.SettlementsStarted,
		m.SettlementsCompleted,
		m.SettlementDuration,
		m.PrizesDistributed,
		m.PrizesTotalAmount,
		m.ActiveSettlements,
		m.StuckSettlementsDetected,
		m.OrphanedSettlingDetected,
		m.FailedSettlingDetected,
	)

	return m
}
