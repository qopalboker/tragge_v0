package server

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PaymentMetrics holds Prometheus metrics for the payment service.
type PaymentMetrics struct {
	// Payment4 payment creation counter
	Payment4PaymentsCreated prometheus.Counter

	// Payment4 webhooks received (labeled by status)
	Payment4WebhooksReceived *prometheus.CounterVec

	// Payment4 API request duration
	Payment4APIRequestDuration *prometheus.HistogramVec

	// Payment4 verification failures
	Payment4VerificationFailures prometheus.Counter
}

// NewPaymentMetrics creates and registers metrics for the payment service.
func NewPaymentMetrics(registry prometheus.Registerer, namespace string) *PaymentMetrics {
	metrics := &PaymentMetrics{
		Payment4PaymentsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "payment4_payments_created_total",
			Help:      "Total number of payments successfully created via Payment4",
		}),

		Payment4WebhooksReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "payment4_webhooks_received_total",
			Help:      "Total number of Payment4 webhooks received, by status",
		}, []string{"status"}),

		Payment4APIRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "payment4_api_request_duration_seconds",
			Help:      "Duration of HTTP requests to the Payment4 API",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
		}, []string{"method", "endpoint"}),

		Payment4VerificationFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "payment4_verification_failures_total",
			Help:      "Total number of Payment4 webhook verification failures",
		}),
	}

	registry.MustRegister(
		metrics.Payment4PaymentsCreated,
		metrics.Payment4WebhooksReceived,
		metrics.Payment4APIRequestDuration,
		metrics.Payment4VerificationFailures,
	)

	return metrics
}

// ObserveHTTPCall is a callback suitable for Payment4Config.OnHTTPCall.
// It records the duration of an HTTP request to the Payment4 API.
func (m *PaymentMetrics) ObserveHTTPCall(method, endpoint string, duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.Payment4APIRequestDuration.WithLabelValues(method, normalizePayment4Endpoint(endpoint)).Observe(duration.Seconds())
}

// OnPayment4Created increments the payment creation counter.
func (m *PaymentMetrics) OnPayment4Created() {
	if m == nil {
		return
	}
	m.Payment4PaymentsCreated.Inc()
}

// OnPayment4Webhook increments the webhook received counter with the given status label.
func (m *PaymentMetrics) OnPayment4Webhook(status string) {
	if m == nil {
		return
	}
	m.Payment4WebhooksReceived.WithLabelValues(status).Inc()
}

// OnPayment4VerificationFailure increments the verification failure counter.
func (m *PaymentMetrics) OnPayment4VerificationFailure() {
	if m == nil {
		return
	}
	m.Payment4VerificationFailures.Inc()
}

// normalizePayment4Endpoint collapses Payment4 URL paths to avoid high-cardinality labels.
// e.g. "/api/v1/payment/abc123/status" → "/payment/{uid}/status"
func normalizePayment4Endpoint(path string) string {
	// Strip common base path prefixes
	for _, prefix := range []string{"/api/v1/sandbox", "/api/v1"} {
		if strings.HasPrefix(path, prefix) {
			path = path[len(prefix):]
			break
		}
	}
	// Normalize /payment/{uid}/status pattern
	if strings.HasPrefix(path, "/payment/") && strings.HasSuffix(path, "/status") {
		return "/payment/{uid}/status"
	}
	return path
}
