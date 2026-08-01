package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"go.uber.org/zap"
)

// MarketTimeSpec represents a specific day and time for market open/close.
type MarketTimeSpec struct {
	Day      string `json:"day"`      // Day of week (e.g., "Sunday", "Monday", "Friday")
	Time     string `json:"time"`     // Time in HH:MM format
	Timezone string `json:"timezone"` // IANA timezone (e.g., "UTC", "America/New_York")
}

// MarketSchedule represents the trading schedule for an asset class.
type MarketSchedule struct {
	// Open can be either a MarketTimeSpec object or "always" string
	Open interface{} `json:"open"`
	// Close can be either a MarketTimeSpec object or "never" string
	Close    interface{} `json:"close"`
	Holidays []string    `json:"holidays"` // Format: "YYYY-MM-DD"
}

// MarketHoursConfig holds the complete market hours configuration.
type MarketHoursConfig map[string]MarketSchedule

// MarketOverride represents a manual override for special events.
type MarketOverride struct {
	AssetClass string    `json:"asset_class"`
	Status     string    `json:"status"` // "open" or "closed"
	Reason     string    `json:"reason"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// MarketStatus represents the current status of a market.
type MarketStatus struct {
	AssetClass string     `json:"asset_class"`
	IsOpen     bool       `json:"is_open"`
	Reason     string     `json:"reason,omitempty"`   // Why it's closed (holiday, weekend, override, etc.)
	NextOpen   *time.Time `json:"next_open,omitempty"`
	NextClose  *time.Time `json:"next_close,omitempty"`
	Override   *string    `json:"override,omitempty"` // If there's an active override
}

// MarketHoursChecker validates trading operations against market hours.
type MarketHoursChecker struct {
	config    MarketHoursConfig
	overrides map[string]*MarketOverride // keyed by asset class
	mu        sync.RWMutex
	logger    *zap.Logger

	// Callback for status change notifications
	onStatusChange func(status MarketStatus)

	// Timezone cache to avoid repeated time.LoadLocation calls
	tzCache sync.Map // string -> *time.Location
}

// NewMarketHoursChecker creates a new market hours checker from a config file.
func NewMarketHoursChecker(configPath string, logger *zap.Logger) (*MarketHoursChecker, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read market hours config: %w", err)
	}

	var config MarketHoursConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse market hours config: %w", err)
	}

	checker := &MarketHoursChecker{
		config:    config,
		overrides: make(map[string]*MarketOverride),
		logger:    logger,
	}

	logger.Info("Market hours checker initialized",
		zap.Int("asset_classes", len(config)))

	return checker, nil
}

// SetStatusChangeCallback sets a callback that is called when market status changes.
func (m *MarketHoursChecker) SetStatusChangeCallback(cb func(status MarketStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStatusChange = cb
}

// IsMarketOpen checks if the market is currently open for the given asset class.
func (m *MarketHoursChecker) IsMarketOpen(assetClass string) bool {
	status := m.GetMarketStatus(assetClass)
	return status.IsOpen
}

// GetMarketStatus returns the detailed market status for an asset class.
func (m *MarketHoursChecker) GetMarketStatus(assetClass string) MarketStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()

	// Check for manual override first
	if override, exists := m.overrides[assetClass]; exists {
		if now.Before(override.ExpiresAt) {
			isOpen := override.Status == "open"
			status := MarketStatus{
				AssetClass: assetClass,
				IsOpen:     isOpen,
				Reason:     fmt.Sprintf("Manual override: %s", override.Reason),
				Override:   &override.Reason,
			}
			if !isOpen {
				nextOpen := m.calculateNextOpen(assetClass, now)
				status.NextOpen = nextOpen
			}
			return status
		}
		// Override expired — skip it; cleanup happens in write-locked methods
		// (SetOverride, ClearOverride) and StartMonitor's periodic sweep
	}

	schedule, exists := m.config[assetClass]
	if !exists {
		// Unknown asset class - default to open (safe for mixed/unknown)
		m.logger.Warn("Unknown asset class, defaulting to open",
			zap.String("asset_class", assetClass))
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
			Reason:     "Unknown asset class - default open",
		}
	}

	// Check if market is always open (crypto)
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
		}
	}

	// Check holidays (in market timezone)
	loc := m.loadScheduleTimezone(schedule)
	if m.isHoliday(schedule.Holidays, now, loc) {
		nextOpen := m.calculateNextOpen(assetClass, now)
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     false,
			Reason:     "Holiday",
			NextOpen:   nextOpen,
		}
	}

	// Check regular market hours
	isOpen, reason := m.checkMarketHours(schedule, now)
	status := MarketStatus{
		AssetClass: assetClass,
		IsOpen:     isOpen,
		Reason:     reason,
	}

	if isOpen {
		nextClose := m.calculateNextClose(assetClass, now)
		status.NextClose = nextClose
	} else {
		nextOpen := m.calculateNextOpen(assetClass, now)
		status.NextOpen = nextOpen
	}

	return status
}

// checkMarketHours checks if the current time is within regular market hours.
func (m *MarketHoursChecker) checkMarketHours(schedule MarketSchedule, now time.Time) (bool, string) {
	openSpec, err := m.parseTimeSpec(schedule.Open)
	if err != nil {
		m.logger.Error("Failed to parse open time spec", zap.Error(err))
		return true, "" // Default to open on error
	}

	closeSpec, err := m.parseTimeSpec(schedule.Close)
	if err != nil {
		m.logger.Error("Failed to parse close time spec", zap.Error(err))
		return true, "" // Default to open on error
	}

	// Load timezone (cached)
	loc, err := m.cachedLoadLocation(openSpec.Timezone)
	if err != nil {
		m.logger.Error("Failed to load timezone", zap.String("tz", openSpec.Timezone), zap.Error(err))
		loc = time.UTC
	}

	// Convert current time to the market's timezone
	nowLocal := now.In(loc)
	currentDay := nowLocal.Weekday()
	currentTime := nowLocal.Format("15:04")

	openDay := m.parseDayOfWeek(openSpec.Day)
	closeDay := m.parseDayOfWeek(closeSpec.Day)

	// For forex-style markets (Sun 22:00 to Fri 22:00):
	// Market is open if we're between open day/time and close day/time

	// Determine if we're in the trading window
	isInTradingWindow := m.isInTradingWindow(
		currentDay, currentTime,
		openDay, openSpec.Time,
		closeDay, closeSpec.Time,
	)

	if !isInTradingWindow {
		return false, "Outside market hours"
	}

	return true, ""
}

// isInTradingWindow checks if the current time is within the trading window.
func (m *MarketHoursChecker) isInTradingWindow(
	currentDay time.Weekday, currentTime string,
	openDay time.Weekday, openTime string,
	closeDay time.Weekday, closeTime string,
) bool {
	// Convert days to integers for comparison (Sunday = 0, Saturday = 6)
	current := int(currentDay)
	open := int(openDay)
	close := int(closeDay)

	// Handle wrap-around weeks (e.g., Sunday to Friday)
	if open <= close {
		// Same week: open on Sunday, close on Friday
		// Trading days are: Sunday (after open time) to Friday (before close time)
		if current < open || current > close {
			return false
		}
		if current == open {
			return currentTime >= openTime
		}
		if current == close {
			return currentTime < closeTime
		}
		return true
	}

	// Wrap-around case (e.g., Friday to Monday)
	// Market is open if current is between open day/time and close day/time,
	// wrapping around the end of the week.
	if current == open {
		return currentTime >= openTime
	}
	if current > open {
		return true
	}
	if current < close {
		return true
	}
	if current == close {
		return currentTime < closeTime
	}

	return false
}

// parseTimeSpec parses a time specification from the config.
func (m *MarketHoursChecker) parseTimeSpec(spec interface{}) (*MarketTimeSpec, error) {
	switch v := spec.(type) {
	case string:
		// "always" or "never"
		return &MarketTimeSpec{
			Day:      "",
			Time:     "",
			Timezone: "UTC",
		}, nil
	case map[string]interface{}:
		result := &MarketTimeSpec{
			Timezone: "UTC", // default
		}
		if day, ok := v["day"].(string); ok {
			result.Day = day
		}
		if tm, ok := v["time"].(string); ok {
			result.Time = tm
		}
		if tz, ok := v["timezone"].(string); ok {
			result.Timezone = tz
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid time spec type: %T", spec)
	}
}

// parseDayOfWeek converts a day name to time.Weekday.
func (m *MarketHoursChecker) parseDayOfWeek(day string) time.Weekday {
	days := map[string]time.Weekday{
		"Sunday":    time.Sunday,
		"Monday":    time.Monday,
		"Tuesday":   time.Tuesday,
		"Wednesday": time.Wednesday,
		"Thursday":  time.Thursday,
		"Friday":    time.Friday,
		"Saturday":  time.Saturday,
	}
	if d, ok := days[day]; ok {
		return d
	}
	return time.Sunday // default
}

// cachedLoadLocation returns a *time.Location, caching results to avoid
// repeated syscalls from time.LoadLocation on every tick.
func (m *MarketHoursChecker) cachedLoadLocation(name string) (*time.Location, error) {
	if loc, ok := m.tzCache.Load(name); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	m.tzCache.Store(name, loc)
	return loc, nil
}

// loadScheduleTimezone extracts and loads the timezone from a MarketSchedule's open spec.
// Returns time.UTC if the timezone cannot be determined or loaded.
func (m *MarketHoursChecker) loadScheduleTimezone(schedule MarketSchedule) *time.Location {
	openSpec, err := m.parseTimeSpec(schedule.Open)
	if err != nil {
		return time.UTC
	}
	loc, err := m.cachedLoadLocation(openSpec.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// isHoliday checks if the given time falls on a holiday in the specified timezone.
func (m *MarketHoursChecker) isHoliday(holidays []string, t time.Time, loc *time.Location) bool {
	dateStr := t.In(loc).Format("2006-01-02")
	for _, holiday := range holidays {
		if holiday == dateStr {
			return true
		}
	}
	return false
}

// calculateNextOpen calculates when the market will next open.
func (m *MarketHoursChecker) calculateNextOpen(assetClass string, from time.Time) *time.Time {
	schedule, exists := m.config[assetClass]
	if !exists {
		return nil
	}

	// If always open, return nil
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return nil
	}

	openSpec, err := m.parseTimeSpec(schedule.Open)
	if err != nil {
		return nil
	}

	loc, err := m.cachedLoadLocation(openSpec.Timezone)
	if err != nil {
		loc = time.UTC
	}

	fromLocal := from.In(loc)
	openDay := m.parseDayOfWeek(openSpec.Day)
	openTimeParts := m.parseTime(openSpec.Time)

	// Find the next occurrence of the open day/time
	nextOpen := fromLocal
	for i := 0; i < 8; i++ { // Max 8 days to find next open
		if nextOpen.Weekday() == openDay {
			// Check if we're past the open time today
			openToday := time.Date(nextOpen.Year(), nextOpen.Month(), nextOpen.Day(),
				openTimeParts[0], openTimeParts[1], 0, 0, loc)
			if nextOpen.Before(openToday) || nextOpen.Equal(openToday) {
				// Skip holidays
				if !m.isHoliday(schedule.Holidays, openToday, loc) {
					result := openToday.UTC()
					return &result
				}
			}
		}
		nextOpen = nextOpen.AddDate(0, 0, 1)
		// Reset to beginning of day
		nextOpen = time.Date(nextOpen.Year(), nextOpen.Month(), nextOpen.Day(), 0, 0, 0, 0, loc)
	}

	return nil
}

// calculateNextClose calculates when the market will next close.
func (m *MarketHoursChecker) calculateNextClose(assetClass string, from time.Time) *time.Time {
	schedule, exists := m.config[assetClass]
	if !exists {
		return nil
	}

	// If never closes, return nil
	if closeStr, ok := schedule.Close.(string); ok && closeStr == "never" {
		return nil
	}

	closeSpec, err := m.parseTimeSpec(schedule.Close)
	if err != nil {
		return nil
	}

	loc, err := m.cachedLoadLocation(closeSpec.Timezone)
	if err != nil {
		loc = time.UTC
	}

	fromLocal := from.In(loc)
	closeDay := m.parseDayOfWeek(closeSpec.Day)
	closeTimeParts := m.parseTime(closeSpec.Time)

	// Find the next occurrence of the close day/time
	nextClose := fromLocal
	for i := 0; i < 8; i++ {
		if nextClose.Weekday() == closeDay {
			closeToday := time.Date(nextClose.Year(), nextClose.Month(), nextClose.Day(),
				closeTimeParts[0], closeTimeParts[1], 0, 0, loc)
			if nextClose.Before(closeToday) {
				result := closeToday.UTC()
				return &result
			}
		}
		nextClose = nextClose.AddDate(0, 0, 1)
		nextClose = time.Date(nextClose.Year(), nextClose.Month(), nextClose.Day(), 0, 0, 0, 0, loc)
	}

	return nil
}

// parseTime parses a HH:MM time string into hours and minutes.
func (m *MarketHoursChecker) parseTime(timeStr string) [2]int {
	var h, min int
	fmt.Sscanf(timeStr, "%d:%d", &h, &min)
	return [2]int{h, min}
}

// SetOverride sets a manual override for an asset class.
func (m *MarketHoursChecker) SetOverride(assetClass, status, reason, createdBy string, expiresAt time.Time) error {
	if status != "open" && status != "closed" {
		return fmt.Errorf("invalid override status: %s (must be 'open' or 'closed')", status)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	override := &MarketOverride{
		AssetClass: assetClass,
		Status:     status,
		Reason:     reason,
		ExpiresAt:  expiresAt,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now().UTC(),
	}

	m.overrides[assetClass] = override

	m.logger.Info("Market override set",
		zap.String("asset_class", assetClass),
		zap.String("status", status),
		zap.String("reason", reason),
		zap.Time("expires_at", expiresAt),
		zap.String("created_by", createdBy))

	// Notify status change — evaluate GetMarketStatus inside the goroutine
	// so it runs after the write lock is released (avoids deadlock).
	if m.onStatusChange != nil {
		ac := assetClass
		cb := m.onStatusChange
		infra.SafeGo(m.logger, "market-status-change-callback", func() {
			cb(m.GetMarketStatus(ac))
		})
	}

	return nil
}

// ClearOverride removes a manual override for an asset class.
func (m *MarketHoursChecker) ClearOverride(assetClass string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.overrides, assetClass)

	m.logger.Info("Market override cleared",
		zap.String("asset_class", assetClass))

	// Notify status change — evaluate GetMarketStatus inside the goroutine
	// so it runs after the write lock is released (avoids deadlock).
	if m.onStatusChange != nil {
		ac := assetClass
		cb := m.onStatusChange
		infra.SafeGo(m.logger, "market-status-change-callback", func() {
			cb(m.GetMarketStatus(ac))
		})
	}
}

// GetOverride returns the current override for an asset class, if any.
func (m *MarketHoursChecker) GetOverride(assetClass string) *MarketOverride {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.overrides[assetClass]
}

// GetAllOverrides returns all active overrides.
func (m *MarketHoursChecker) GetAllOverrides() map[string]*MarketOverride {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*MarketOverride)
	now := time.Now().UTC()

	for k, v := range m.overrides {
		if now.Before(v.ExpiresAt) {
			result[k] = v
		}
	}
	return result
}

// GetAllStatuses returns the market status for all configured asset classes.
func (m *MarketHoursChecker) GetAllStatuses() []MarketStatus {
	m.mu.RLock()
	assetClasses := make([]string, 0, len(m.config))
	for ac := range m.config {
		assetClasses = append(assetClasses, ac)
	}
	m.mu.RUnlock()

	statuses := make([]MarketStatus, 0, len(assetClasses))
	for _, ac := range assetClasses {
		statuses = append(statuses, m.GetMarketStatus(ac))
	}
	return statuses
}

// ValidateContestTimes validates that contest times fall within market hours.
func (m *MarketHoursChecker) ValidateContestTimes(assetClass string, startsAt, endsAt time.Time) error {
	schedule, exists := m.config[assetClass]
	if !exists {
		// Unknown asset class - allow it
		return nil
	}

	// If always open, any time is valid
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return nil
	}

	// Check if start time is during market hours
	startStatus := m.getStatusAt(assetClass, startsAt)
	if !startStatus.IsOpen {
		return fmt.Errorf("contest start time (%s) falls outside market hours: %s",
			startsAt.Format(time.RFC3339), startStatus.Reason)
	}

	// Check if end time is during market hours
	endStatus := m.getStatusAt(assetClass, endsAt)
	if !endStatus.IsOpen {
		return fmt.Errorf("contest end time (%s) falls outside market hours: %s",
			endsAt.Format(time.RFC3339), endStatus.Reason)
	}

	// Check for holidays during the contest duration
	loc := m.loadScheduleTimezone(schedule)
	current := startsAt
	for current.Before(endsAt) {
		if m.isHoliday(schedule.Holidays, current, loc) {
			return fmt.Errorf("contest period includes holiday: %s", current.In(loc).Format("2006-01-02"))
		}
		current = current.AddDate(0, 0, 1)
	}

	return nil
}

// getStatusAt returns the market status at a specific point in time.
func (m *MarketHoursChecker) getStatusAt(assetClass string, t time.Time) MarketStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	schedule, exists := m.config[assetClass]
	if !exists {
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
			Reason:     "Unknown asset class - default open",
		}
	}

	// Check if market is always open
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
		}
	}

	// Check holidays (in market timezone)
	loc := m.loadScheduleTimezone(schedule)
	if m.isHoliday(schedule.Holidays, t, loc) {
		return MarketStatus{
			AssetClass: assetClass,
			IsOpen:     false,
			Reason:     "Holiday",
		}
	}

	// Check regular market hours
	isOpen, reason := m.checkMarketHours(schedule, t)
	return MarketStatus{
		AssetClass: assetClass,
		IsOpen:     isOpen,
		Reason:     reason,
	}
}

// AssetClassFromContracts converts a contracts.AssetClass to string.
func AssetClassFromContracts(ac contracts.AssetClass) string {
	return string(ac)
}

// GetNextMarketEvent returns the next market open or close event.
func (m *MarketHoursChecker) GetNextMarketEvent(assetClass string) (eventType string, eventTime *time.Time) {
	status := m.GetMarketStatus(assetClass)
	if status.IsOpen {
		return "close", status.NextClose
	}
	return "open", status.NextOpen
}

// CalculatePauseCompensation calculates how much time to add to a contest's end time
// to compensate for market-hours-related pauses.
func (m *MarketHoursChecker) CalculatePauseCompensation(assetClass string, pausedAt, resumedAt time.Time) time.Duration {
	// Simply return the duration of the pause
	// The caller can add this to the contest's ends_at
	return resumedAt.Sub(pausedAt)
}

// StartMonitor starts a goroutine that monitors market status changes and
// calls the callback when markets open or close.
func (m *MarketHoursChecker) StartMonitor(ctx context.Context, checkInterval time.Duration) {
	infra.SafeGo(m.logger, "market-hours-monitor", func() {
		lastStatuses := make(map[string]bool)

		// Initialize with current statuses
		for ac := range m.config {
			status := m.GetMarketStatus(ac)
			lastStatuses[ac] = status.IsOpen
		}

		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Sweep expired overrides under a write lock
				now := time.Now().UTC()
				m.mu.Lock()
				for ac, override := range m.overrides {
					if now.After(override.ExpiresAt) {
						delete(m.overrides, ac)
					}
				}
				m.mu.Unlock()

				for ac := range m.config {
					status := m.GetMarketStatus(ac)
					wasOpen, exists := lastStatuses[ac]

					if !exists || wasOpen != status.IsOpen {
						lastStatuses[ac] = status.IsOpen

						m.logger.Info("Market status changed",
							zap.String("asset_class", ac),
							zap.Bool("is_open", status.IsOpen),
							zap.String("reason", status.Reason))

						if m.onStatusChange != nil {
							m.onStatusChange(status)
						}
					}
				}
			}
		}
	})
}
