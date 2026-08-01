package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// EngineMetrics holds Prometheus metrics for the trading engine.
type EngineMetrics struct {
	// Stale price rejection metrics
	OrdersRejectedStalePrice *prometheus.CounterVec

	// Price freshness metrics
	PriceStalenessSeconds *prometheus.GaugeVec
	PriceStaleAlerts      *prometheus.CounterVec

	// Unrealized score broadcast metrics
	UnrealizedBroadcastDuration *prometheus.HistogramVec

	// Cache metrics
	ContestCacheHits       prometheus.Counter
	ContestCacheMisses     prometheus.Counter
	ContestCacheSize       prometheus.Gauge
	ParticipantCacheHits   prometheus.Counter
	ParticipantCacheMisses prometheus.Counter
	ParticipantCacheSize   prometheus.Gauge
	DBQueriesSaved         prometheus.Counter

	// Rate limit effective rate per contest
	RateLimitContestEffective *prometheus.GaugeVec

	// P1-4: Order execution latency metrics
	OrderProcessingDuration *prometheus.HistogramVec
	OrdersProcessedTotal    *prometheus.CounterVec

	// Kafka produce failure metric
	KafkaProduceFailures *prometheus.CounterVec

	// Redis price fallback metric
	RedisPriceFallbackTotal prometheus.Counter
}

// NewEngineMetrics creates and registers metrics for the trading engine.
func NewEngineMetrics(registry prometheus.Registerer, namespace string) *EngineMetrics {
	metrics := &EngineMetrics{
		OrdersRejectedStalePrice: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_rejected_stale_price_total",
			Help:      "Total number of orders rejected due to stale price data",
		}, []string{"contest_id", "order_type"}),

		PriceStalenessSeconds: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "price_staleness_seconds",
			Help:      "Current age in seconds of the latest price for each symbol (only reported when stale)",
		}, []string{"symbol"}),

		PriceStaleAlerts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "price_stale_alerts_total",
			Help:      "Total number of price stale alerts published to Kafka",
		}, []string{"symbol"}),

		UnrealizedBroadcastDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "unrealized_broadcast_duration_seconds",
			Help:      "Duration of unrealized score broadcast operations in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}, []string{"mode"}),

		ContestCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "contest_cache_hits_total",
			Help:      "Total number of contest cache hits",
		}),
		ContestCacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "contest_cache_misses_total",
			Help:      "Total number of contest cache misses",
		}),
		ContestCacheSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "contest_cache_size",
			Help:      "Current number of cached contests",
		}),
		ParticipantCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "participant_cache_hits_total",
			Help:      "Total number of participant cache hits",
		}),
		ParticipantCacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "participant_cache_misses_total",
			Help:      "Total number of participant cache misses",
		}),
		ParticipantCacheSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "participant_cache_size",
			Help:      "Current number of cached participants",
		}),
		DBQueriesSaved: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_queries_saved_total",
			Help:      "Total number of database queries avoided due to cache hits (contest + participant)",
		}),
		RateLimitContestEffective: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "rate_limit_contest_effective",
			Help:      "Current effective rate limit per contest (orders per second)",
		}, []string{"contest_id"}),

		// P1-4: Order execution latency metrics
		OrderProcessingDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "order_processing_duration_seconds",
			Help:      "Total time to process an order from receipt to completion",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		}, []string{"order_type", "result"}),
		OrdersProcessedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_processed_total",
			Help:      "Total number of orders processed",
		}, []string{"order_type", "result"}),
		KafkaProduceFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_produce_failures_total",
			Help:      "Total number of failed Kafka produce operations",
		}, []string{"topic"}),
		RedisPriceFallbackTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "redis_price_fallback_total",
			Help:      "Times getMarketPrice fell back to Redis due to PriceBook miss",
		}),
	}

	// Register metrics
	registry.MustRegister(
		metrics.OrdersRejectedStalePrice,
		metrics.PriceStalenessSeconds,
		metrics.PriceStaleAlerts,
		metrics.UnrealizedBroadcastDuration,
		metrics.ContestCacheHits,
		metrics.ContestCacheMisses,
		metrics.ContestCacheSize,
		metrics.ParticipantCacheHits,
		metrics.ParticipantCacheMisses,
		metrics.ParticipantCacheSize,
		metrics.DBQueriesSaved,
		metrics.RateLimitContestEffective,
		metrics.OrderProcessingDuration,
		metrics.OrdersProcessedTotal,
		metrics.KafkaProduceFailures,
		metrics.RedisPriceFallbackTotal,
	)

	return metrics
}

// StateProvider is an interface for state management that supports both
// regular and sharded state managers.
type StateProvider interface {
	GetOrCreateContest(contestID string) *ContestState
	RemoveContest(contestID string)
}

// ShardValidator is an interface for shard validation.
type ShardValidator interface {
	RejectIfNotAssigned(contestID string) error
	IsReady() bool
	GetStats() ShardStats
}

// Engine is the core trading engine.
type Engine struct {
	db              *sql.DB
	redis           redis.UniversalClient
	kafka           *kgo.Client
	state           StateProvider
	shardedState    *ShardedStateManager // Set when sharding is enabled
	priceBook       *PriceBook
	pendingBook     *PendingOrderBook
	config          *Config
	shardingEnabled bool
	metrics         *EngineMetrics
	positionLocks   *PositionLockManager // Per-position mutex map to prevent race conditions
	logger          *zap.Logger          // Structured logger

	// Rate limiter for order submissions
	rateLimiter *OrderRateLimiter

	// WAL (Write-Ahead Log) for state consistency
	wal           *WriteAheadLog
	stateOperator *StateOperator
	stateReloader *StateReloader

	// Market hours checker for validating trading hours
	marketHours *MarketHoursChecker

	// In-memory caches for contest and participant lookups
	contestCache     *ContestCache
	participantCache *ParticipantCache

	// Cache metrics collection
	cacheMetricsStop      chan struct{}
	lastContestHits       uint64
	lastContestMisses     uint64
	lastParticipantHits   uint64
	lastParticipantMisses uint64

	// Symbol cache for contest symbol validation (P0-1)
	symbolCache *SymbolCache

	// P3-1: Monotonic sequence counter for PnL delta ordering
	pnlSeqNum atomic.Uint64
}

// NewEngine creates a new trading engine.
func NewEngine(db *sql.DB, redis redis.UniversalClient, kafka *kgo.Client, config *Config, logger *zap.Logger) *Engine {
	e := &Engine{
		db:              db,
		redis:           redis,
		kafka:           kafka,
		state:           NewStateManager(),
		priceBook:       NewPriceBook(),
		pendingBook:     NewPendingOrderBook(),
		config:          config,
		shardingEnabled: false,
		positionLocks:   NewPositionLockManager(db, nil, "trading_engine", logger),
		logger:          logger,
		contestCache: NewContestCache(ContestCacheConfig{
			TTL:             config.ContestCacheTTL,
			CleanupInterval: config.CacheCleanupInterval,
		}),
		participantCache: NewParticipantCache(ParticipantCacheConfig{
			TTL:             config.ParticipantCacheTTL,
			CleanupInterval: config.CacheCleanupInterval,
		}),
		symbolCache: NewSymbolCache(config.ContestCacheTTL, config.CacheCleanupInterval),
	}

	// Initialize WAL (Write-Ahead Log) for state consistency - logger will be set later
	e.wal = NewWriteAheadLog(WALConfig{
		MaxEntries:  DefaultWALConfig().MaxEntries,
		PersistPath: config.WALPersistPath,
	}, nil)

	return e
}

// NewShardedEngine creates a new trading engine with sharding enabled.
func NewShardedEngine(db *sql.DB, redis redis.UniversalClient, kafka *kgo.Client, config *Config, shardedState *ShardedStateManager, logger *zap.Logger) *Engine {
	e := &Engine{
		db:              db,
		redis:           redis,
		kafka:           kafka,
		state:           NewStateManagerAdapter(shardedState),
		shardedState:    shardedState,
		priceBook:       NewPriceBook(),
		pendingBook:     NewPendingOrderBook(),
		config:          config,
		shardingEnabled: true,
		positionLocks:   NewPositionLockManager(db, nil, "trading_engine", logger),
		logger:          logger,
		contestCache: NewContestCache(ContestCacheConfig{
			TTL:             config.ContestCacheTTL,
			CleanupInterval: config.CacheCleanupInterval,
		}),
		participantCache: NewParticipantCache(ParticipantCacheConfig{
			TTL:             config.ParticipantCacheTTL,
			CleanupInterval: config.CacheCleanupInterval,
		}),
		symbolCache: NewSymbolCache(config.ContestCacheTTL, config.CacheCleanupInterval),
	}

	// Initialize WAL (Write-Ahead Log) for state consistency - logger will be set later
	e.wal = NewWriteAheadLog(WALConfig{
		MaxEntries:  DefaultWALConfig().MaxEntries,
		PersistPath: config.WALPersistPath,
	}, nil)

	return e
}

// SetMetrics sets the metrics for the engine.
func (e *Engine) SetMetrics(metrics *EngineMetrics) {
	e.metrics = metrics
}

// SetRateLimiter sets the rate limiter for the engine.
func (e *Engine) SetRateLimiter(rateLimiter *OrderRateLimiter) {
	e.rateLimiter = rateLimiter
}

// GetRateLimiter returns the rate limiter for monitoring/stats.
func (e *Engine) GetRateLimiter() *OrderRateLimiter {
	return e.rateLimiter
}

// SetMarketHoursChecker sets the market hours checker for the engine.
func (e *Engine) SetMarketHoursChecker(marketHours *MarketHoursChecker) {
	e.marketHours = marketHours
}

// isSymbolAllowedInContest checks if a symbol is allowed for trading in a contest.
// Uses an in-memory cache to avoid repeated DB queries.
func (e *Engine) isSymbolAllowedInContest(ctx context.Context, contestID, symbol string) (bool, error) {
	// Check cache first
	if symbols, hit := e.symbolCache.Get(contestID); hit {
		return symbols[symbol], nil
	}

	// Cache miss - query database
	symbols, err := GetContestSymbols(ctx, e.db, contestID)
	if err != nil {
		return false, err
	}

	// If no symbols configured, allow all (backward compatibility)
	if len(symbols) == 0 {
		return true, nil
	}

	e.symbolCache.Set(contestID, symbols)
	return symbols[symbol], nil
}

// GetMarketHoursChecker returns the market hours checker.
func (e *Engine) GetMarketHoursChecker() *MarketHoursChecker {
	return e.marketHours
}

// StartCacheMetricsLoop starts a background goroutine that pushes cache and
// rate limiter statistics to Prometheus gauges/counters every 10 seconds.
func (e *Engine) StartCacheMetricsLoop() {
	if e.metrics == nil {
		return
	}
	e.cacheMetricsStop = make(chan struct{})
	go e.cacheMetricsLoop()
}

// StopCacheMetricsLoop signals the cache metrics goroutine to exit.
func (e *Engine) StopCacheMetricsLoop() {
	if e.cacheMetricsStop != nil {
		close(e.cacheMetricsStop)
	}
}

func (e *Engine) cacheMetricsLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.updateCacheMetrics()
		case <-e.cacheMetricsStop:
			return
		}
	}
}

// updateCacheMetrics reads Stats() from both caches and the rate limiter,
// then updates the Prometheus metrics. Counters are set by computing the
// delta from the previous scrape (since atomic counters on the caches are
// monotonically increasing). We use prometheus.Counter.Add(delta) to stay
// compatible with Prometheus semantics.
func (e *Engine) updateCacheMetrics() {
	if e.metrics == nil {
		return
	}

	// Contest cache
	cs := e.contestCache.Stats()
	e.metrics.ContestCacheSize.Set(float64(cs.Entries))

	// We snapshot the atomic counters and add the delta since last push.
	// On first call lastContestHits/lastContestMisses are 0, so we add the full count.
	contestHitsDelta := cs.Hits - e.lastContestHits
	contestMissesDelta := cs.Misses - e.lastContestMisses
	if contestHitsDelta > 0 {
		e.metrics.ContestCacheHits.Add(float64(contestHitsDelta))
		e.metrics.DBQueriesSaved.Add(float64(contestHitsDelta))
	}
	if contestMissesDelta > 0 {
		e.metrics.ContestCacheMisses.Add(float64(contestMissesDelta))
	}
	e.lastContestHits = cs.Hits
	e.lastContestMisses = cs.Misses

	// Participant cache
	ps := e.participantCache.Stats()
	e.metrics.ParticipantCacheSize.Set(float64(ps.Entries))

	participantHitsDelta := ps.Hits - e.lastParticipantHits
	participantMissesDelta := ps.Misses - e.lastParticipantMisses
	if participantHitsDelta > 0 {
		e.metrics.ParticipantCacheHits.Add(float64(participantHitsDelta))
		e.metrics.DBQueriesSaved.Add(float64(participantHitsDelta))
	}
	if participantMissesDelta > 0 {
		e.metrics.ParticipantCacheMisses.Add(float64(participantMissesDelta))
	}
	e.lastParticipantHits = ps.Hits
	e.lastParticipantMisses = ps.Misses

	// Rate limiter effective rates per contest
	if e.rateLimiter != nil {
		rates := e.rateLimiter.GetContestEffectiveRates()
		for contestID, rate := range rates {
			e.metrics.RateLimitContestEffective.WithLabelValues(contestID).Set(float64(rate))
		}
	}
}

// InitWAL initializes the Write-Ahead Log components with the provided logger.
// This should be called after the engine is created and the logger is available.
func (e *Engine) InitWAL(logger *zap.Logger) {
	// Update WAL with logger
	if e.wal != nil {
		e.wal.logger = logger
	}

	// Create state operator
	e.stateOperator = NewStateOperator(e.wal, e.db, logger)

	// Create state reloader
	e.stateReloader = NewStateReloader(e.db, e.state, e.shardedState, e.pendingBook, logger, e.shardingEnabled)

	// Set divergence callback to trigger state reload
	e.stateOperator.SetDivergenceCallback(func(ctx context.Context) error {
		return e.stateReloader.ReloadState(ctx)
	})

	logger.Info("WAL components initialized for state consistency protection")
}

// GetWAL returns the Write-Ahead Log for monitoring/stats.
func (e *Engine) GetWAL() *WriteAheadLog {
	return e.wal
}

// GetStateReloader returns the state reloader.
func (e *Engine) GetStateReloader() *StateReloader {
	return e.stateReloader
}

// IsStateDiverged returns whether state divergence has been detected.
func (e *Engine) IsStateDiverged() bool {
	if e.wal == nil {
		return false
	}
	return e.wal.IsDiverged()
}

// ReplayWAL replays pending WAL entries on startup.
func (e *Engine) ReplayWAL(ctx context.Context) error {
	if e.stateOperator == nil {
		return nil
	}

	return e.stateOperator.ReplayPendingEntries(ctx, func(entry WALEntry) error {
		// Apply the entry to in-memory state based on operation type
		return e.applyWALEntry(entry)
	})
}

// applyWALEntry applies a WAL entry to in-memory state.
func (e *Engine) applyWALEntry(entry WALEntry) error {
	contestState := e.state.GetOrCreateContest(entry.ContestID)
	if contestState == nil {
		return fmt.Errorf("contest not found: %s", entry.ContestID)
	}

	userState, exists := contestState.GetUser(entry.UserID)
	if !exists {
		// Create user state from DB
		participant, err := GetParticipant(context.Background(), e.db, entry.ContestID, entry.UserID)
		if err != nil || participant == nil {
			return fmt.Errorf("participant not found: %s/%s", entry.ContestID, entry.UserID)
		}
		userState = contestState.GetOrCreateUser(entry.UserID, participant.QtyTotal, participant.QtyAvailable, participant.TotalScore)
	}

	switch entry.Operation {
	case WALOpCreatePosition, WALOpUpdatePosition:
		var data PositionUpdateData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return fmt.Errorf("unmarshal position data: %w", err)
		}
		side := PositionSideToOrderSide(data.Side)
		userState.SetPosition(&PositionState{
			PositionID:    data.PositionID,
			Symbol:        data.Symbol,
			Side:          side,
			QtyOpen:       data.QtyOpen,
			EntryPrice:    data.EntryPrice,
			QtyUsed:       data.QtyUsed,
			RealizedScore: data.RealizedScore,
		})

	case WALOpClosePosition:
		var data PositionUpdateData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return fmt.Errorf("unmarshal position data: %w", err)
		}
		userState.RemovePosition(data.Symbol)

	case WALOpUpdateQtyAvailable, WALOpUpdateRealizedScore:
		var data QtyScoreUpdateData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return fmt.Errorf("unmarshal qty/score data: %w", err)
		}
		userState.mu.Lock()
		userState.QtyAvailable = data.NewQtyAvailable
		userState.RealizedScore = data.NewRealizedScore
		userState.mu.Unlock()

	case WALOpAddPendingOrder:
		var data PendingOrderData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return fmt.Errorf("unmarshal pending order data: %w", err)
		}
		side := DBOrderSideToOrderSide(data.Side)
		orderType := contracts.OrderTypeLimit
		switch data.Type {
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
		userState.AddPendingOrder(&PendingOrder{
			OrderID:    data.OrderID,
			Symbol:     data.Symbol,
			Side:       side,
			Type:       orderType,
			Qty:        data.Qty,
			LimitPrice: data.LimitPrice,
			StopPrice:  data.StopPrice,
		})

	case WALOpRemovePendingOrder:
		var data PendingOrderData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return fmt.Errorf("unmarshal pending order data: %w", err)
		}
		userState.RemovePendingOrder(data.OrderID)
	}

	return nil
}

// safeUpdateInMemoryState wraps an in-memory state update with WAL protection.
// If the update fails (panics), it sets the divergence flag and triggers reload.
// This should be called AFTER a successful database transaction.
func (e *Engine) safeUpdateInMemoryState(
	ctx context.Context,
	op WALOperationType,
	contestID, userID, symbol string,
	data interface{},
	updateFunc func() error,
) error {
	// If WAL is not initialized, just execute directly
	if e.wal == nil {
		return updateFunc()
	}

	// Write to WAL
	dataBytes, err := json.Marshal(data)
	if err != nil {
		// If we can't marshal, just execute without WAL tracking
		return updateFunc()
	}
	seqNum := e.wal.Write(op, contestID, userID, symbol, dataBytes)

	// Execute with panic recovery
	var updateErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				updateErr = fmt.Errorf("panic during in-memory state update: %v", r)
				e.logger.Error("Panic during in-memory state update after DB commit",
					zap.Any("panic", r),
					zap.String("contest_id", contestID),
					zap.String("user_id", userID),
					zap.String("symbol", symbol))

				// Set divergence flag
				e.wal.SetDiverged()

				// Trigger async reload
				if e.stateReloader != nil {
					infra.SafeGo(e.logger, "state-reload-after-divergence", func() {
						reloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if reloadErr := e.stateReloader.ReloadState(reloadCtx); reloadErr != nil {
							e.logger.Error("Failed to reload state after divergence", zap.Error(reloadErr))
						} else {
							e.wal.ClearDiverged()
							e.logger.Info("State successfully reloaded after divergence")
						}
					})
				}
			}
		}()

		updateErr = updateFunc()
	}()

	if updateErr != nil {
		e.logger.Error("In-memory state update failed after DB commit",
			zap.Error(updateErr),
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.String("symbol", symbol))

		// Set divergence flag
		e.wal.SetDiverged()

		// The DB transaction already succeeded, so mark WAL as committed
		// (the change exists in DB even though in-memory failed)
		e.wal.MarkCommitted(seqNum)

		// Trigger async reload
		if e.stateReloader != nil {
			infra.SafeGo(e.logger, "wal-state-reload", func() {
				reloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if reloadErr := e.stateReloader.ReloadState(reloadCtx); reloadErr != nil {
					e.logger.Error("Failed to reload state after divergence", zap.Error(reloadErr))
				} else {
					e.wal.ClearDiverged()
					e.logger.Info("State successfully reloaded after divergence")
				}
			})
		}

		return updateErr
	}

	// Success - mark WAL as committed
	e.wal.MarkCommitted(seqNum)
	return nil
}

// StartPositionLockCleanup starts the periodic cleanup of position locks for closed positions.
// This should be called after the engine is fully initialized.
func (e *Engine) StartPositionLockCleanup(ctx context.Context) {
	if e.positionLocks != nil {
		e.positionLocks.StartCleanup(ctx)
		e.logger.Info("Position lock cleanup started")
	}
}

// StopPositionLockCleanup stops the position lock cleanup goroutine.
// This should be called during shutdown.
func (e *Engine) StopPositionLockCleanup() {
	if e.positionLocks != nil {
		e.positionLocks.StopCleanup()
		e.logger.Info("Position lock cleanup stopped")
	}
}

// GetPositionLockCount returns the number of position locks currently held.
// This is useful for monitoring and debugging.
func (e *Engine) GetPositionLockCount() int {
	if e.positionLocks != nil {
		return e.positionLocks.GetLockCount()
	}
	return 0
}

// ProcessTick processes a tick snapshot and evaluates pending orders and TP/SL.
func (e *Engine) ProcessTick(ctx context.Context, tick *contracts.TickSnapshot) {
	// Update price book
	e.priceBook.UpdateFromTick(tick)

	// Process each symbol in the tick
	for _, st := range tick.Symbols {
		e.processSymbolTick(ctx, st.Symbol)
	}
}

// processSymbolTick evaluates pending orders and TP/SL for a single symbol.
func (e *Engine) processSymbolTick(ctx context.Context, symbol string) {
	// Get current bid/ask
	bid, ask, ok := e.priceBook.GetBidAskDirect(symbol)
	if !ok {
		return
	}

	// Evaluate pending orders
	triggeredOrders := e.pendingBook.EvaluatePendingOrders(symbol, bid, ask)
	for _, triggered := range triggeredOrders {
		e.executePendingOrder(ctx, triggered)
	}

	// Evaluate TP/SL
	triggeredTPSL := e.pendingBook.EvaluateTPSL(symbol, bid, ask)
	for _, triggered := range triggeredTPSL {
		e.executeTPSL(ctx, triggered)
	}
}

// BroadcastUnrealizedScores broadcasts unrealized PnL updates for all active positions.
func (e *Engine) BroadcastUnrealizedScores(ctx context.Context) {
	startTime := time.Now()

	// Get exit price function that returns appropriate price based on position side.
	// For LONG positions, use bid (you sell at bid to exit).
	// For SHORT positions, use ask (you buy at ask to exit).
	getExitPriceFunc := func(symbol string, positionSide contracts.OrderSide) (float64, bool) {
		if exitPrice, ok := e.priceBook.GetExitPrice(symbol, positionSide); ok {
			return exitPrice, true
		}
		// Fallback to Redis (uses real bid/ask when available)
		pd, _, err := e.getPriceDataFromRedis(ctx, symbol)
		if err != nil {
			return 0, false
		}
		return exitPriceFromData(pd, positionSide), true
	}

	// Legacy price function for backward compatibility (deprecated code paths)
	getPriceFunc := func(symbol string) (float64, bool) {
		if price, ok := e.priceBook.GetLast(symbol); ok {
			return price, true
		}
		price, err := e.getCurrentPrice(ctx, symbol)
		if err != nil {
			return 0, false
		}
		return price, true
	}

	// Broadcast function that handles a single contest
	broadcastForContest := func(contestID string, cs *ContestState) {
		cs.ForEachUser(func(userID string, userState *UserState) {
			// Skip users with no open positions
			if !userState.HasOpenPositions() {
				return
			}

			// Calculate unrealized score using positions and exit prices (bid for LONG, ask for SHORT)
			realizedScore := userState.GetRealizedScore()
			unrealizedScore := userState.CalculateUnrealizedScoreWithExitPrice(getExitPriceFunc)
			totalScore := realizedScore + unrealizedScore

			// Calculate decimal versions for high-precision fields
			realizedScoreDecimal := userState.GetRealizedScoreDecimal()
			unrealizedScoreDecimal := userState.CalculateUnrealizedScoreDecimalWithExitPrice(getExitPriceFunc)
			totalScoreDecimal := realizedScoreDecimal.Add(unrealizedScoreDecimal)

			// Keep legacy function available for potential fallback
			_ = getPriceFunc

			delta := &contracts.PnLDelta{
				UserID:          userID,
				ContestID:       contestID,
				DeltaScore:      0, // No realized change, just unrealized update
				RealizedScore:   realizedScore,
				UnrealizedScore: unrealizedScore,
				TotalScore:      totalScore,
				Ts:              time.Now().UnixMilli(),
				SeqNum:          e.pnlSeqNum.Add(1),
				// High-precision decimal string fields (8 decimal places)
				DeltaScoreDecimal:      "0.00000000",
				RealizedScoreDecimal:   realizedScoreDecimal.StringFixed(8),
				UnrealizedScoreDecimal: unrealizedScoreDecimal.StringFixed(8),
				TotalScoreDecimal:      totalScoreDecimal.StringFixed(8),
			}

			data, err := json.Marshal(delta)
			if err != nil {
				e.logger.Error("Failed to marshal PnLDelta for broadcast",
					zap.String("contest_id", contestID),
					zap.String("user_id", userID),
					zap.Error(err))
				return
			}

			record := &kgo.Record{
				Topic: e.config.PnLDeltasTopic,
				Key:   []byte(contestID),
				Value: data,
			}

			e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
				if err != nil {
					e.logger.Error("Failed to publish PnLDelta broadcast",
						zap.String("contest_id", contestID),
						zap.String("user_id", userID),
						zap.Error(err))
					if e.metrics != nil {
						e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
					}
				}
			})
		})
	}

	// Determine which mode we're in and iterate appropriately
	var mode string
	if e.shardingEnabled && e.shardedState != nil {
		// Sharded mode: iterate over contests assigned to this shard
		mode = "sharded"
		e.shardedState.ForEachContest(broadcastForContest)
	} else if sm, ok := e.state.(*StateManager); ok {
		// Local mode: use StateManager's ForEachContest
		mode = "local"
		sm.ForEachContest(broadcastForContest)
	} else {
		// Unknown state provider, skip
		return
	}

	// Record broadcast duration metric
	if e.metrics != nil && e.metrics.UnrealizedBroadcastDuration != nil {
		duration := time.Since(startTime).Seconds()
		e.metrics.UnrealizedBroadcastDuration.WithLabelValues(mode).Observe(duration)
	}
}
