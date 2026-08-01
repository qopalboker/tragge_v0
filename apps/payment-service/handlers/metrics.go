package handlers

// PaymentMetricsObserver is an interface for recording payment metrics.
// Implemented by *PaymentMetrics in the main package.
type PaymentMetricsObserver interface {
	OnPayment4Created()
	OnPayment4Webhook(status string)
	OnPayment4VerificationFailure()
}
