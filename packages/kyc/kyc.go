package kyc

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// TxExecutor is an interface for executing database operations.
// It can be either a *sql.DB or *sql.Tx.
type TxExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// Service provides KYC verification operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new KYC service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// CheckVerification checks the KYC verification status for a user.
// It returns a VerificationResult indicating whether the user is verified
// and can perform actions that require KYC (like withdrawals).
func (s *Service) CheckVerification(ctx context.Context, userID string) (*VerificationResult, error) {
	return s.checkVerification(ctx, s.db, userID)
}

// CheckVerificationTx checks the KYC verification status within a transaction.
func (s *Service) CheckVerificationTx(ctx context.Context, tx TxExecutor, userID string) (*VerificationResult, error) {
	return s.checkVerification(ctx, tx, userID)
}

func (s *Service) checkVerification(ctx context.Context, exec TxExecutor, userID string) (*VerificationResult, error) {
	var status string
	var expiresAt sql.NullTime
	var rejectionReason sql.NullString

	err := exec.QueryRowContext(ctx, `
		SELECT status, expires_at, rejection_reason
		FROM user_verification
		WHERE user_id = $1
	`, userID).Scan(&status, &expiresAt, &rejectionReason)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No KYC record exists - user has not started verification
			return &VerificationResult{
				Verified: false,
				Status:   StatusNone,
				Message:  "KYC verification required. Please submit your documents to enable withdrawals.",
			}, nil
		}
		return nil, err
	}

	result := &VerificationResult{
		Status: Status(status),
	}

	if expiresAt.Valid {
		result.ExpiresAt = &expiresAt.Time
	}

	if rejectionReason.Valid {
		result.RejectionReason = &rejectionReason.String
	}

	// Determine verification status based on status and expiry
	switch Status(status) {
	case StatusVerified:
		// Check if verification has expired
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			result.Verified = false
			result.Status = StatusExpired
			result.Message = "Your KYC verification has expired. Please reverify to enable withdrawals."
		} else {
			result.Verified = true
			result.Message = "KYC verified"
		}

	case StatusPending:
		result.Verified = false
		result.Message = "Your KYC submission is pending review. Please wait for verification."

	case StatusUnderReview:
		result.Verified = false
		result.Message = "Your KYC is under review. Please wait for verification."

	case StatusRejected:
		result.Verified = false
		msg := "Your KYC verification was rejected."
		if rejectionReason.Valid && rejectionReason.String != "" {
			msg += " Reason: " + rejectionReason.String
		}
		msg += " Please resubmit with valid documents."
		result.Message = msg

	case StatusExpired:
		result.Verified = false
		result.Message = "Your KYC verification has expired. Please reverify to enable withdrawals."

	default:
		result.Verified = false
		result.Message = "KYC verification required. Please submit your documents to enable withdrawals."
	}

	return result, nil
}

// RequireVerification checks if a user is verified and returns an error if not.
// This is a convenience method for handlers that need to enforce KYC.
func (s *Service) RequireVerification(ctx context.Context, userID string) error {
	result, err := s.CheckVerification(ctx, userID)
	if err != nil {
		return err
	}

	if !result.Verified {
		if result.ExpiresAt != nil && (result.Status == StatusExpired || result.ExpiresAt.Before(time.Now())) {
			return &KYCExpiredError{
				ExpiredAt: *result.ExpiresAt,
				Message:   result.Message,
			}
		}
		if result.Status == StatusExpired && result.ExpiresAt == nil {
			return &KYCExpiredError{
				ExpiredAt: time.Now(),
				Message:   result.Message,
			}
		}
		return &KYCRequiredError{
			Status:  result.Status,
			Message: result.Message,
		}
	}

	return nil
}

// GetStatus returns just the KYC status for a user (for display purposes).
func (s *Service) GetStatus(ctx context.Context, userID string) (Status, error) {
	var status string

	err := s.db.QueryRowContext(ctx, `
		SELECT status FROM user_verification WHERE user_id = $1
	`, userID).Scan(&status)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StatusNone, nil
		}
		return "", err
	}

	return Status(status), nil
}

// IsWithdrawalAllowed checks if a user can perform withdrawals based on KYC status.
// Returns (canWithdraw, kycStatus, message).
func (s *Service) IsWithdrawalAllowed(ctx context.Context, userID string) (bool, Status, string) {
	result, err := s.CheckVerification(ctx, userID)
	if err != nil {
		return false, StatusNone, "Unable to verify KYC status"
	}

	return result.Verified, result.Status, result.Message
}
