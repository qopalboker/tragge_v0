package providers

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitExecutor wraps function execution through a circuit breaker.
// Satisfied by *circuitbreaker.CircuitBreaker.
type CircuitExecutor interface {
	ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error
}

// Common errors
var (
	ErrPaymentNotFound     = errors.New("payment not found")
	ErrPaymentFailed       = errors.New("payment failed")
	ErrInvalidSignature    = errors.New("invalid webhook signature")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrProviderUnavailable = errors.New("payment provider unavailable")
	ErrUnsupportedCurrency = errors.New("unsupported currency")
)

// ProviderType represents a payment provider type
type ProviderType string

const (
	ProviderNowPayments ProviderType = "nowpayments"
	ProviderJibit       ProviderType = "jibit"
)

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusWaiting    PaymentStatus = "waiting"
	PaymentStatusConfirming PaymentStatus = "confirming"
	PaymentStatusConfirmed  PaymentStatus = "confirmed"
	PaymentStatusSending    PaymentStatus = "sending"
	PaymentStatusFinished   PaymentStatus = "finished"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusRefunded   PaymentStatus = "refunded"
	PaymentStatusExpired    PaymentStatus = "expired"
)

// CreatePaymentRequest represents a request to create a payment
type CreatePaymentRequest struct {
	// Amount in smallest currency unit (cents for USD, Rials for IRR)
	AmountCents    int64
	Currency       string // USD, IRR, etc.
	UserID         string
	OrderID        string // Internal order/payment intent ID
	Description    string
	CallbackURL    string // Success redirect URL
	CancelURL      string // Cancel/failure redirect URL
	IPNCallbackURL string // For crypto webhooks
	CustomerEmail  string
	CustomerPhone  string

	// Crypto-specific fields
	PayCurrency string // BTC, ETH, USDT, etc.
}

// CreatePaymentResponse represents the response from creating a payment
type CreatePaymentResponse struct {
	ProviderPaymentID string            // ID from the payment provider
	PaymentURL        string            // URL to redirect user for payment
	PayAddress        string            // For crypto payments: wallet address
	PayAmount         float64           // For crypto: amount in crypto currency
	PayCurrency       string            // For crypto: currency code (BTC, ETH, etc.)
	QRCode            string            // QR code URL/data for crypto payments
	ExpiresAt         int64             // Unix timestamp when payment expires
	Status            PaymentStatus     // Initial status
	Metadata          map[string]string // Additional provider-specific data
}

// PaymentStatusResponse represents the status of a payment
type PaymentStatusResponse struct {
	ProviderPaymentID string
	Status            PaymentStatus
	AmountCents       int64
	Currency          string
	PaidAmountCents   int64  // Actual amount paid (may differ for crypto)
	RefNumber         string // Bank reference number (for fiat)
	Metadata          map[string]string
}

// WebhookEvent represents a parsed webhook event
type WebhookEvent struct {
	Provider          ProviderType
	ProviderPaymentID string
	OrderID           string // Internal order ID if available
	Status            PaymentStatus
	AmountCents       int64
	Currency          string
	PaidAmountCents   int64
	RefNumber         string
	RawData           map[string]interface{}
}

// PayoutRequest represents a request to create a payout
type PayoutRequest struct {
	AmountCents int64
	Currency    string
	UserID      string
	PayoutID    string // Internal payout ID
	Description string

	// Bank transfer fields (Jibit)
	BankAccount   string // IBAN or account number
	BankName      string
	AccountHolder string

	// Crypto payout fields (NOWPayments)
	WalletAddress  string
	CryptoCurrency string // BTC, ETH, USDT, etc.
}

// PayoutResponse represents the response from creating a payout
type PayoutResponse struct {
	ProviderPayoutID string
	Status           PaymentStatus
	Metadata         map[string]string
}

// RefundRequest represents a request to refund a payment
type RefundRequest struct {
	ProviderPaymentID string // purchaseId from the provider
	OrderID           string // clientReferenceNumber (internal reference)
	AmountCents       int64  // Partial refund amount; 0 means full refund
	Cancellable       bool   // Whether the refund itself can be cancelled
}

// RefundResponse represents the response from a refund or reverse operation
type RefundResponse struct {
	Status   PaymentStatus
	Metadata map[string]string
}

// Provider is the interface that payment providers must implement
type Provider interface {
	// Name returns the provider name
	Name() ProviderType

	// CreatePayment creates a new payment and returns payment details
	CreatePayment(ctx context.Context, req *CreatePaymentRequest) (*CreatePaymentResponse, error)

	// GetPaymentStatus retrieves the current status of a payment
	GetPaymentStatus(ctx context.Context, providerPaymentID string) (*PaymentStatusResponse, error)

	// VerifyWebhook verifies the webhook signature and parses the event
	// Returns the parsed event if valid, error if invalid signature
	VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error)

	// CreatePayout creates a withdrawal/payout (optional - not all providers support this)
	CreatePayout(ctx context.Context, req *PayoutRequest) (*PayoutResponse, error)

	// GetPayoutStatus retrieves the status of a payout
	GetPayoutStatus(ctx context.Context, providerPayoutID string) (*PayoutResponse, error)

	// RefundPayment issues a partial or full refund for a completed payment
	RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error)

	// ReversePayment fully reverses a payment (must be called before settlement)
	ReversePayment(ctx context.Context, purchaseID string) (*RefundResponse, error)

	// IsAvailable checks if the provider is currently available
	IsAvailable(ctx context.Context) bool

	// SupportedCurrencies returns the list of supported currencies
	SupportedCurrencies() []string
}

const availabilityCacheTTL = 30 * time.Second

type cachedStatus struct {
	available bool
	checkedAt time.Time
}

// MapStatusToIntentStatus maps provider PaymentStatus to payment intent status string.
func MapStatusToIntentStatus(status PaymentStatus) string {
	switch status {
	case PaymentStatusPending, PaymentStatusWaiting:
		return "pending"
	case PaymentStatusConfirming, PaymentStatusSending:
		return "processing"
	case PaymentStatusFinished, PaymentStatusConfirmed:
		return "succeeded"
	case PaymentStatusFailed:
		return "failed"
	case PaymentStatusRefunded:
		return "refunded"
	case PaymentStatusExpired:
		return "expired"
	default:
		return "pending"
	}
}

// ProviderRegistry manages multiple payment providers
type ProviderRegistry struct {
	providers      map[ProviderType]Provider
	mu             sync.RWMutex
	availableCache map[ProviderType]cachedStatus
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:      make(map[ProviderType]Provider),
		availableCache: make(map[ProviderType]cachedStatus),
	}
}

// Register registers a payment provider
func (r *ProviderRegistry) Register(provider Provider) {
	r.providers[provider.Name()] = provider
}

// Get returns a provider by type
func (r *ProviderRegistry) Get(providerType ProviderType) (Provider, bool) {
	p, ok := r.providers[providerType]
	return p, ok
}

// GetAvailable returns the first available provider for the given currency
func (r *ProviderRegistry) GetAvailable(ctx context.Context, currency string) (Provider, bool) {
	for _, p := range r.providers {
		if !p.IsAvailable(ctx) {
			continue
		}
		for _, c := range p.SupportedCurrencies() {
			if c == currency {
				return p, true
			}
		}
	}
	return nil, false
}

// All returns all registered providers
func (r *ProviderRegistry) All() []Provider {
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// IsProviderAvailable checks provider availability with a cached result (30s TTL).
// This avoids making external API calls on every readiness probe.
func (r *ProviderRegistry) IsProviderAvailable(ctx context.Context, providerType ProviderType) bool {
	r.mu.RLock()
	cached, ok := r.availableCache[providerType]
	r.mu.RUnlock()

	if ok && time.Since(cached.checkedAt) < availabilityCacheTTL {
		return cached.available
	}

	p, exists := r.providers[providerType]
	if !exists {
		return false
	}

	available := p.IsAvailable(ctx)

	r.mu.Lock()
	r.availableCache[providerType] = cachedStatus{
		available: available,
		checkedAt: time.Now(),
	}
	r.mu.Unlock()

	return available
}
