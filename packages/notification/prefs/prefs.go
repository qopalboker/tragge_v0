package prefs

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// Categories maps notification types to preference categories.
var Categories = map[string]string{
	"contest_starting":    "contest_reminders",
	"contest_ending":      "contest_reminders",
	"registration_closed": "contest_reminders",
	"contest_completed":   "contest_results",
	"contest_cancelled":   "contest_results",
	"prize_won":           "contest_results",
	"contest_paused":      "contest_results",
	"contest_resumed":     "contest_results",
	"contest_joined":      "contest_activity",
	"contest_left":        "contest_activity",
	"contest_started":     "contest_activity",
	"deposit_confirmed":   "transactions",
	"deposit_failed":      "transactions",
	"withdrawal_update":   "transactions",
	"kyc_update":          "account",
	"system":              "account",
	"ticket_reply":        "support",
	"password_changed":    "account",
}

// NonDismissableTypes are notification types that cannot be disabled by user preferences.
// These are security-critical notifications.
var NonDismissableTypes = map[string]bool{
	"password_changed": true,
}

// AllCategories lists valid categories in display order.
var AllCategories = []string{
	"contest_reminders",
	"contest_results",
	"contest_activity",
	"transactions",
	"account",
	"support",
}

// AllChannels lists valid channels.
var AllChannels = []string{"in_app", "email"}

// Querier is satisfied by *sql.DB, *sql.Tx, and pool.Replica().
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Execer is satisfied by *sql.DB, *sql.Tx, and pool.Primary().
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Preference represents a single preference row.
type Preference struct {
	Category string `json:"category"`
	Channel  string `json:"channel"`
	Enabled  bool   `json:"enabled"`
}

// UserPreferences is the full set of preferences for a user.
type UserPreferences struct {
	Preferences []Preference `json:"preferences"`
}

// GetUserPreferences returns all explicitly set preferences for a user.
// Missing category+channel combos mean "enabled" (default).
func GetUserPreferences(ctx context.Context, db Querier, userID string) (*UserPreferences, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT category, channel, enabled FROM user_notification_preferences WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification preferences: %w", err)
	}
	defer rows.Close()

	var prefs []Preference
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Category, &p.Channel, &p.Enabled); err != nil {
			return nil, fmt.Errorf("failed to scan preference: %w", err)
		}
		prefs = append(prefs, p)
	}
	return &UserPreferences{Preferences: prefs}, rows.Err()
}

// GetFullPreferences returns the complete preferences matrix (all categories x channels),
// filling in defaults (true) for any missing combos.
func GetFullPreferences(ctx context.Context, db Querier, userID string) ([]Preference, error) {
	up, err := GetUserPreferences(ctx, db, userID)
	if err != nil {
		return nil, err
	}

	// Build lookup map
	lookup := make(map[string]bool)
	for _, p := range up.Preferences {
		lookup[p.Category+":"+p.Channel] = p.Enabled
	}

	// Build full matrix
	var result []Preference
	for _, cat := range AllCategories {
		for _, ch := range AllChannels {
			key := cat + ":" + ch
			enabled := true
			if v, ok := lookup[key]; ok {
				enabled = v
			}
			result = append(result, Preference{
				Category: cat,
				Channel:  ch,
				Enabled:  enabled,
			})
		}
	}
	return result, nil
}

// isValidCategory checks if the given category is in AllCategories.
func isValidCategory(category string) bool {
	for _, c := range AllCategories {
		if c == category {
			return true
		}
	}
	return false
}

// isValidChannel checks if the given channel is in AllChannels.
func isValidChannel(channel string) bool {
	for _, c := range AllChannels {
		if c == channel {
			return true
		}
	}
	return false
}

// SetPreference sets a single preference (upsert).
func SetPreference(ctx context.Context, db Execer, userID, category, channel string, enabled bool) error {
	if !isValidCategory(category) {
		return fmt.Errorf("invalid notification category: %q", category)
	}
	if !isValidChannel(channel) {
		return fmt.Errorf("invalid notification channel: %q", channel)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_notification_preferences (user_id, category, channel, enabled, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, category, channel)
		DO UPDATE SET enabled = $4, updated_at = NOW()
	`, userID, category, channel, enabled)
	if err != nil {
		return fmt.Errorf("failed to set notification preference: %w", err)
	}
	return nil
}

// SetBulkPreferences sets multiple preferences in one transaction.
func SetBulkPreferences(ctx context.Context, db Execer, userID string, prefs []Preference) error {
	for _, p := range prefs {
		if err := SetPreference(ctx, db, userID, p.Category, p.Channel, p.Enabled); err != nil {
			return err
		}
	}
	return nil
}

// IsEnabled checks if a specific notification type + channel is enabled for a user.
// Returns true if no preference is set (default = enabled).
// Security-critical notifications (NonDismissableTypes) always return true.
func IsEnabled(ctx context.Context, db Querier, userID, notifType, channel string) (bool, error) {
	// Security-critical notifications cannot be disabled
	if NonDismissableTypes[notifType] {
		return true, nil
	}

	category, ok := Categories[notifType]
	if !ok {
		// Unknown type — allow by default
		return true, nil
	}

	rows, err := db.QueryContext(ctx,
		`SELECT enabled FROM user_notification_preferences WHERE user_id = $1 AND category = $2 AND channel = $3`,
		userID, category, channel,
	)
	if err != nil {
		return false, fmt.Errorf("failed to query notification preference: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var enabled bool
		if err := rows.Scan(&enabled); err != nil {
			return false, fmt.Errorf("failed to scan notification preference: %w", err)
		}
		return enabled, nil
	}
	return true, nil // No row = enabled
}

// IsEnabledBatch checks preferences for multiple users at once.
// Returns a map[userID]bool. Missing users default to true.
func IsEnabledBatch(ctx context.Context, db Querier, userIDs []string, notifType, channel string) (map[string]bool, error) {
	category, ok := Categories[notifType]
	if !ok {
		result := make(map[string]bool, len(userIDs))
		for _, uid := range userIDs {
			result[uid] = true
		}
		return result, nil
	}

	// Start with all enabled
	result := make(map[string]bool, len(userIDs))
	for _, uid := range userIDs {
		result[uid] = true
	}

	// Query disabled users
	rows, err := db.QueryContext(ctx, `
		SELECT user_id FROM user_notification_preferences
		WHERE category = $1 AND channel = $2 AND enabled = FALSE
		AND user_id = ANY($3)
	`, category, channel, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query batch notification preferences: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		result[uid] = false
	}

	return result, nil
}
