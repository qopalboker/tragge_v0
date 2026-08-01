package v1

// AlertType represents the type of system alert.
type AlertType string

const (
	// AlertTypePriceStale indicates that price data for a symbol is stale.
	AlertTypePriceStale AlertType = "PRICE_STALE"
)

// AlertSeverity represents the severity level of an alert.
type AlertSeverity string

const (
	AlertSeverityWarning  AlertSeverity = "WARNING"
	AlertSeverityCritical AlertSeverity = "CRITICAL"
)

// PriceStaleAlert represents an alert event when price data becomes stale.
// Published to the alerts.v1 Kafka topic.
type PriceStaleAlert struct {
	// AlertID is a unique identifier for this alert instance.
	AlertID string `json:"alert_id"`

	// Type is the alert type (PRICE_STALE).
	Type AlertType `json:"type"`

	// Severity indicates the severity level of the alert.
	Severity AlertSeverity `json:"severity"`

	// Symbol is the trading symbol with stale price data.
	Symbol string `json:"symbol"`

	// LastUpdateTs is the Unix timestamp (milliseconds) of the last price update.
	LastUpdateTs int64 `json:"last_update_ts"`

	// AgeSeconds is the number of seconds since the last price update.
	AgeSeconds float64 `json:"age_seconds"`

	// ThresholdSeconds is the staleness threshold that was exceeded.
	ThresholdSeconds float64 `json:"threshold_seconds"`

	// Source identifies the service that generated the alert.
	Source string `json:"source"`

	// Ts is the Unix timestamp (milliseconds) when the alert was generated.
	Ts int64 `json:"ts"`
}
