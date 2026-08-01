package server

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DynamicSymbolManager tracks which symbols are needed by active contests
// and manages provider subscriptions accordingly. It ref-counts symbols
// so a symbol is only unsubscribed when the last contest using it ends.
type DynamicSymbolManager struct {
	db     *sql.DB
	logger *zap.Logger

	// Provider references for subscribe/unsubscribe
	massiveProvider *MassiveProvider
	nobitexFeed     *NobitexCryptoFeed
	binanceFeed     *BinanceCryptoFeed

	// symbol → count of active contests using it
	activeSymbols map[string]int
	mu            sync.RWMutex
}

func NewDynamicSymbolManager(db *sql.DB, logger *zap.Logger) *DynamicSymbolManager {
	return &DynamicSymbolManager{
		db:            db,
		logger:        logger,
		activeSymbols: make(map[string]int),
	}
}

// SetProviders wires the manager to the actual market data providers.
// Called after providers are initialized in Run().
func (m *DynamicSymbolManager) SetProviders(massive *MassiveProvider, nobitex *NobitexCryptoFeed, binance *BinanceCryptoFeed) {
	m.massiveProvider = massive
	m.nobitexFeed = nobitex
	m.binanceFeed = binance
}

// Start runs the periodic refresh loop as a safety net.
// Primary updates come from Kafka contest events via OnContestEvent.
func (m *DynamicSymbolManager) Start(ctx context.Context) {
	// Initial load from DB
	if err := m.RefreshFromDB(ctx); err != nil {
		m.logger.Error("initial symbol refresh failed", zap.Error(err))
	}

	// Periodic refresh every 60s catches missed Kafka events
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.RefreshFromDB(ctx); err != nil {
				m.logger.Warn("periodic symbol refresh failed", zap.Error(err))
			}
		}
	}
}

// RefreshFromDB queries all symbols needed by active/scheduled contests
// and reconciles subscriptions.
func (m *DynamicSymbolManager) RefreshFromDB(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `
		SELECT cs.symbol, COUNT(DISTINCT cs.contest_id) as contest_count
		FROM contest_symbols cs
		JOIN contests c ON c.id = cs.contest_id
		WHERE c.status IN ('scheduled', 'registration_open', 'running')
		GROUP BY cs.symbol
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	needed := make(map[string]int)
	for rows.Next() {
		var sym string
		var count int
		if err := rows.Scan(&sym, &count); err != nil {
			return err
		}
		needed[sym] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Fallback: if no active contests, use SYMBOLS_FALLBACK env var
	if len(needed) == 0 {
		if fallback := os.Getenv("SYMBOLS_FALLBACK"); fallback != "" {
			for _, sym := range strings.Split(fallback, ",") {
				sym = strings.TrimSpace(sym)
				if sym != "" {
					needed[sym] = 0 // 0 = fallback, not contest-backed
				}
			}
			m.logger.Info("no active contests, using fallback symbols",
				zap.Int("count", len(needed)))
		} else {
			m.logger.Info("no active contests, no symbols subscribed")
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Unsubscribe symbols no longer needed
	for sym := range m.activeSymbols {
		if _, ok := needed[sym]; !ok {
			m.unsubscribe(sym)
			m.logger.Info("unsubscribed symbol (no active contests)", zap.String("symbol", sym))
		}
	}

	// Subscribe new symbols
	for sym, count := range needed {
		if _, ok := m.activeSymbols[sym]; !ok {
			m.subscribe(sym)
			m.logger.Info("subscribed symbol for active contest",
				zap.String("symbol", sym),
				zap.Int("contest_count", count))
		}
	}

	m.activeSymbols = needed
	m.logger.Info("symbol refresh complete",
		zap.Int("active_symbols", len(needed)))

	return nil
}

// OnContestEvent handles real-time contest events from Kafka.
func (m *DynamicSymbolManager) OnContestEvent(ctx context.Context, contestID string, status string) {
	switch status {
	case "scheduled", "registration_open", "running":
		m.addContestSymbols(ctx, contestID)
	case "completed", "cancelled":
		m.removeContestSymbols(ctx, contestID)
	}
}

func (m *DynamicSymbolManager) addContestSymbols(ctx context.Context, contestID string) {
	symbols, err := m.queryContestSymbols(ctx, contestID)
	if err != nil {
		m.logger.Error("failed to query contest symbols",
			zap.String("contest_id", contestID), zap.Error(err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sym := range symbols {
		count := m.activeSymbols[sym]
		if count == 0 {
			m.subscribe(sym)
			m.logger.Info("subscribed symbol for new contest",
				zap.String("symbol", sym),
				zap.String("contest_id", contestID))
		}
		m.activeSymbols[sym] = count + 1
	}
}

func (m *DynamicSymbolManager) removeContestSymbols(ctx context.Context, contestID string) {
	symbols, err := m.queryContestSymbols(ctx, contestID)
	if err != nil {
		m.logger.Error("failed to query contest symbols",
			zap.String("contest_id", contestID), zap.Error(err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sym := range symbols {
		count := m.activeSymbols[sym]
		if count <= 1 {
			m.unsubscribe(sym)
			delete(m.activeSymbols, sym)
			m.logger.Info("unsubscribed symbol (last contest ended)",
				zap.String("symbol", sym),
				zap.String("contest_id", contestID))
		} else {
			m.activeSymbols[sym] = count - 1
		}
	}
}

func (m *DynamicSymbolManager) queryContestSymbols(ctx context.Context, contestID string) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT symbol FROM contest_symbols WHERE contest_id = $1`, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

// subscribe routes to the correct provider based on symbol type.
func (m *DynamicSymbolManager) subscribe(symbol string) {
	if isCryptoSymbol(symbol) {
		if m.nobitexFeed != nil {
			m.nobitexFeed.AddSymbol(symbol)
		}
		if m.binanceFeed != nil {
			m.binanceFeed.AddSymbol(symbol)
		}
	} else if m.massiveProvider != nil {
		if err := m.massiveProvider.SubscribeSymbol(symbol); err != nil {
			m.logger.Warn("failed to subscribe symbol on Massive",
				zap.String("symbol", symbol), zap.Error(err))
		}
	}
}

// unsubscribe routes to the correct provider based on symbol type.
func (m *DynamicSymbolManager) unsubscribe(symbol string) {
	if isCryptoSymbol(symbol) {
		if m.nobitexFeed != nil {
			m.nobitexFeed.RemoveSymbol(symbol)
		}
		if m.binanceFeed != nil {
			m.binanceFeed.RemoveSymbol(symbol)
		}
	} else if m.massiveProvider != nil {
		if err := m.massiveProvider.UnsubscribeSymbol(symbol); err != nil {
			m.logger.Warn("failed to unsubscribe symbol on Massive",
				zap.String("symbol", symbol), zap.Error(err))
		}
	}
}

// isCryptoSymbol checks if a symbol should be routed to crypto providers.
func isCryptoSymbol(symbol string) bool {
	// Use the same logic as isForexSymbol but inverted:
	// if it's not forex, it's crypto.
	return !isForexSymbol(symbol)
}

// GetActiveSymbols returns currently subscribed symbols for status endpoints.
func (m *DynamicSymbolManager) GetActiveSymbols() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	symbols := make([]string, 0, len(m.activeSymbols))
	for sym := range m.activeSymbols {
		symbols = append(symbols, sym)
	}
	return symbols
}

// GetActiveSymbolCount returns the number of active symbols.
func (m *DynamicSymbolManager) GetActiveSymbolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeSymbols)
}
