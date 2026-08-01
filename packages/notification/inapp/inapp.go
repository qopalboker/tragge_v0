// Package inapp provides in-app notification database operations.
// This package handles creating, reading, and managing user notifications
// stored in the notifications table.
package inapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/google/uuid"
)

// batchChunkSize is the maximum number of rows per multi-row INSERT.
// PostgreSQL supports up to 65535 parameters; with 6 params per row,
// 500 rows (3000 params) is a safe batch size.
const batchChunkSize = 500

// Common errors for in-app notifications.
var (
	ErrNotificationNotFound    = errors.New("notification not found")
	ErrNotificationNotOwned    = errors.New("notification does not belong to user")
	ErrPartialScanFailure      = errors.New("some notification rows failed to scan")
)

// Notification types.
const (
	NotifTypeContestStarting  = "contest_starting"
	NotifTypeContestEnding    = "contest_ending"
	NotifTypeContestCompleted = "contest_completed"
	NotifTypeContestCancelled = "contest_cancelled"
	NotifTypeContestJoined    = "contest_joined"
	NotifTypeContestLeft      = "contest_left"
	NotifTypePrizeWon         = "prize_won"
	NotifTypeWithdrawalUpdate = "withdrawal_update"
	NotifTypeDepositConfirmed = "deposit_confirmed"
	NotifTypeDepositFailed    = "deposit_failed"
	NotifTypeKYCUpdate        = "kyc_update"
	NotifTypeSystem           = "system"
	NotifTypeTicketReply      = "ticket_reply"
	NotifTypePasswordChanged  = "password_changed"
)

// Notification represents an in-app notification.
type Notification struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	ReadAt    *time.Time             `json:"read_at,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// Querier is a minimal interface for read operations (satisfied by *sql.DB, *sql.Tx, and pool.Replica()).
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Execer is a minimal interface for write operations (satisfied by *sql.DB, *sql.Tx, and pool.Primary()).
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ReadWriter combines read and write operations (satisfied by *sql.DB, *sql.Tx).
type ReadWriter interface {
	Execer
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CreateNotification creates a new in-app notification for a user.
// The db parameter should be pool.Primary() or a transaction for write operations.
func CreateNotification(ctx context.Context, db Execer, userID, notifType, title, message string, metadata map[string]interface{}) error {
	id := uuid.New().String()

	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO notifications (id, user_id, type, title, message, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, id, userID, notifType, title, message, metadataJSON)

	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

// CreateNotificationIfEnabled creates a notification only if user preferences allow it.
// prefsDB is used for reading preferences (e.g., pool.Replica()), writeDB for writing the notification.
func CreateNotificationIfEnabled(ctx context.Context, prefsDB prefs.Querier, writeDB Execer, userID, notifType, title, message string, metadata map[string]interface{}) error {
	enabled, err := prefs.IsEnabled(ctx, prefsDB, userID, notifType, "in_app")
	if err != nil {
		// Fail closed: skip notification if we can't verify preferences
		return fmt.Errorf("failed to check notification preference: %w", err)
	}
	if !enabled {
		return nil // User disabled this category for in_app
	}
	return CreateNotification(ctx, writeDB, userID, notifType, title, message, metadata)
}

// CreateNotificationBatch creates notifications for multiple users using multi-row INSERTs.
// This is useful for sending the same notification to many users (e.g., contest reminders).
// Rows are inserted in chunks of batchChunkSize to stay within PostgreSQL's parameter limit.
func CreateNotificationBatch(ctx context.Context, db Execer, userIDs []string, notifType, title, message string, metadata map[string]interface{}) error {
	if len(userIDs) == 0 {
		return nil
	}

	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	var failures int
	for i := 0; i < len(userIDs); i += batchChunkSize {
		end := i + batchChunkSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		chunk := userIDs[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO notifications (id, user_id, type, title, message, metadata) VALUES ")
		args := make([]any, 0, len(chunk)*6)

		for j, userID := range chunk {
			if j > 0 {
				sb.WriteString(", ")
			}
			base := j * 6
			fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d, $%d::jsonb)", base+1, base+2, base+3, base+4, base+5, base+6)
			args = append(args, uuid.New().String(), userID, notifType, title, message, metadataJSON)
		}

		_, err := db.ExecContext(ctx, sb.String(), args...)
		if err != nil {
			failures += len(chunk)
		}
	}

	if failures > 0 {
		return fmt.Errorf("failed to create %d/%d notifications", failures, len(userIDs))
	}

	return nil
}

// GetNotifications retrieves notifications for a user with pagination.
// If unreadOnly is true, only unread notifications are returned.
// The db parameter should be pool.Replica() for read operations.
// Returns the list of notifications and the total count.
func GetNotifications(ctx context.Context, db Querier, userID string, limit, offset int, unreadOnly bool) ([]Notification, int, error) {
	// Apply defaults
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Build query based on unreadOnly flag
	var countQuery, selectQuery string
	var args []interface{}

	if unreadOnly {
		countQuery = `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`
		selectQuery = `
			SELECT id, user_id, type, title, message, metadata, read_at, created_at
			FROM notifications
			WHERE user_id = $1 AND read_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{userID, limit, offset}
	} else {
		countQuery = `SELECT COUNT(*) FROM notifications WHERE user_id = $1`
		selectQuery = `
			SELECT id, user_id, type, title, message, metadata, read_at, created_at
			FROM notifications
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{userID, limit, offset}
	}

	// Get total count
	var total int
	err := db.QueryRowContext(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count notifications: %w", err)
	}

	// Get notifications
	rows, err := db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]Notification, 0)
	var scanErrors int
	for rows.Next() {
		var n Notification
		var message, metadataJSON sql.NullString
		var readAt sql.NullTime

		err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &message, &metadataJSON, &readAt, &n.CreatedAt)
		if err != nil {
			scanErrors++
			continue
		}

		if message.Valid {
			n.Message = message.String
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &n.Metadata); err != nil {
				// Skip invalid metadata but keep the notification
				n.Metadata = nil
			}
		}

		if readAt.Valid {
			n.ReadAt = &readAt.Time
		}

		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate notifications: %w", err)
	}

	if scanErrors > 0 {
		return notifications, total, fmt.Errorf("%w: %d rows failed", ErrPartialScanFailure, scanErrors)
	}

	return notifications, total, nil
}

// GetUnreadCount returns the count of unread notifications for a user.
// The db parameter should be pool.Replica() for read operations.
func GetUnreadCount(ctx context.Context, db Querier, userID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications
		WHERE user_id = $1 AND read_at IS NULL
	`, userID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return count, nil
}

// MarkAsRead marks a single notification as read.
// Returns ErrNotificationNotFound if the notification doesn't exist,
// ErrNotificationNotOwned if the notification belongs to a different user,
// or nil if the notification was marked as read (or was already read).
func MarkAsRead(ctx context.Context, db ReadWriter, notificationID, userID string) error {
	result, err := db.ExecContext(ctx, `
		UPDATE notifications
		SET read_at = NOW()
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL
	`, notificationID, userID)

	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// Disambiguate: already read, wrong user, or doesn't exist
		var ownerID string
		err := db.QueryRowContext(ctx, `
			SELECT user_id FROM notifications WHERE id = $1
		`, notificationID).Scan(&ownerID)

		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotificationNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to check notification existence: %w", err)
		}
		if ownerID != userID {
			return ErrNotificationNotOwned
		}
		// Notification exists, belongs to user, but already read — idempotent success
		return nil
	}

	return nil
}

// MarkAllAsRead marks all unread notifications as read for a user.
// Returns the number of notifications that were marked as read.
func MarkAllAsRead(ctx context.Context, db Execer, userID string) (int, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE notifications
		SET read_at = NOW()
		WHERE user_id = $1 AND read_at IS NULL
	`, userID)

	if err != nil {
		return 0, fmt.Errorf("failed to mark all notifications as read: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// DeleteNotification deletes a single notification.
// Returns ErrNotificationNotOwned if the notification belongs to a different user.
func DeleteNotification(ctx context.Context, db Execer, notificationID, userID string) error {
	result, err := db.ExecContext(ctx, `
		DELETE FROM notifications
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)

	if err != nil {
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// CleanupResult holds the outcome of a notification cleanup operation.
type CleanupResult struct {
	// ReadExpired is the number of read notifications deleted (older than readRetentionDays).
	ReadExpired int64
	// MaxExpired is the number of notifications deleted (older than maxRetentionDays, regardless of read status).
	MaxExpired int64
}

// CleanupOldNotifications removes stale notifications in two passes:
//  1. Read notifications older than readRetentionDays are deleted.
//  2. All notifications (read or unread) older than maxRetentionDays are deleted.
//
// This prevents unbounded table growth. The idx_notifications_created_at index
// is used for efficient range scans.
func CleanupOldNotifications(ctx context.Context, db Execer, readRetentionDays, maxRetentionDays int) (*CleanupResult, error) {
	result := &CleanupResult{}

	// Pass 1: delete read notifications older than readRetentionDays
	res, err := db.ExecContext(ctx, `
		DELETE FROM notifications
		WHERE read_at IS NOT NULL
		  AND created_at < NOW() - ($1 || ' days')::interval
	`, readRetentionDays)
	if err != nil {
		return nil, fmt.Errorf("failed to cleanup read notifications: %w", err)
	}
	result.ReadExpired, _ = res.RowsAffected()

	// Pass 2: delete all notifications older than maxRetentionDays
	res, err = db.ExecContext(ctx, `
		DELETE FROM notifications
		WHERE created_at < NOW() - ($1 || ' days')::interval
	`, maxRetentionDays)
	if err != nil {
		return nil, fmt.Errorf("failed to cleanup expired notifications: %w", err)
	}
	result.MaxExpired, _ = res.RowsAffected()

	return result, nil
}

// DeleteReadNotifications deletes all read notifications for a specific user.
// Returns the number of notifications deleted.
func DeleteReadNotifications(ctx context.Context, db Execer, userID string) (int, error) {
	result, err := db.ExecContext(ctx, `
		DELETE FROM notifications
		WHERE user_id = $1 AND read_at IS NOT NULL
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete read notifications: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// Helper functions for creating specific notification types

// CreateContestJoinedNotification creates a notification when a user joins a contest.
func CreateContestJoinedNotification(ctx context.Context, db Execer, userID, contestID, contestName string) error {
	title := fmt.Sprintf("You joined \"%s\"", contestName)
	message := "Good luck! The contest will start soon. Make sure to be ready for trading."
	metadata := map[string]interface{}{
		"contest_id":   contestID,
		"contest_name": contestName,
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestJoined, title, message, metadata)
}

// CreateContestLeftNotification creates a notification when a user leaves a contest.
func CreateContestLeftNotification(ctx context.Context, db Execer, userID, contestID, contestName string, refunded bool) error {
	title := fmt.Sprintf("You left \"%s\"", contestName)
	message := "You have been removed from this contest."
	if refunded {
		message = "You have been removed from this contest. Your entry fee has been refunded to your wallet."
	}
	metadata := map[string]interface{}{
		"contest_id":   contestID,
		"contest_name": contestName,
		"refunded":     refunded,
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestLeft, title, message, metadata)
}

// CreatePrizeWonNotification creates a notification when a user wins a prize.
func CreatePrizeWonNotification(ctx context.Context, db Execer, userID, contestID, contestName string, rank int, prizeAmountCents int64, currency string) error {
	if currency == "" {
		currency = "USD"
	}
	amount := FormatAmount(prizeAmountCents, currency)
	title := fmt.Sprintf("You won %s in \"%s\"!", amount, contestName)
	message := fmt.Sprintf("Congratulations! You finished in position #%d and won a prize of %s. The prize has been credited to your wallet.", rank, amount)
	metadata := map[string]interface{}{
		"contest_id":         contestID,
		"contest_name":       contestName,
		"rank":               rank,
		"prize_amount_cents": prizeAmountCents,
		"currency":           currency,
	}
	return CreateNotification(ctx, db, userID, NotifTypePrizeWon, title, message, metadata)
}

// CreateContestCompletedNotification creates a notification when a contest ends.
func CreateContestCompletedNotification(ctx context.Context, db Execer, userID, contestID, contestName string, finalRank int, totalParticipants int) error {
	title := fmt.Sprintf("Contest \"%s\" has ended", contestName)
	message := fmt.Sprintf("You finished in position #%d out of %d participants. Check the results for more details.", finalRank, totalParticipants)
	metadata := map[string]interface{}{
		"contest_id":         contestID,
		"contest_name":       contestName,
		"final_rank":         finalRank,
		"total_participants": totalParticipants,
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestCompleted, title, message, metadata)
}

// CreateContestCancelledNotification creates a notification when a contest is cancelled.
func CreateContestCancelledNotification(ctx context.Context, db Execer, userID, contestID, contestName, reason string, refundAmountCents int64) error {
	title := fmt.Sprintf("Contest \"%s\" has been cancelled", contestName)
	var message string
	if refundAmountCents > 0 {
		refundAmount := float64(refundAmountCents) / 100.0
		message = fmt.Sprintf("Reason: %s. A refund of $%.2f has been credited to your wallet.", reason, refundAmount)
	} else {
		message = fmt.Sprintf("Reason: %s.", reason)
	}
	metadata := map[string]interface{}{
		"contest_id":          contestID,
		"contest_name":        contestName,
		"reason":              reason,
		"refund_amount_cents": refundAmountCents,
		"currency":            "IRR", // entry fees are in IRR
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestCancelled, title, message, metadata)
}

// CreateContestStartingNotification creates a notification when a contest is about to start.
func CreateContestStartingNotification(ctx context.Context, db Execer, userID, contestID, contestName string, startsAt time.Time) error {
	timeUntilStart := time.Until(startsAt)
	var timeStr string
	if timeUntilStart.Minutes() < 60 {
		timeStr = fmt.Sprintf("%.0f minutes", timeUntilStart.Minutes())
	} else {
		timeStr = fmt.Sprintf("%.0f hours", timeUntilStart.Hours())
	}

	title := fmt.Sprintf("Contest \"%s\" starts soon!", contestName)
	message := fmt.Sprintf("Your contest starts in approximately %s. Get ready to trade!", timeStr)
	metadata := map[string]interface{}{
		"contest_id":   contestID,
		"contest_name": contestName,
		"starts_at":    startsAt.Format(time.RFC3339),
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestStarting, title, message, metadata)
}

// CreateContestEndingNotification creates a notification when a contest is about to end.
func CreateContestEndingNotification(ctx context.Context, db Execer, userID, contestID, contestName string, endsAt time.Time) error {
	timeUntilEnd := time.Until(endsAt)
	var timeStr string
	if timeUntilEnd.Minutes() < 60 {
		timeStr = fmt.Sprintf("%.0f minutes", timeUntilEnd.Minutes())
	} else {
		timeStr = fmt.Sprintf("%.0f hours", timeUntilEnd.Hours())
	}

	title := fmt.Sprintf("Contest \"%s\" ends soon!", contestName)
	message := fmt.Sprintf("Your contest ends in approximately %s. Close your positions before the contest ends!", timeStr)
	metadata := map[string]interface{}{
		"contest_id":   contestID,
		"contest_name": contestName,
		"ends_at":      endsAt.Format(time.RFC3339),
	}
	return CreateNotification(ctx, db, userID, NotifTypeContestEnding, title, message, metadata)
}

// CreateKYCUpdateNotification creates a notification for KYC status changes.
func CreateKYCUpdateNotification(ctx context.Context, db Execer, userID, status, message string) error {
	var title string
	switch status {
	case "approved":
		title = "KYC Verification Approved"
	case "rejected":
		title = "KYC Verification Rejected"
	case "pending":
		title = "KYC Verification Submitted"
	default:
		title = "KYC Verification Update"
	}
	metadata := map[string]interface{}{
		"status": status,
	}
	return CreateNotification(ctx, db, userID, NotifTypeKYCUpdate, title, message, metadata)
}

// CreateDepositConfirmedNotification creates a notification when a deposit is confirmed.
func CreateDepositConfirmedNotification(ctx context.Context, db Execer, userID string, amountCents int64, currency, provider, transactionID string, newBalanceCents int64) error {
	amount := FormatAmount(amountCents, currency)
	title := fmt.Sprintf("Deposit of %s confirmed", amount)
	message := fmt.Sprintf("Your deposit of %s via %s has been credited to your wallet.", amount, FriendlyProviderName(provider))
	metadata := map[string]interface{}{
		"amount_cents":      amountCents,
		"currency":          currency,
		"provider":          provider,
		"transaction_id":    transactionID,
		"new_balance_cents": newBalanceCents,
	}
	return CreateNotification(ctx, db, userID, NotifTypeDepositConfirmed, title, message, metadata)
}

// CreateDepositFailedNotification creates a notification when a deposit fails.
func CreateDepositFailedNotification(ctx context.Context, db Execer, userID string, amountCents int64, currency, provider, transactionID string) error {
	amount := FormatAmount(amountCents, currency)
	title := "Deposit Failed"
	message := fmt.Sprintf("Your deposit of %s via %s could not be processed. Please try again.", amount, FriendlyProviderName(provider))
	metadata := map[string]interface{}{
		"amount_cents":   amountCents,
		"currency":       currency,
		"provider":       provider,
		"transaction_id": transactionID,
	}
	return CreateNotification(ctx, db, userID, NotifTypeDepositFailed, title, message, metadata)
}

// CreateWithdrawalUpdateNotification creates a notification for withdrawal status changes.
func CreateWithdrawalUpdateNotification(ctx context.Context, db Execer, userID, status string, amountCents int64, currency, withdrawalID string) error {
	amount := FormatAmount(amountCents, currency)
	var title, message string
	switch status {
	case "approved":
		title = fmt.Sprintf("Withdrawal of %s approved", amount)
		message = "Your withdrawal request has been approved and is being processed."
	case "rejected":
		title = fmt.Sprintf("Withdrawal of %s rejected", amount)
		message = "Your withdrawal request has been rejected. Please check your email for details."
	case "completed":
		title = fmt.Sprintf("Withdrawal of %s completed", amount)
		message = "Your withdrawal has been completed. The funds have been sent to your account."
	case "processing":
		title = fmt.Sprintf("Withdrawal of %s is processing", amount)
		message = "Your withdrawal is being processed. You will be notified when it's complete."
	default:
		title = "Withdrawal Update"
		message = "Your withdrawal status has been updated."
	}
	metadata := map[string]interface{}{
		"status":         status,
		"amount_cents":   amountCents,
		"currency":       currency,
		"withdrawal_id":  withdrawalID,
		"transaction_id": withdrawalID, // alias for frontend navigation
	}
	return CreateNotification(ctx, db, userID, NotifTypeWithdrawalUpdate, title, message, metadata)
}

// CreateTicketReplyNotification creates a notification when an admin replies to a support ticket.
func CreateTicketReplyNotification(ctx context.Context, db Execer, userID, ticketID, ticketSubject string) error {
	title := "پاسخ جدید به تیکت پشتیبانی"
	message := ticketSubject
	metadata := map[string]interface{}{
		"ticket_id": ticketID,
		"subject":   ticketSubject,
	}
	return CreateNotification(ctx, db, userID, NotifTypeTicketReply, title, message, metadata)
}

// CreateSystemNotification creates a general system notification.
func CreateSystemNotification(ctx context.Context, db Execer, userID, title, message string, metadata map[string]interface{}) error {
	return CreateNotification(ctx, db, userID, NotifTypeSystem, title, message, metadata)
}

// FormatAmount formats an amount in cents with the appropriate currency symbol.
// For IRR, amounts are displayed in Toman (Rials / 10) with comma separators.
// For USD (or other currencies), amounts are displayed as $X.XX.
func FormatAmount(amountCents int64, currency string) string {
	if currency == "IRR" {
		toman := amountCents / 10
		return formatWithThousandSep(toman) + " تومان"
	}
	return fmt.Sprintf("$%.2f", float64(amountCents)/100)
}

// FriendlyProviderName returns a user-friendly display name for a payment provider.
func FriendlyProviderName(provider string) string {
	switch provider {
	case "jibit":
		return "Jibit"
	case "nowpayments":
		return "NOWPayments"
	default:
		return provider
	}
}

// formatWithThousandSep formats an integer with comma thousand separators.
func formatWithThousandSep(n int64) string {
	if n < 0 {
		return "-" + formatWithThousandSep(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
