package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
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

// MarketStatus represents the current status of a market.
type MarketStatus struct {
	AssetClass string
	IsOpen     bool
	Reason     string
	NextOpen   *time.Time
	NextClose  *time.Time
}

// MarketStatusMonitor monitors market hours and broadcasts status changes.
type MarketStatusMonitor struct {
	config       MarketHoursConfig
	lastStatuses map[string]bool // Last known status for each asset class
	mu           sync.RWMutex
	hub          *Hub
	logger       *zap.Logger
}

// NewMarketStatusMonitor creates a new market status monitor.
func NewMarketStatusMonitor(configPath string, hub *Hub, logger *zap.Logger) (*MarketStatusMonitor, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read market hours config: %w", err)
	}

	var config MarketHoursConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse market hours config: %w", err)
	}

	m := &MarketStatusMonitor{
		config:       config,
		lastStatuses: make(map[string]bool),
		hub:          hub,
		logger:       logger,
	}

	// Initialize with current statuses
	for ac := range config {
		status := m.GetStatus(ac)
		m.lastStatuses[ac] = status.IsOpen
	}

	logger.Info("Market status monitor initialized",
		zap.Int("asset_classes", len(config)))

	return m, nil
}

// Start begins monitoring market status changes.
func (m *MarketStatusMonitor) Start(ctx context.Context, checkInterval time.Duration) {
	infra.SafeGo(m.logger, "market-status-monitor", func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.checkAndBroadcastChanges()
			}
		}
	})
}

// checkAndBroadcastChanges checks for market status changes and broadcasts them.
func (m *MarketStatusMonitor) checkAndBroadcastChanges() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for ac := range m.config {
		status := m.GetStatus(ac)
		wasOpen, exists := m.lastStatuses[ac]

		if !exists || wasOpen != status.IsOpen {
			m.lastStatuses[ac] = status.IsOpen

			m.logger.Info("Market status changed",
				zap.String("asset_class", ac),
				zap.Bool("is_open", status.IsOpen),
				zap.String("reason", status.Reason))

			// Broadcast to all connected clients
			m.broadcastStatusChange(ac, status)
		}
	}
}

// broadcastStatusChange sends a market status event to all connected clients.
func (m *MarketStatusMonitor) broadcastStatusChange(assetClass string, status MarketStatus) {
	if m.hub == nil {
		return
	}

	event := contracts.MarketStatusEvent{
		Type:       "market_status",
		AssetClass: assetClass,
		Status:     "open",
		Reason:     status.Reason,
		Ts:         time.Now().UnixMilli(),
	}

	if !status.IsOpen {
		event.Status = "closed"
		if status.NextOpen != nil {
			event.ReopensAt = status.NextOpen.Format(time.RFC3339)
		}
	} else {
		if status.NextClose != nil {
			event.ClosesAt = status.NextClose.Format(time.RFC3339)
		}
	}

	// Broadcast to all clients
	m.hub.BroadcastMarketStatus(&event)
}

// GetStatus returns the current market status for an asset class.
func (m *MarketStatusMonitor) GetStatus(assetClass string) MarketStatus {
	now := time.Now().UTC()

	schedule, exists := m.config[assetClass]
	if !exists {
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

	// Check holidays
	dateStr := now.Format("2006-01-02")
	for _, holiday := range schedule.Holidays {
		if holiday == dateStr {
			return MarketStatus{
				AssetClass: assetClass,
				IsOpen:     false,
				Reason:     "Holiday",
			}
		}
	}

	// Check regular market hours
	isOpen, reason := m.checkMarketHours(schedule, now)
	return MarketStatus{
		AssetClass: assetClass,
		IsOpen:     isOpen,
		Reason:     reason,
	}
}

// checkMarketHours checks if the current time is within regular market hours.
func (m *MarketStatusMonitor) checkMarketHours(schedule MarketSchedule, now time.Time) (bool, string) {
	openSpec, err := parseMarketTimeSpec(schedule.Open)
	if err != nil {
		return true, "" // Default to open on error
	}

	closeSpec, err := parseMarketTimeSpec(schedule.Close)
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

	openDay := parseMarketDayOfWeek(openSpec.Day)
	closeDay := parseMarketDayOfWeek(closeSpec.Day)

	// Check if we're in the trading window
	isInWindow := isInMarketTradingWindow(
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
func (m *MarketStatusMonitor) GetAllStatuses() []MarketStatus {
	m.mu.RLock()
	assetClasses := make([]string, 0, len(m.config))
	for ac := range m.config {
		assetClasses = append(assetClasses, ac)
	}
	m.mu.RUnlock()

	statuses := make([]MarketStatus, 0, len(assetClasses))
	for _, ac := range assetClasses {
		statuses = append(statuses, m.GetStatus(ac))
	}
	return statuses
}

// Helper functions
func parseMarketTimeSpec(spec interface{}) (*MarketTimeSpec, error) {
	switch v := spec.(type) {
	case string:
		return nil, fmt.Errorf("unsupported string schedule spec: %s", v)
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

func parseMarketDayOfWeek(day string) time.Weekday {
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

func isInMarketTradingWindow(
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

	if current > open || (current == open && currentTime >= openTime) {
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

// BroadcastMarketStatus broadcasts a market status event to clients in relevant contests.
// Only clients in contests whose asset class matches the event's asset class (or "mixed")
// receive the notification, avoiding unnecessary bandwidth for unrelated contests.
func (h *Hub) BroadcastMarketStatus(event *contracts.MarketStatusEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal market status event: %v", err)
		return
	}

	eventAssetClass := event.AssetClass

	// Snapshot the contest → clients mapping
	h.contestClientsMu.RLock()
	type contestEntry struct {
		contestID string
		clients   []*Client
	}
	entries := make([]contestEntry, 0, len(h.contestClients))
	for contestID, clientSet := range h.contestClients {
		clients := make([]*Client, 0, len(clientSet))
		for client := range clientSet {
			clients = append(clients, client)
		}
		entries = append(entries, contestEntry{contestID: contestID, clients: clients})
	}
	h.contestClientsMu.RUnlock()

	// Filter to only clients in contests matching the event's asset class
	var relevantClients []*Client
	for _, entry := range entries {
		contestAC := h.loadContestAssetClass(entry.contestID)

		// A contest receives this market status event if:
		// 1. The contest is "mixed" (receives all market events)
		// 2. The contest's asset class matches the event's asset class
		// 3. The event is for "mixed" (platform-wide status, sent to all)
		if contestAC == "mixed" || contestAC == eventAssetClass || eventAssetClass == "mixed" {
			relevantClients = append(relevantClients, entry.clients...)
		}
	}

	if len(relevantClients) == 0 {
		return
	}

	// Use worker pool for broadcasting to relevant clients
	if h.workerPool != nil && len(relevantClients) > 10 {
		h.workerPool.BroadcastToAll(relevantClients, data)
	} else {
		for _, client := range relevantClients {
			client.SendMessage(data)
		}
	}
}
