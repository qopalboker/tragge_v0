package kyc

import "github.com/prometheus/client_golang/prometheus"

var (
	ValidationFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tragge",
			Subsystem: "kyc",
			Name:      "validation_failures_total",
			Help:      "Total number of KYC validation failures by type",
		},
		[]string{"validation_type", "reason"},
	)
)

func init() {
	prometheus.MustRegister(ValidationFailures)
}
