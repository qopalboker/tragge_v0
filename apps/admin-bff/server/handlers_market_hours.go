package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// MarketTimeSpec represents a specific day and time for market open/close.
type MarketTimeSpec struct {
	Day      string `json:"day"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

// MarketSchedule represents the trading schedule for an asset class.
type MarketSchedule struct {
	Open     interface{} `json:"open"`
	Close    interface{} `json:"close"`
	Holidays []string    `json:"holidays"`
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

// MarketHoursManager manages market hours configuration and overrides.
type MarketHoursManager struct {
	config     MarketHoursConfig
	overrides  map[string]*MarketOverride
	configPath string
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewMarketHoursManager creates a new market hours manager.
func NewMarketHoursManager(configPath string, logger *zap.Logger) (*MarketHoursManager, error) {
	m := &MarketHoursManager{
		overrides:  make(map[string]*MarketOverride),
		configPath: configPath,
		logger:     logger,
	}

	if err := m.loadConfig(); err != nil {
		return nil, err
	}

	return m, nil
}

// loadConfig loads the market hours configuration from file.
func (m *MarketHoursManager) loadConfig() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("read market hours config: %w", err)
	}

	var config MarketHoursConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse market hours config: %w", err)
	}

	m.mu.Lock()
	m.config = config
	m.mu.Unlock()

	return nil
}

// GetConfig returns the current market hours configuration.
func (m *MarketHoursManager) GetConfig() MarketHoursConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	result := make(MarketHoursConfig)
	for k, v := range m.config {
		result[k] = v
	}
	return result
}

// GetStatus returns the market status for an asset class.
func (m *MarketHoursManager) GetStatus(assetClass string) contracts.MarketStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()

	// Check for manual override first
	if override, exists := m.overrides[assetClass]; exists {
		if now.Before(override.ExpiresAt) {
			isOpen := override.Status == "open"
			return contracts.MarketStatus{
				AssetClass: assetClass,
				IsOpen:     isOpen,
				Reason:     fmt.Sprintf("Manual override: %s", override.Reason),
				Override:   &override.Reason,
			}
		}
		// Override expired, remove it
		delete(m.overrides, assetClass)
	}

	schedule, exists := m.config[assetClass]
	if !exists {
		return contracts.MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
			Reason:     "Unknown asset class - default open",
		}
	}

	// Check if market is always open (crypto)
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return contracts.MarketStatus{
			AssetClass: assetClass,
			IsOpen:     true,
		}
	}

	// Check holidays
	dateStr := now.Format("2006-01-02")
	for _, holiday := range schedule.Holidays {
		if holiday == dateStr {
			return contracts.MarketStatus{
				AssetClass: assetClass,
				IsOpen:     false,
				Reason:     "Holiday",
			}
		}
	}

	// Check regular market hours
	isOpen, reason := m.checkMarketHours(schedule, now)
	return contracts.MarketStatus{
		AssetClass: assetClass,
		IsOpen:     isOpen,
		Reason:     reason,
	}
}

// checkMarketHours checks if the current time is within regular market hours.
func (m *MarketHoursManager) checkMarketHours(schedule MarketSchedule, now time.Time) (bool, string) {
	openSpec, err := parseTimeSpec(schedule.Open)
	if err != nil {
		return true, "" // Default to open on error
	}

	closeSpec, err := parseTimeSpec(schedule.Close)
	if err != nil {
		return true, "" // Default to open on error
	}

	// Load timezone
	loc, err := time.LoadLocation(openSpec.Timezone)
	if err != nil {
		loc = time.UTC
	}

	// Convert current time to the market's timezone
	nowLocal := now.In(loc)
	currentDay := nowLocal.Weekday()
	currentTime := nowLocal.Format("15:04")

	openDay := parseDayOfWeek(openSpec.Day)
	closeDay := parseDayOfWeek(closeSpec.Day)

	// Check if we're in the trading window
	isInWindow := isInTradingWindow(
		currentDay, currentTime,
		openDay, openSpec.Time,
		closeDay, closeSpec.Time,
	)

	if !isInWindow {
		return false, "Outside market hours"
	}

	return true, ""
}

// GetAllStatuses returns the status for all configured asset classes.
func (m *MarketHoursManager) GetAllStatuses() []contracts.MarketStatus {
	m.mu.RLock()
	assetClasses := make([]string, 0, len(m.config))
	for ac := range m.config {
		assetClasses = append(assetClasses, ac)
	}
	m.mu.RUnlock()

	statuses := make([]contracts.MarketStatus, 0, len(assetClasses))
	for _, ac := range assetClasses {
		statuses = append(statuses, m.GetStatus(ac))
	}
	return statuses
}

// SetOverride sets a manual override for an asset class.
func (m *MarketHoursManager) SetOverride(assetClass, status, reason, createdBy string, expiresAt time.Time) error {
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

	return nil
}

// ClearOverride removes a manual override for an asset class.
func (m *MarketHoursManager) ClearOverride(assetClass string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.overrides, assetClass)

	m.logger.Info("Market override cleared",
		zap.String("asset_class", assetClass))
}

// GetOverride returns the current override for an asset class, if any.
func (m *MarketHoursManager) GetOverride(assetClass string) *MarketOverride {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.overrides[assetClass]
}

// GetAllOverrides returns all active overrides.
func (m *MarketHoursManager) GetAllOverrides() map[string]*MarketOverride {
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

// ValidateContestTimes validates that contest times fall within market hours.
func (m *MarketHoursManager) ValidateContestTimes(assetClass string, startsAt, endsAt time.Time) error {
	m.mu.RLock()
	schedule, exists := m.config[assetClass]
	m.mu.RUnlock()

	if !exists {
		return nil // Unknown asset class - allow it
	}

	// If always open, any time is valid
	if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
		return nil
	}

	// Check if start time is during market hours
	startOpen, startReason := m.checkMarketHours(schedule, startsAt)
	if !startOpen {
		return fmt.Errorf("contest start time (%s) falls outside market hours: %s",
			startsAt.Format(time.RFC3339), startReason)
	}

	// Check if end time is during market hours
	endOpen, endReason := m.checkMarketHours(schedule, endsAt)
	if !endOpen {
		return fmt.Errorf("contest end time (%s) falls outside market hours: %s",
			endsAt.Format(time.RFC3339), endReason)
	}

	return nil
}

// Helper functions
func parseTimeSpec(spec interface{}) (*MarketTimeSpec, error) {
	switch v := spec.(type) {
	case string:
		return &MarketTimeSpec{
			Day:      "",
			Time:     "",
			Timezone: "UTC",
		}, nil
	case map[string]interface{}:
		result := &MarketTimeSpec{
			Timezone: "UTC",
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

func parseDayOfWeek(day string) time.Weekday {
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
	return time.Sunday
}

func isInTradingWindow(
	currentDay time.Weekday, currentTime string,
	openDay time.Weekday, openTime string,
	closeDay time.Weekday, closeTime string,
) bool {
	current := int(currentDay)
	open := int(openDay)
	close := int(closeDay)

	if open <= close {
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

	if current > open {
		if current == open {
			return currentTime >= openTime
		}
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

// HTTP Handlers

// handleGetMarketStatus returns the current market status for all or specific asset classes.
func (a *App) handleGetMarketStatus(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		http.Error(w, "Market hours not configured", http.StatusServiceUnavailable)
		return
	}

	assetClass := r.URL.Query().Get("asset_class")

	var response contracts.MarketStatusResponse

	if assetClass != "" {
		status := a.marketHours.GetStatus(assetClass)
		response.Statuses = []contracts.MarketStatus{status}
	} else {
		response.Statuses = a.marketHours.GetAllStatuses()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetMarketHoursConfig returns the market hours configuration.
func (a *App) handleGetMarketHoursConfig(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		http.Error(w, "Market hours not configured", http.StatusServiceUnavailable)
		return
	}

	config := a.marketHours.GetConfig()

	// Transform to API format
	response := make([]contracts.MarketHoursConfig, 0)
	for ac, schedule := range config {
		cfg := contracts.MarketHoursConfig{
			AssetClass: ac,
			Holidays:   schedule.Holidays,
		}

		if openStr, ok := schedule.Open.(string); ok && openStr == "always" {
			cfg.AlwaysOpen = true
		} else if openMap, ok := schedule.Open.(map[string]interface{}); ok {
			cfg.OpenTime = &contracts.MarketTimeSpec{
				Day:      openMap["day"].(string),
				Time:     openMap["time"].(string),
				Timezone: openMap["timezone"].(string),
			}
		}

		if closeMap, ok := schedule.Close.(map[string]interface{}); ok {
			cfg.CloseTime = &contracts.MarketTimeSpec{
				Day:      closeMap["day"].(string),
				Time:     closeMap["time"].(string),
				Timezone: closeMap["timezone"].(string),
			}
		}

		response = append(response, cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSetMarketOverride sets a manual market override.
func (a *App) handleSetMarketOverride(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		http.Error(w, "Market hours not configured", http.StatusServiceUnavailable)
		return
	}

	var req contracts.SetOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AssetClass == "" {
		http.Error(w, "asset_class is required", http.StatusBadRequest)
		return
	}
	if req.Status != "open" && req.Status != "closed" {
		http.Error(w, "status must be 'open' or 'closed'", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		http.Error(w, "expires_at must be in RFC3339 format", http.StatusBadRequest)
		return
	}

	// Get user ID from context
	userID := auth.GetUserID(r.Context())
	if userID == "" {
		userID = "system"
	}

	if err := a.marketHours.SetOverride(req.AssetClass, req.Status, req.Reason, userID, expiresAt); err != nil {
		a.log().Error("Failed to set market hours override",
			zap.String("asset_class", req.AssetClass),
			zap.Error(err))
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Log audit event
	a.logAuditEvent(r.Context(), userID, "market_override_set", "market_hours", req.AssetClass, map[string]interface{}{
		"asset_class": req.AssetClass,
		"status":      req.Status,
		"reason":      req.Reason,
		"expires_at":  req.ExpiresAt,
	})

	// Return the new status
	status := a.marketHours.GetStatus(req.AssetClass)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleClearMarketOverride clears a market override.
func (a *App) handleClearMarketOverride(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		http.Error(w, "Market hours not configured", http.StatusServiceUnavailable)
		return
	}

	assetClass := chi.URLParam(r, "asset_class")
	if assetClass == "" {
		http.Error(w, "asset_class is required", http.StatusBadRequest)
		return
	}

	a.marketHours.ClearOverride(assetClass)

	// Log audit event
	userID := auth.GetUserID(r.Context())
	a.logAuditEvent(r.Context(), userID, "market_override_cleared", "market_hours", assetClass, map[string]interface{}{
		"asset_class": assetClass,
	})

	// Return the new status
	status := a.marketHours.GetStatus(assetClass)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleGetMarketOverrides returns all active overrides.
func (a *App) handleGetMarketOverrides(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		http.Error(w, "Market hours not configured", http.StatusServiceUnavailable)
		return
	}

	overrides := a.marketHours.GetAllOverrides()

	// Convert to API format
	response := make([]contracts.MarketOverride, 0, len(overrides))
	for _, o := range overrides {
		response = append(response, contracts.MarketOverride{
			AssetClass: o.AssetClass,
			Status:     o.Status,
			Reason:     o.Reason,
			ExpiresAt:  o.ExpiresAt.Format(time.RFC3339),
			CreatedBy:  o.CreatedBy,
			CreatedAt:  o.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleValidateContestTimes validates contest times against market hours.
func (a *App) handleValidateContestTimes(w http.ResponseWriter, r *http.Request) {
	if a.marketHours == nil {
		// If market hours not configured, all times are valid
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contracts.ValidateContestTimesResponse{Valid: true})
		return
	}

	var req contracts.ValidateContestTimesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		http.Error(w, "starts_at must be in RFC3339 format", http.StatusBadRequest)
		return
	}

	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		http.Error(w, "ends_at must be in RFC3339 format", http.StatusBadRequest)
		return
	}

	validationErr := a.marketHours.ValidateContestTimes(req.AssetClass, startsAt, endsAt)

	response := contracts.ValidateContestTimesResponse{Valid: validationErr == nil}
	if validationErr != nil {
		reason := validationErr.Error()
		response.Reason = reason
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
