// Package v1 contains versioned contract types for the trading platform.
package v1

// OrderSide represents the side of an order (buy or sell).
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// OrderMode represents the execution mode of an order.
type OrderMode string

const (
	OrderModeMarket  OrderMode = "MARKET"  // Execute immediately at current price
	OrderModePending OrderMode = "PENDING" // Execute when condition is met
)

// OrderType represents the type of an order.
type OrderType string

const (
	OrderTypeMarket    OrderType = "MARKET"     // Immediate execution at current price
	OrderTypeBuyLimit  OrderType = "BUY_LIMIT"  // Pending: trigger when ask <= limit_price
	OrderTypeSellLimit OrderType = "SELL_LIMIT" // Pending: trigger when bid >= limit_price
	OrderTypeBuyStop   OrderType = "BUY_STOP"   // Pending: trigger when ask >= stop_price
	OrderTypeSellStop  OrderType = "SELL_STOP"  // Pending: trigger when bid <= stop_price

	// Legacy types (deprecated, kept for backward compatibility)
	OrderTypeLimit OrderType = "LIMIT"
	OrderTypeStop  OrderType = "STOP"
)

// IsPending returns true if the order type is a pending order.
func (ot OrderType) IsPending() bool {
	switch ot {
	case OrderTypeBuyLimit, OrderTypeSellLimit, OrderTypeBuyStop, OrderTypeSellStop,
		OrderTypeLimit, OrderTypeStop:
		return true
	default:
		return false
	}
}

// GetMode returns the order mode for this order type.
func (ot OrderType) GetMode() OrderMode {
	if ot.IsPending() {
		return OrderModePending
	}
	return OrderModeMarket
}

// OrderStatus represents the acknowledgment status of an order.
type OrderStatus string

const (
	OrderStatusAccepted OrderStatus = "ACCEPTED"
	OrderStatusRejected OrderStatus = "REJECTED"
)

// ContestPhase represents the current phase of a contest for Kafka events.
// This is a simplified view of the contest lifecycle for event consumers.
type ContestPhase string

const (
	ContestPhaseUpcoming  ContestPhase = "UPCOMING"  // Draft, Scheduled, RegistrationOpen, RegistrationClosed
	ContestPhaseLive      ContestPhase = "LIVE"      // Running
	ContestPhaseFrozen    ContestPhase = "FROZEN"    // Paused, Settling
	ContestPhaseEnded     ContestPhase = "ENDED"     // Completed, Cancelled
	ContestPhaseCancelled ContestPhase = "CANCELLED" // Cancelled (with refunds)
)

// ContestStatus represents the detailed lifecycle status of a contest.
// This matches the database enum and provides finer-grained state tracking.
type ContestStatus string

const (
	// Draft is the initial state when a contest is created but not published.
	ContestStatusDraft ContestStatus = "draft"

	// Scheduled is when the contest is published and accepting registrations.
	ContestStatusScheduled ContestStatus = "scheduled"

	// RegistrationOpen is when registration is actively open.
	ContestStatusRegistrationOpen ContestStatus = "registration_open"

	// RegistrationClosed is when registration deadline passed or contest is full.
	ContestStatusRegistrationClosed ContestStatus = "registration_closed"

	// Running is the active trading period.
	ContestStatusRunning ContestStatus = "running"

	// Paused is when the contest is temporarily frozen (admin action).
	ContestStatusPaused ContestStatus = "paused"

	// Settling is when trading ended and results are being calculated.
	ContestStatusSettling ContestStatus = "settling"

	// Completed is when settlement is done and prizes distributed.
	ContestStatusCompleted ContestStatus = "completed"

	// Cancelled is when the contest was cancelled (refunds issued).
	ContestStatusCancelled ContestStatus = "cancelled"
)

// IsValid returns true if the contest status is a valid value.
func (cs ContestStatus) IsValid() bool {
	switch cs {
	case ContestStatusDraft, ContestStatusScheduled, ContestStatusRegistrationOpen,
		ContestStatusRegistrationClosed, ContestStatusRunning, ContestStatusPaused,
		ContestStatusSettling, ContestStatusCompleted, ContestStatusCancelled:
		return true
	default:
		return false
	}
}

// ToPhase converts a ContestStatus to its corresponding ContestPhase.
func (cs ContestStatus) ToPhase() ContestPhase {
	switch cs {
	case ContestStatusDraft, ContestStatusScheduled, ContestStatusRegistrationOpen, ContestStatusRegistrationClosed:
		return ContestPhaseUpcoming
	case ContestStatusRunning:
		return ContestPhaseLive
	case ContestStatusPaused, ContestStatusSettling:
		return ContestPhaseFrozen
	case ContestStatusCompleted:
		return ContestPhaseEnded
	case ContestStatusCancelled:
		return ContestPhaseCancelled
	default:
		return ContestPhaseUpcoming
	}
}

// IsFinal returns true if the contest status is a final state.
func (cs ContestStatus) IsFinal() bool {
	return cs == ContestStatusCompleted || cs == ContestStatusCancelled
}

// AllowsTrading returns true if the contest status allows trading.
func (cs ContestStatus) AllowsTrading() bool {
	return cs == ContestStatusRunning
}

// AllowsRegistration returns true if the contest status allows registration.
func (cs ContestStatus) AllowsRegistration() bool {
	return cs == ContestStatusScheduled || cs == ContestStatusRegistrationOpen
}

// CancelReason represents the reason for order cancellation.
type CancelReason string

const (
	CancelReasonUserRequested     CancelReason = "user_requested"     // User manually cancelled the order
	CancelReasonContestEnded      CancelReason = "contest_ended"      // Auto-cancelled when contest ends
	CancelReasonInsufficientFunds CancelReason = "insufficient_funds" // Balance check failed
	CancelReasonExpired           CancelReason = "expired"            // Order reached expiration time
)

// AssetClass represents the category of tradable assets.
type AssetClass string

const (
	AssetClassForex  AssetClass = "forex"
	AssetClassCrypto AssetClass = "crypto"
	AssetClassStocks AssetClass = "stocks"
	AssetClassMixed  AssetClass = "mixed"
)

// IsValid returns true if the asset class is a valid value.
func (ac AssetClass) IsValid() bool {
	switch ac {
	case AssetClassForex, AssetClassCrypto, AssetClassStocks, AssetClassMixed:
		return true
	default:
		return false
	}
}

// ContestDurationType represents the duration category of a contest.
type ContestDurationType string

const (
	ContestDurationRush30Min ContestDurationType = "rush_30min"
	ContestDurationHourly    ContestDurationType = "hourly"
	ContestDurationFourHour  ContestDurationType = "four_hour"
	ContestDurationDaily     ContestDurationType = "daily"
	ContestDurationWeekly    ContestDurationType = "weekly"
)

// IsValid returns true if the duration type is a valid value.
func (dt ContestDurationType) IsValid() bool {
	switch dt {
	case ContestDurationRush30Min, ContestDurationHourly, ContestDurationFourHour, ContestDurationDaily, ContestDurationWeekly:
		return true
	default:
		return false
	}
}

// DurationMinutes returns the standard duration in minutes for this type.
func (dt ContestDurationType) DurationMinutes() int {
	switch dt {
	case ContestDurationRush30Min:
		return 30
	case ContestDurationHourly:
		return 60
	case ContestDurationFourHour:
		return 240
	case ContestDurationDaily:
		return 1440
	case ContestDurationWeekly:
		return 10080
	default:
		return 60 // default to hourly
	}
}

// DefaultQtyAllocation returns the recommended QTY allocation for this duration type.
func (dt ContestDurationType) DefaultQtyAllocation() int64 {
	switch dt {
	case ContestDurationRush30Min:
		return 50000 // 10 QTY equivalent in cents
	case ContestDurationHourly:
		return 100000
	case ContestDurationFourHour:
		return 150000
	case ContestDurationDaily:
		return 200000 // 20 QTY equivalent
	case ContestDurationWeekly:
		return 500000
	default:
		return 100000
	}
}
