// Package kyc provides KYC (Know Your Customer) verification services for the tragge platform.
//
// Features:
//   - KYC status verification
//   - Expiration checking
//   - Shared service for use across multiple services
//
// Example usage:
//
//	svc := kyc.NewService(db)
//
//	// Check if user is verified
//	result, err := svc.CheckVerification(ctx, userID)
//	if err != nil {
//	    return err
//	}
//	if !result.Verified {
//	    // Handle unverified user
//	}
package kyc

import (
	"time"
)

// Status represents the KYC verification status.
type Status string

const (
	StatusNone        Status = "none"
	StatusPending     Status = "pending"
	StatusUnderReview Status = "under_review"
	StatusVerified    Status = "verified"
	StatusRejected    Status = "rejected"
	StatusExpired     Status = "expired"
)

// VerificationResult contains the result of a KYC verification check.
type VerificationResult struct {
	// Verified indicates if the user has a valid, non-expired KYC verification.
	Verified bool `json:"verified"`

	// Status is the current KYC status.
	Status Status `json:"status"`

	// ExpiresAt is when the verification expires (if verified).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// RejectionReason is the reason for rejection (if rejected).
	RejectionReason *string `json:"rejection_reason,omitempty"`

	// Message provides a human-readable description of the verification state.
	Message string `json:"message,omitempty"`
}

// CanWithdraw returns true if the verification status allows withdrawals.
func (r *VerificationResult) CanWithdraw() bool {
	return r.Verified
}

// KYCRequiredError is returned when KYC verification is required but not complete.
type KYCRequiredError struct {
	Status  Status
	Message string
}

func (e *KYCRequiredError) Error() string {
	return e.Message
}

// KYCExpiredError is returned when KYC verification has expired.
type KYCExpiredError struct {
	ExpiredAt time.Time
	Message   string
}

func (e *KYCExpiredError) Error() string {
	return e.Message
}
