package prefs

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFullPreferences_EmptyDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Return empty result set
	mock.ExpectQuery("SELECT category, channel, enabled FROM user_notification_preferences").
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"category", "channel", "enabled"}))

	prefs, err := GetFullPreferences(context.Background(), db, "user-123")
	require.NoError(t, err)

	// 6 categories x 2 channels = 12
	assert.Len(t, prefs, 12)
	for _, p := range prefs {
		assert.True(t, p.Enabled, "category=%s channel=%s should default to enabled", p.Category, p.Channel)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFullPreferences_WithDisabledPref(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT category, channel, enabled FROM user_notification_preferences").
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"category", "channel", "enabled"}).
			AddRow("transactions", "email", false))

	prefs, err := GetFullPreferences(context.Background(), db, "user-123")
	require.NoError(t, err)
	assert.Len(t, prefs, 12)

	for _, p := range prefs {
		if p.Category == "transactions" && p.Channel == "email" {
			assert.False(t, p.Enabled, "transactions/email should be disabled")
		} else {
			assert.True(t, p.Enabled, "category=%s channel=%s should default to enabled", p.Category, p.Channel)
		}
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIsEnabled_NoRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT enabled FROM user_notification_preferences").
		WithArgs("user-123", "contest_reminders", "in_app").
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}))

	enabled, err := IsEnabled(context.Background(), db, "user-123", "contest_starting", "in_app")
	require.NoError(t, err)
	assert.True(t, enabled, "missing row should default to enabled")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIsEnabled_UnknownType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// No queries expected for unknown type
	enabled, err := IsEnabled(context.Background(), db, "user-123", "unknown_type", "in_app")
	require.NoError(t, err)
	assert.True(t, enabled, "unknown type should default to enabled")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIsEnabled_DisabledRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT enabled FROM user_notification_preferences").
		WithArgs("user-123", "contest_reminders", "in_app").
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))

	enabled, err := IsEnabled(context.Background(), db, "user-123", "contest_starting", "in_app")
	require.NoError(t, err)
	assert.False(t, enabled, "should respect disabled preference")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIsEnabled_EnabledRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT enabled FROM user_notification_preferences").
		WithArgs("user-123", "transactions", "email").
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))

	enabled, err := IsEnabled(context.Background(), db, "user-123", "deposit_confirmed", "email")
	require.NoError(t, err)
	assert.True(t, enabled, "should respect enabled preference")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSetPreference(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("INSERT INTO user_notification_preferences").
		WithArgs("user-123", "transactions", "email", false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = SetPreference(context.Background(), db, "user-123", "transactions", "email", false)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCategories_AllTypesHaveCategory(t *testing.T) {
	knownTypes := []string{
		"contest_starting", "contest_ending", "registration_closed",
		"contest_completed", "contest_cancelled", "prize_won", "contest_paused", "contest_resumed",
		"contest_joined", "contest_left", "contest_started",
		"deposit_confirmed", "deposit_failed", "withdrawal_update",
		"kyc_update", "system",
	}
	for _, nt := range knownTypes {
		cat, ok := Categories[nt]
		assert.True(t, ok, "type %q should have a category mapping", nt)
		assert.NotEmpty(t, cat)
	}
}

func TestAllCategories_Valid(t *testing.T) {
	assert.Len(t, AllCategories, 6)
	assert.Len(t, AllChannels, 2)
}
