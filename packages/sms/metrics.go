package sms

import "github.com/prometheus/client_golang/prometheus"

var (
	otpSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_otp_sent_total",
			Help: "Total OTP SMS sent",
		},
		[]string{"status"}, // "success", "failure"
	)
	otpVerifiedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_otp_verified_total",
			Help: "Total OTP verifications",
		},
		[]string{"status"}, // "success", "failure", "expired", "blocked"
	)
	smsLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sms_provider_latency_seconds",
			Help:    "SMS provider response latency",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
		},
	)
)

func init() {
	prometheus.MustRegister(otpSentTotal, otpVerifiedTotal, smsLatency)
}
