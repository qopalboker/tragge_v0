package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Prometheus metrics for state consistency
var (
	stateConsistencyChecks = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "trading_engine_state_consistency_checks_total",
			Help: "Total number of state consistency checks performed",
		},
	)

	stateConsistencyErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "trading_engine_state_consistency_errors_total",
			Help: "Total number of state consistency errors detected",
		},
	)

	stateReloadAttempts = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "trading_engine_state_reload_attempts_total",
			Help: "Total number of state reload attempts",
		},
	)

	stateReloadSuccess = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "trading_engine_state_reload_success_total",
			Help: "Total number of successful state reloads",
		},
	)
)

// StateConsistencyChecker compares in-memory state against database records.
type StateConsistencyChecker struct {
	db              *sql.DB
	state           StateProvider
	shardedState    *ShardedStateManager
	wal             *WriteAheadLog
	logger          *zap.Logger
	sampleSize      int // Number of positions to sample per check
	shardingEnabled bool
}

// NewStateConsistencyChecker creates a new state consistency checker.
func NewStateConsistencyChecker(
	db *sql.DB,
	state StateProvider,
	shardedState *ShardedStateManager,
	wal *WriteAheadLog,
	logger *zap.Logger,
	shardingEnabled bool,
) *StateConsistencyChecker {
	return &StateConsistencyChecker{
		db:              db,
		state:           state,
		shardedState:    shardedState,
		wal:             wal,
		logger:          logger,
		sampleSize:      50, // Sample 50 positions per check
		shardingEnabled: shardingEnabled,
	}
}

// ConsistencyReport holds the results of a consistency check.
type ConsistencyReport struct {
	CheckTime       time.Time             `json:"check_time"`
	TotalChecked    int                   `json:"total_checked"`
	Consistent      int                   `json:"consistent"`
	Inconsistent    int                   `json:"inconsistent"`
	Errors          int                   `json:"errors"`
	IsConsistent    bool                  `json:"is_consistent"`
	WALStats        WALStats              `json:"wal_stats"`
	Discrepancies   []PositionDiscrepancy `json:"discrepancies,omitempty"`
	DurationMs      int64                 `json:"duration_ms"`
	ShardingEnabled bool                  `json:"sharding_enabled"`
}

// PositionDiscrepancy describes a single position inconsistency.
type PositionDiscrepancy struct {
	PositionID    string `json:"position_id"`
	ContestID     string `json:"contest_id"`
	UserID        string `json:"user_id"`
	Symbol        string `json:"symbol"`
	Field         string `json:"field"`
	MemoryValue   string `json:"memory_value"`
	DatabaseValue string `json:"database_value"`
}

// Check performs a consistency check comparing in-memory state with database.
func (scc *StateConsistencyChecker) Check(ctx context.Context) (*ConsistencyReport, error) {
	startTime := time.Now()
	stateConsistencyChecks.Inc()

	report := &ConsistencyReport{
		CheckTime:       startTime,
		IsConsistent:    true,
		ShardingEnabled: scc.shardingEnabled,
		Discrepancies:   make([]PositionDiscrepancy, 0),
	}

	// Get WAL stats
	if scc.wal != nil {
		report.WALStats = scc.wal.GetStats()
		if report.WALStats.IsDiverged {
			report.IsConsistent = false
		}
	}

	// Sample positions to check
	positions, err := scc.samplePositions(ctx)
	if err != nil {
		report.Errors++
		scc.logger.Error("Failed to sample positions for consistency check", zap.Error(err))
		return report, err
	}

	// Check each position
	for _, dbPos := range positions {
		report.TotalChecked++

		discrepancies, err := scc.checkPosition(ctx, dbPos)
		if err != nil {
			report.Errors++
			continue
		}

		if len(discrepancies) > 0 {
			report.Inconsistent++
			report.IsConsistent = false
			report.Discrepancies = append(report.Discrepancies, discrepancies...)
			stateConsistencyErrors.Inc()
		} else {
			report.Consistent++
		}
	}

	report.DurationMs = time.Since(startTime).Milliseconds()

	// Log result
	if !report.IsConsistent {
		scc.logger.Warn("State consistency check found discrepancies",
			zap.Int("total_checked", report.TotalChecked),
			zap.Int("inconsistent", report.Inconsistent),
			zap.Int("discrepancy_count", len(report.Discrepancies)))
	} else {
		scc.logger.Debug("State consistency check passed",
			zap.Int("total_checked", report.TotalChecked))
	}

	return report, nil
}

// samplePositions retrieves a sample of positions from the database.
func (scc *StateConsistencyChecker) samplePositions(ctx context.Context) ([]*DBPosition, error) {
	// Get random sample of open positions
	query := `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE closed_at IS NULL
		ORDER BY RANDOM()
		LIMIT $1
	`

	rows, err := scc.db.QueryContext(ctx, query, scc.sampleSize)
	if err != nil {
		return nil, fmt.Errorf("query positions: %w", err)
	}
	defer rows.Close()

	var positions []*DBPosition
	for rows.Next() {
		pos := &DBPosition{}
		if err := rows.Scan(&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
			&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore); err != nil {
			continue
		}
		positions = append(positions, pos)
	}

	return positions, rows.Err()
}

// checkPosition compares a database position with in-memory state.
func (scc *StateConsistencyChecker) checkPosition(ctx context.Context, dbPos *DBPosition) ([]PositionDiscrepancy, error) {
	var discrepancies []PositionDiscrepancy

	// Get contest state
	var contestState *ContestState
	if scc.shardingEnabled && scc.shardedState != nil {
		// In sharded mode, check if this contest is assigned to us
		if !scc.shardedState.IsAssigned(dbPos.ContestID) {
			// Not our contest, skip
			return nil, nil
		}
		var err error
		contestState, err = scc.shardedState.GetOrCreateContest(dbPos.ContestID)
		if err != nil {
			return nil, fmt.Errorf("get contest state: %w", err)
		}
	} else {
		contestState = scc.state.GetOrCreateContest(dbPos.ContestID)
	}

	if contestState == nil {
		// Contest not loaded in memory - this may be expected
		return nil, nil
	}

	// Get user state
	userState, exists := contestState.GetUser(dbPos.UserID)
	if !exists {
		// User not loaded in memory - this may be expected
		return nil, nil
	}

	// Get position from memory
	memPos, posExists := userState.GetPosition(dbPos.Symbol)
	if !posExists {
		// Position exists in DB but not in memory
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "existence",
			MemoryValue:   "not_found",
			DatabaseValue: "exists",
		})
		return discrepancies, nil
	}

	// Compare fields
	if memPos.PositionID != dbPos.PositionID {
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "position_id",
			MemoryValue:   memPos.PositionID,
			DatabaseValue: dbPos.PositionID,
		})
	}

	if memPos.QtyOpen != dbPos.QtyOpen {
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "qty_open",
			MemoryValue:   fmt.Sprintf("%d", memPos.QtyOpen),
			DatabaseValue: fmt.Sprintf("%d", dbPos.QtyOpen),
		})
	}

	// Compare entry price with tolerance
	if !floatsEqual(memPos.EntryPrice, dbPos.EntryPrice, 0.0001) {
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "entry_price",
			MemoryValue:   fmt.Sprintf("%.6f", memPos.EntryPrice),
			DatabaseValue: fmt.Sprintf("%.6f", dbPos.EntryPrice),
		})
	}

	if memPos.QtyUsed != dbPos.QtyUsed {
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "qty_used",
			MemoryValue:   fmt.Sprintf("%d", memPos.QtyUsed),
			DatabaseValue: fmt.Sprintf("%d", dbPos.QtyUsed),
		})
	}

	// Compare realized score with tolerance
	if !floatsEqual(memPos.RealizedScore, dbPos.RealizedScore, 0.01) {
		discrepancies = append(discrepancies, PositionDiscrepancy{
			PositionID:    dbPos.PositionID,
			ContestID:     dbPos.ContestID,
			UserID:        dbPos.UserID,
			Symbol:        dbPos.Symbol,
			Field:         "realized_score",
			MemoryValue:   fmt.Sprintf("%.4f", memPos.RealizedScore),
			DatabaseValue: fmt.Sprintf("%.4f", dbPos.RealizedScore),
		})
	}

	return discrepancies, nil
}

// floatsEqual compares two floats with a tolerance.
func floatsEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// StateReloader handles reloading state from database when divergence is detected.
type StateReloader struct {
	db              *sql.DB
	state           StateProvider
	shardedState    *ShardedStateManager
	pendingBook     *PendingOrderBook
	logger          *zap.Logger
	shardingEnabled bool
	reloadMu        sync.Mutex
	lastReload      time.Time
}

// NewStateReloader creates a new state reloader.
func NewStateReloader(
	db *sql.DB,
	state StateProvider,
	shardedState *ShardedStateManager,
	pendingBook *PendingOrderBook,
	logger *zap.Logger,
	shardingEnabled bool,
) *StateReloader {
	return &StateReloader{
		db:              db,
		state:           state,
		shardedState:    shardedState,
		pendingBook:     pendingBook,
		logger:          logger,
		shardingEnabled: shardingEnabled,
	}
}

// ReloadState reloads all in-memory state from the database.
func (sr *StateReloader) ReloadState(ctx context.Context) error {
	sr.reloadMu.Lock()
	defer sr.reloadMu.Unlock()

	stateReloadAttempts.Inc()
	startTime := time.Now()

	sr.logger.Info("Starting state reload from database")

	if sr.shardingEnabled && sr.shardedState != nil {
		// For sharded mode, use the existing WarmUp mechanism
		if err := sr.shardedState.WarmUp(ctx); err != nil {
			sr.logger.Error("Failed to warm up sharded state during reload", zap.Error(err))
			return fmt.Errorf("warmup sharded state: %w", err)
		}
	} else {
		// For non-sharded mode, reload each contest
		if err := sr.reloadNonShardedState(ctx); err != nil {
			sr.logger.Error("Failed to reload non-sharded state", zap.Error(err))
			return fmt.Errorf("reload non-sharded state: %w", err)
		}
	}

	// Repopulate the pending order book (symbol-indexed structure used by tick processing).
	// Without this, pending orders and TP/SL won't trigger after divergence recovery.
	if sr.pendingBook != nil {
		sr.pendingBook.Clear()
		if err := sr.pendingBook.ReloadFromDB(ctx, sr.db, sr.logger); err != nil {
			sr.logger.Error("Failed to reload pending order book after state reload", zap.Error(err))
			return fmt.Errorf("reload pending book: %w", err)
		}
	}

	sr.lastReload = time.Now()
	stateReloadSuccess.Inc()

	sr.logger.Info("State reload completed successfully",
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// reloadNonShardedState reloads state for non-sharded mode.
func (sr *StateReloader) reloadNonShardedState(ctx context.Context) error {
	// Get all running contests
	rows, err := sr.db.QueryContext(ctx, `
		SELECT id FROM contests WHERE status = 'running'
	`)
	if err != nil {
		return fmt.Errorf("query running contests: %w", err)
	}
	defer rows.Close()

	var contestIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		contestIDs = append(contestIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate contests: %w", err)
	}

	// Reload each contest
	for _, contestID := range contestIDs {
		if err := sr.reloadContest(ctx, contestID); err != nil {
			sr.logger.Warn("Failed to reload contest",
				zap.String("contest_id", contestID),
				zap.Error(err))
			// Continue with other contests
		}
	}

	return nil
}

// reloadContest reloads a single contest's state from database.
// Uses batch queries (3 total) instead of per-user queries to avoid N+1 problem.
func (sr *StateReloader) reloadContest(ctx context.Context, contestID string) error {
	contestState := sr.state.GetOrCreateContest(contestID)
	if contestState == nil {
		return fmt.Errorf("failed to get/create contest state")
	}

	// --- Batch query 1: Load all participants ---
	participantRows, err := sr.db.QueryContext(ctx, `
		SELECT user_id, qty_total, qty_available, total_score
		FROM contest_participants
		WHERE contest_id = $1
	`, contestID)
	if err != nil {
		return fmt.Errorf("query participants: %w", err)
	}

	type participantData struct {
		QtyTotal     int64
		QtyAvailable int64
		TotalScore   float64
	}
	participants := make(map[string]*participantData)
	for participantRows.Next() {
		var userID string
		var pd participantData
		if err := participantRows.Scan(&userID, &pd.QtyTotal, &pd.QtyAvailable, &pd.TotalScore); err != nil {
			continue
		}
		participants[userID] = &pd
	}
	if err := participantRows.Err(); err != nil {
		participantRows.Close()
		return fmt.Errorf("iterate participants: %w", err)
	}
	participantRows.Close()

	// --- Batch query 2: Load all open positions (including TP/SL) ---
	posRows, err := sr.db.QueryContext(ctx, `
		SELECT position_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score,
		       take_profit, stop_loss
		FROM positions
		WHERE contest_id = $1 AND closed_at IS NULL
	`, contestID)
	if err != nil {
		return fmt.Errorf("query positions: %w", err)
	}

	type positionEntry struct {
		State      *PositionState
		TakeProfit *float64
		StopLoss   *float64
	}
	positionsByUser := make(map[string][]*positionEntry)
	for posRows.Next() {
		var pos DBPosition
		var userID, sideStr string
		var tp, sl sql.NullFloat64
		if err := posRows.Scan(&pos.PositionID, &userID, &pos.Symbol, &sideStr,
			&pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore,
			&tp, &sl); err != nil {
			continue
		}

		posState := &PositionState{
			PositionID:           pos.PositionID,
			Symbol:               pos.Symbol,
			QtyOpen:              pos.QtyOpen,
			EntryPrice:           pos.EntryPrice,
			EntryPriceDecimal:    decimal.NewFromFloat(pos.EntryPrice),
			QtyUsed:              pos.QtyUsed,
			RealizedScore:        pos.RealizedScore,
			RealizedScoreDecimal: decimal.NewFromFloat(pos.RealizedScore),
		}
		posState.Side = PositionSideToOrderSide(sideStr)

		entry := &positionEntry{State: posState}
		if tp.Valid {
			tpVal := tp.Float64
			entry.TakeProfit = &tpVal
		}
		if sl.Valid {
			slVal := sl.Float64
			entry.StopLoss = &slVal
		}
		positionsByUser[userID] = append(positionsByUser[userID], entry)
	}
	posRows.Close()

	// --- Batch query 3: Load all pending orders ---
	pendingRows, err := sr.db.QueryContext(ctx, `
		SELECT order_id, user_id, symbol, side, type, qty, qty_filled, limit_price, stop_price
		FROM orders
		WHERE contest_id = $1 AND status = 'open'
	`, contestID)
	if err != nil {
		return fmt.Errorf("query pending orders: %w", err)
	}

	type pendingEntry struct {
		Order  *PendingOrder
		UserID string
	}
	pendingByUser := make(map[string][]*pendingEntry)
	for pendingRows.Next() {
		var orderID, userID, symbol, sideStr, typeStr string
		var qty, qtyFilled int64
		var limitPrice, stopPrice sql.NullFloat64

		if err := pendingRows.Scan(&orderID, &userID, &symbol, &sideStr, &typeStr,
			&qty, &qtyFilled, &limitPrice, &stopPrice); err != nil {
			continue
		}

		pending := &PendingOrder{
			OrderID:   orderID,
			Symbol:    symbol,
			Qty:       qty,
			QtyFilled: qtyFilled,
		}
		pending.Side = DBOrderSideToOrderSide(sideStr)

		orderType := contracts.OrderTypeLimit
		switch typeStr {
		case "stop":
			orderType = contracts.OrderTypeStop
		case "buy_limit":
			orderType = contracts.OrderTypeBuyLimit
		case "sell_limit":
			orderType = contracts.OrderTypeSellLimit
		case "buy_stop":
			orderType = contracts.OrderTypeBuyStop
		case "sell_stop":
			orderType = contracts.OrderTypeSellStop
		}
		pending.Type = orderType

		if limitPrice.Valid {
			lp := limitPrice.Float64
			pending.LimitPrice = &lp
		}
		if stopPrice.Valid {
			sp := stopPrice.Float64
			pending.StopPrice = &sp
		}

		pendingByUser[userID] = append(pendingByUser[userID], &pendingEntry{Order: pending, UserID: userID})
	}
	pendingRows.Close()

	// --- Assemble state from batch results ---
	for userID, pd := range participants {
		userState := contestState.GetOrCreateUser(userID, pd.QtyTotal, pd.QtyAvailable, pd.TotalScore)

		// Clear existing positions and pending orders, then reload from batch results
		userState.mu.Lock()
		userState.Positions = make(map[string]*PositionState)
		userState.PendingOrders = make(map[string]*PendingOrder)
		userState.QtyTotal = pd.QtyTotal
		userState.QtyAvailable = pd.QtyAvailable
		userState.RealizedScore = pd.TotalScore
		userState.RealizedScoreDecimal = decimal.NewFromFloat(pd.TotalScore)
		userState.mu.Unlock()

		// Assign positions from batch result
		for _, entry := range positionsByUser[userID] {
			userState.SetPosition(entry.State)

			// Register TP/SL positions in pendingBook for tick evaluation
			if sr.pendingBook != nil && (entry.TakeProfit != nil || entry.StopLoss != nil) {
				sr.pendingBook.AddPositionWithTPSL(&PositionWithTPSL{
					PositionID: entry.State.PositionID,
					ContestID:  contestID,
					UserID:     userID,
					Symbol:     entry.State.Symbol,
					Side:       entry.State.Side,
					QtyOpen:    entry.State.QtyOpen,
					EntryPrice: entry.State.EntryPrice,
					TakeProfit: entry.TakeProfit,
					StopLoss:   entry.StopLoss,
				})
			}
		}

		// Assign pending orders from batch result
		for _, entry := range pendingByUser[userID] {
			userState.AddPendingOrder(entry.Order)

			// Register in pendingBook for symbol-indexed tick evaluation
			if sr.pendingBook != nil {
				sr.pendingBook.AddPendingOrder(&PendingOrderInfo{
					OrderID:    entry.Order.OrderID,
					ContestID:  contestID,
					UserID:     userID,
					Symbol:     entry.Order.Symbol,
					Side:       entry.Order.Side,
					Type:       entry.Order.Type,
					Qty:        entry.Order.Qty - entry.Order.QtyFilled,
					LimitPrice: entry.Order.LimitPrice,
					StopPrice:  entry.Order.StopPrice,
				})
			}
		}
	}

	return nil
}

// GetLastReloadTime returns the time of the last successful reload.
func (sr *StateReloader) GetLastReloadTime() time.Time {
	sr.reloadMu.Lock()
	defer sr.reloadMu.Unlock()
	return sr.lastReload
}

// HandleStateConsistency is the HTTP handler for the state consistency endpoint.
func (scc *StateConsistencyChecker) HandleStateConsistency(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	report, err := scc.Check(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"status": "error",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if !report.IsConsistent {
		w.WriteHeader(http.StatusConflict) // 409 Conflict
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(report)
}
