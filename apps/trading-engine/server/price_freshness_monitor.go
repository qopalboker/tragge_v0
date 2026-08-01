package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// SymbolFreshnessStatus represents the freshness status of a symbol's price.
type SymbolFreshnessStatus struct {
	Symbol     string    `json:"symbol"`
	LastUpdate time.Time `json:"last_update"`
	AgeSeconds float64   `json:"age_seconds"`
	Status     string    `json:"status"` // "fresh", "warning", "stale"
}

// PriceFreshnessResponse is the response for the /health/prices endpoint.
type PriceFreshnessResponse struct {
	Symbols   []SymbolFreshnessStatus `json:"symbols"`
	Timestamp time.Time               `json:"timestamp"`
}

// PriceFreshnessMonitor monitors the freshness of price data for all tracked symbols.
type PriceFreshnessMonitor struct {
	priceBook        *PriceBook
	metrics          *EngineMetrics
	kafka            *kgo.Client
	config           *Config
	logger           *zap.Logger
	alertsTopic      string
	warningThreshold time.Duration
	alertThreshold   time.Duration
	checkInterval    time.Duration

	// Track which symbols have active alerts to avoid duplicate alerts
	activeAlerts   map[string]time.Time
	activeAlertsMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPriceFreshnessMonitor creates a new price freshness monitor.
func NewPriceFreshnessMonitor(
	priceBook *PriceBook,
	metrics *EngineMetrics,
	kafka *kgo.Client,
	config *Config,
	logger *zap.Logger,
) *PriceFreshnessMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &PriceFreshnessMonitor{
		priceBook:        priceBook,
		metrics:          metrics,
		kafka:            kafka,
		config:           config,
		logger:           logger,
		alertsTopic:      config.AlertsTopic,
		warningThreshold: config.PriceFreshnessWarningThreshold,
		alertThreshold:   config.PriceFreshnessAlertThreshold,
		checkInterval:    config.PriceFreshnessCheckInterval,
		activeAlerts:     make(map[string]time.Time),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins the price freshness monitoring loop.
func (m *PriceFreshnessMonitor) Start() {
	m.wg.Add(1)
	go m.monitorLoop()
	m.logger.Info("Price freshness monitor started",
		zap.Duration("check_interval", m.checkInterval),
		zap.Duration("warning_threshold", m.warningThreshold),
		zap.Duration("alert_threshold", m.alertThreshold))
}

// Stop gracefully stops the price freshness monitor.
func (m *PriceFreshnessMonitor) Stop() {
	m.cancel()
	m.wg.Wait()
	m.logger.Info("Price freshness monitor stopped")
}

// monitorLoop runs the periodic freshness check.
func (m *PriceFreshnessMonitor) monitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkFreshness()
		}
	}
}

// checkFreshness checks the freshness of all tracked symbols.
func (m *PriceFreshnessMonitor) checkFreshness() {
	symbols := m.priceBook.GetAllSymbols()
	now := time.Now()

	for _, symbol := range symbols {
		age, exists := m.priceBook.GetPriceAge(symbol)
		if !exists {
			continue
		}

		ageSeconds := age.Seconds()

		// Check if price is stale (above alert threshold)
		if age > m.alertThreshold {
			// Update gauge metric
			if m.metrics != nil && m.metrics.PriceStalenessSeconds != nil {
				m.metrics.PriceStalenessSeconds.WithLabelValues(symbol).Set(ageSeconds)
			}

			// Log error and publish alert (with deduplication)
			m.handleStalePrice(symbol, age, now)
		} else if age > m.warningThreshold {
			// Update gauge metric for warning level
			if m.metrics != nil && m.metrics.PriceStalenessSeconds != nil {
				m.metrics.PriceStalenessSeconds.WithLabelValues(symbol).Set(ageSeconds)
			}

			// Clear any active alert for this symbol since it's now just warning level
			m.clearAlert(symbol)
		} else {
			// Price is fresh - reset the gauge to 0 and clear any alerts
			if m.metrics != nil && m.metrics.PriceStalenessSeconds != nil {
				m.metrics.PriceStalenessSeconds.WithLabelValues(symbol).Set(0)
			}
			m.clearAlert(symbol)
		}
	}
}

// handleStalePrice handles a stale price by logging and publishing an alert.
func (m *PriceFreshnessMonitor) handleStalePrice(symbol string, age time.Duration, now time.Time) {
	// Check if we already have an active alert for this symbol
	// to avoid flooding with duplicate alerts
	m.activeAlertsMu.RLock()
	lastAlert, hasAlert := m.activeAlerts[symbol]
	m.activeAlertsMu.RUnlock()

	// Only publish a new alert if:
	// 1. No active alert exists, OR
	// 2. The last alert was more than 60 seconds ago (re-alert interval)
	reAlertInterval := 60 * time.Second
	if hasAlert && now.Sub(lastAlert) < reAlertInterval {
		return
	}

	// Get the quote to get the actual last update timestamp
	quote, exists := m.priceBook.GetQuote(symbol)
	if !exists {
		return
	}

	lastUpdateTs := quote.Timestamp

	// Log the error
	m.logger.Error("Price data is stale",
		zap.String("symbol", symbol),
		zap.Float64("age_seconds", age.Seconds()),
		zap.Float64("threshold_seconds", m.alertThreshold.Seconds()),
		zap.Int64("last_update_ts", lastUpdateTs))

	// Publish alert to Kafka
	m.publishAlert(symbol, lastUpdateTs, age)

	// Update active alert tracking
	m.activeAlertsMu.Lock()
	m.activeAlerts[symbol] = now
	m.activeAlertsMu.Unlock()

	// Increment alert counter metric
	if m.metrics != nil && m.metrics.PriceStaleAlerts != nil {
		m.metrics.PriceStaleAlerts.WithLabelValues(symbol).Inc()
	}
}

// clearAlert clears the active alert for a symbol.
func (m *PriceFreshnessMonitor) clearAlert(symbol string) {
	m.activeAlertsMu.Lock()
	delete(m.activeAlerts, symbol)
	m.activeAlertsMu.Unlock()
}

// publishAlert publishes a price stale alert to Kafka.
func (m *PriceFreshnessMonitor) publishAlert(symbol string, lastUpdateTs int64, age time.Duration) {
	if m.kafka == nil {
		return
	}

	alert := &contracts.PriceStaleAlert{
		AlertID:          uuid.New().String(),
		Type:             contracts.AlertTypePriceStale,
		Severity:         contracts.AlertSeverityCritical,
		Symbol:           symbol,
		LastUpdateTs:     lastUpdateTs,
		AgeSeconds:       age.Seconds(),
		ThresholdSeconds: m.alertThreshold.Seconds(),
		Source:           "trading-engine",
		Ts:               time.Now().UnixMilli(),
	}

	data, err := json.Marshal(alert)
	if err != nil {
		m.logger.Error("Failed to marshal price stale alert",
			zap.String("symbol", symbol),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: m.alertsTopic,
		Key:   []byte(symbol),
		Value: data,
	}

	m.kafka.Produce(m.ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			m.logger.Error("Failed to publish price stale alert",
				zap.String("symbol", symbol),
				zap.Error(err))
		} else {
			m.logger.Info("Published price stale alert",
				zap.String("symbol", symbol),
				zap.String("alert_id", alert.AlertID))
		}
	})
}

// GetFreshnessStatus returns the freshness status of all tracked symbols.
func (m *PriceFreshnessMonitor) GetFreshnessStatus() *PriceFreshnessResponse {
	symbols := m.priceBook.GetAllSymbols()
	statuses := make([]SymbolFreshnessStatus, 0, len(symbols))

	for _, symbol := range symbols {
		quote, exists := m.priceBook.GetQuote(symbol)
		if !exists {
			continue
		}

		lastUpdate := time.UnixMilli(quote.Timestamp)
		age := time.Since(lastUpdate)
		ageSeconds := age.Seconds()

		var status string
		switch {
		case age > m.alertThreshold:
			status = "stale"
		case age > m.warningThreshold:
			status = "warning"
		default:
			status = "fresh"
		}

		statuses = append(statuses, SymbolFreshnessStatus{
			Symbol:     symbol,
			LastUpdate: lastUpdate,
			AgeSeconds: ageSeconds,
			Status:     status,
		})
	}

	return &PriceFreshnessResponse{
		Symbols:   statuses,
		Timestamp: time.Now(),
	}
}
