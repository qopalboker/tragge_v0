package notification

import "time"

// Severity represents the severity level of a notification.
type Severity string

const (
	// SeverityCritical indicates a critical issue requiring immediate attention.
	// Examples: system outages, data loss, security breaches
	SeverityCritical Severity = "critical"

	// SeverityHigh indicates a high-priority issue that needs prompt attention.
	// Examples: service degradation, high error rates
	SeverityHigh Severity = "high"

	// SeverityMedium indicates a medium-priority issue.
	// Examples: elevated latency, resource warnings
	SeverityMedium Severity = "medium"

	// SeverityLow indicates a low-priority issue.
	// Examples: minor issues, non-urgent warnings
	SeverityLow Severity = "low"

	// SeverityInfo indicates informational notifications.
	// Examples: deployments, scheduled maintenance notices
	SeverityInfo Severity = "info"
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}

// IsValid checks if the severity is a valid value.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo:
		return true
	default:
		return false
	}
}

// AlertType represents the type of alert.
type AlertType string

const (
	AlertTypeBug     AlertType = "bug"
	AlertTypeContest AlertType = "contest"
	AlertTypeSystem  AlertType = "system"
	AlertTypeTrade   AlertType = "trade"
	AlertTypeUser    AlertType = "user"
	AlertTypeCustom  AlertType = "custom"
)

// String returns the string representation of the alert type.
func (t AlertType) String() string {
	return string(t)
}

// Alert represents a notification alert that requires attention.
type Alert struct {
	// ID is the unique identifier for this alert (optional, auto-generated if empty)
	ID string `json:"id,omitempty"`

	// Type is the category of this alert
	Type AlertType `json:"type"`

	// Severity indicates the importance level
	Severity Severity `json:"severity"`

	// Title is a short summary of the alert
	Title string `json:"title"`

	// Message is the detailed description
	Message string `json:"message"`

	// Service is the name of the service that generated this alert
	Service string `json:"service,omitempty"`

	// Timestamp is when the alert was created
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional key-value pairs for context
	Metadata map[string]string `json:"metadata,omitempty"`

	// TraceID links to distributed tracing (if available)
	TraceID string `json:"trace_id,omitempty"`

	// SpanID links to distributed tracing (if available)
	SpanID string `json:"span_id,omitempty"`
}

// Info represents an informational notification (non-urgent).
type Info struct {
	// ID is the unique identifier for this info (optional, auto-generated if empty)
	ID string `json:"id,omitempty"`

	// Title is a short summary
	Title string `json:"title"`

	// Message is the detailed content
	Message string `json:"message"`

	// Service is the name of the service that generated this info
	Service string `json:"service,omitempty"`

	// Timestamp is when the info was created
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional key-value pairs for context
	Metadata map[string]string `json:"metadata,omitempty"`
}

// BugAlert creates a new bug alert with the given details.
func BugAlert(severity Severity, title, message string) Alert {
	return Alert{
		Type:      AlertTypeBug,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

// ContestAlert creates a new contest-related alert.
func ContestAlert(severity Severity, title, message string, contestID string) Alert {
	return Alert{
		Type:      AlertTypeContest,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"contest_id": contestID,
		},
	}
}

// SystemAlert creates a new system-level alert.
func SystemAlert(severity Severity, title, message string) Alert {
	return Alert{
		Type:      AlertTypeSystem,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

// TradeAlert creates a new trade-related alert.
func TradeAlert(severity Severity, title, message string, userID, orderID string) Alert {
	metadata := make(map[string]string)
	if userID != "" {
		metadata["user_id"] = userID
	}
	if orderID != "" {
		metadata["order_id"] = orderID
	}
	return Alert{
		Type:      AlertTypeTrade,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
}

// UserAlert creates a new user-related alert.
func UserAlert(severity Severity, title, message string, userID string) Alert {
	return Alert{
		Type:      AlertTypeUser,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"user_id": userID,
		},
	}
}

// CustomAlert creates a custom alert with the given type.
func CustomAlert(alertType AlertType, severity Severity, title, message string) Alert {
	return Alert{
		Type:      alertType,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

// WithService adds service information to an alert.
func (a Alert) WithService(service string) Alert {
	a.Service = service
	return a
}

// WithMetadata adds metadata to an alert.
func (a Alert) WithMetadata(key, value string) Alert {
	if a.Metadata == nil {
		a.Metadata = make(map[string]string)
	}
	a.Metadata[key] = value
	return a
}

// WithTracing adds trace information to an alert.
func (a Alert) WithTracing(traceID, spanID string) Alert {
	a.TraceID = traceID
	a.SpanID = spanID
	return a
}

// NewInfo creates a new informational notification.
func NewInfo(title, message string) Info {
	return Info{
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

// WithService adds service information to an info notification.
func (i Info) WithService(service string) Info {
	i.Service = service
	return i
}

// WithMetadata adds metadata to an info notification.
func (i Info) WithMetadata(key, value string) Info {
	if i.Metadata == nil {
		i.Metadata = make(map[string]string)
	}
	i.Metadata[key] = value
	return i
}

// Channel represents a notification delivery channel.
type Channel string

const (
	// ChannelDiscord sends notifications to Discord via webhook
	ChannelDiscord Channel = "discord"

	// ChannelEmail sends notifications via email (Resend)
	ChannelEmail Channel = "email"
)

// String returns the string representation of the channel.
func (c Channel) String() string {
	return string(c)
}

// NotificationResult represents the result of a notification send attempt.
type NotificationResult struct {
	// Success indicates whether the notification was sent successfully
	Success bool `json:"success"`

	// Channel indicates which channel was used
	Channel Channel `json:"channel"`

	// MessageID is the identifier returned by the channel (if available)
	MessageID string `json:"message_id,omitempty"`

	// Error contains error details if the send failed
	Error error `json:"error,omitempty"`

	// Timestamp is when the result was recorded
	Timestamp time.Time `json:"timestamp"`
}
