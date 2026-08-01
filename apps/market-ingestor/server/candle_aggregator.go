package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"time"

	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Resolution represents a candle timeframe.
type Resolution string

const (
	Resolution1m  Resolution = "1m"
	Resolution5m  Resolution = "5m"
	Resolution15m Resolution = "15m"
	Resolution30m Resolution = "30m"
	Resolution1h  Resolution = "1h"
	Resolution4h  Resolution = "4h"
	Resolution1d  Resolution = "1d"
	Resolution1w  Resolution = "1w"
)

// AllResolutions lists all supported candle resolutions.
var AllResolutions = []Resolution{
	Resolution1m,
	Resolution5m,
	Resolution15m,
	Resolution30m,
	Resolution1h,
	Resolution4h,
	Resolution1d,
	Resolution1w,
}

// ResolutionDurations maps resolutions to their durations.
var ResolutionDurations = map[Resolution]time.Duration{
	Resolution1m:  1 * time.Minute,
	Resolution5m:  5 * time.Minute,
	Resolution15m: 15 * time.Minute,
	Resolution30m: 30 * time.Minute,
	Resolution1h:  1 * time.Hour,
	Resolution4h:  4 * time.Hour,
	Resolution1d:  24 * time.Hour,
	Resolution1w:  7 * 24 * time.Hour,
}

// Candle represents an in-progress or completed OHLCV candle.
type Candle struct {
	Symbol     string     `json:"symbol"`
	Resolution Resolution `json:"resolution"`
	Time       int64      `json:"time"`       // Unix timestamp (start of candle window)
	Open       float64    `json:"open"`       // First tick price
	High       float64    `json:"high"`       // Maximum price
	Low        float64    `json:"low"`        // Minimum price
	Close      float64    `json:"close"`      // Last tick price
	Volume     float64    `json:"volume"`     // Aggregated volume from provider (or tick count fallback)
	TickCount  int        `json:"tick_count"` // Number of ticks received
	LastUpdate int64      `json:"last_update"`// Last tick timestamp
}

// CandleAggregatorConfig holds configuration for the candle aggregator.
type CandleAggregatorConfig struct {
	// FlushInterval is how often to check for completed candles and flush to DB.
	FlushInterval time.Duration
	// BatchSize is the maximum number of candles to insert in one batch.
	BatchSize int
	// EnableHigherTimeframes enables aggregation of 5m, 15m, 30m, 1h, 1d from 1m candles.
	EnableHigherTimeframes bool
	// CheckpointInterval is how often to checkpoint active candles to Redis for crash recovery.
	CheckpointInterval time.Duration
}

// DefaultCandleAggregatorConfig returns default configuration.
func DefaultCandleAggregatorConfig() CandleAggregatorConfig {
	return CandleAggregatorConfig{
		FlushInterval:          5 * time.Second, // Check every 5 seconds
		BatchSize:              100,
		EnableHigherTimeframes: true,
		CheckpointInterval:     10 * time.Second,
	}
}

const (
	checkpointKeyPrefix   = "candle:active:"
	checkpointScanPattern = "candle:active:*"
)

// checkpointKey returns the Redis key for a candle checkpoint.
func checkpointKey(symbol string, resolution Resolution) string {
	return fmt.Sprintf("%s%s:%s", checkpointKeyPrefix, symbol, string(resolution))
}

// checkpointTTL returns the TTL for a candle checkpoint.
func checkpointTTL(resolution Resolution) time.Duration {
	d := ResolutionDurations[resolution]
	ttl := d * 2
	if ttl < 5*time.Minute {
		ttl = 5 * time.Minute
	}
	return ttl
}

// CandleAggregator aggregates tick data into OHLCV candles.
//
// Lock ordering convention (never acquire in reverse order):
//
//	candlesMu -> (release) -> pendingFlushMu
//	candlesMu -> (release) -> recentCandlesMu
//
// All methods must release candlesMu BEFORE acquiring pendingFlushMu or recentCandlesMu.
type CandleAggregator struct {
	config CandleAggregatorConfig
	db     *sql.DB
	redis  *pkgredis.Client // Redis client for checkpoint persistence (may be nil)
	logger *zap.Logger

	// In-progress candles: map[symbol]map[resolution]*Candle
	candles   map[string]map[Resolution]*Candle
	candlesMu sync.RWMutex

	// Completed 1-minute candles for higher timeframe aggregation
	// map[symbol][]Candle - stores recent 1m candles for aggregation
	recentCandles   map[string][]*Candle
	recentCandlesMu sync.RWMutex

	// Pending completed candles to be flushed to DB
	pendingFlush   []*Candle
	pendingFlushMu sync.Mutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	candlesGenerated  *prometheus.CounterVec
	ticksProcessed    prometheus.Counter
	flushDuration     prometheus.Histogram
	flushErrors       prometheus.Counter
	activeCandleGauge prometheus.Gauge
}

// NewCandleAggregator creates a new candle aggregator.
// redisClient may be nil; checkpoint/recovery will be disabled if so.
func NewCandleAggregator(db *sql.DB, redisClient *pkgredis.Client, logger *zap.Logger, config CandleAggregatorConfig) *CandleAggregator {
	ctx, cancel := context.WithCancel(context.Background())

	ca := &CandleAggregator{
		config:        config,
		db:            db,
		redis:         redisClient,
		logger:        logger.With(zap.String("component", "candle_aggregator")),
		candles:       make(map[string]map[Resolution]*Candle),
		recentCandles: make(map[string][]*Candle),
		pendingFlush:  make([]*Candle, 0, config.BatchSize),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize Prometheus metrics
	ca.candlesGenerated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "market_ingestor",
			Subsystem: "candles",
			Name:      "generated_total",
			Help:      "Total number of candles generated by resolution",
		},
		[]string{"resolution"},
	)

	ca.ticksProcessed = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "market_ingestor",
			Subsystem: "candles",
			Name:      "ticks_processed_total",
			Help:      "Total number of ticks processed for candle aggregation",
		},
	)

	ca.flushDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "market_ingestor",
			Subsystem: "candles",
			Name:      "flush_duration_seconds",
			Help:      "Time taken to flush candles to database",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
	)

	ca.flushErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "market_ingestor",
			Subsystem: "candles",
			Name:      "flush_errors_total",
			Help:      "Total number of candle flush errors",
		},
	)

	ca.activeCandleGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "market_ingestor",
			Subsystem: "candles",
			Name:      "active_candles",
			Help:      "Number of active (in-progress) candles",
		},
	)

	return ca
}

// Start begins the candle aggregation background processes.
func (ca *CandleAggregator) Start() {
	ca.logger.Info("Starting candle aggregator",
		zap.Duration("flush_interval", ca.config.FlushInterval),
		zap.Int("batch_size", ca.config.BatchSize))

	// Restore any previously checkpointed candles from Redis
	ca.restoreFromCheckpoint()

	// Start the flush worker
	ca.wg.Add(1)
	go ca.flushWorker()

	// Start the minute boundary checker
	ca.wg.Add(1)
	go ca.minuteBoundaryChecker()

	// Start the checkpoint worker for crash recovery
	ca.wg.Add(1)
	go ca.checkpointWorker()
}

// Stop stops the candle aggregator gracefully.
func (ca *CandleAggregator) Stop() {
	ca.logger.Info("Stopping candle aggregator")
	ca.cancel()
	ca.wg.Wait()

	// Final flush of any remaining candles
	ca.flushAll()
	ca.logger.Info("Candle aggregator stopped")
}

// ProcessTick processes a tick and updates candles.
// This method is thread-safe and can be called from multiple goroutines.
func (ca *CandleAggregator) ProcessTick(symbol string, price float64, timestamp int64, volume float64) {
	if price <= 0 {
		return
	}

	ca.ticksProcessed.Inc()

	// Process tick for 1-minute candles
	ca.updateCandle(symbol, Resolution1m, price, timestamp, volume)

	// For higher timeframes, we aggregate from 1-minute candles
	// but we also track them in real-time for immediate updates
	if ca.config.EnableHigherTimeframes {
		for _, res := range []Resolution{Resolution5m, Resolution15m, Resolution30m, Resolution1h, Resolution4h, Resolution1d, Resolution1w} {
			ca.updateCandle(symbol, res, price, timestamp, volume)
		}
	}
}

// updateCandle updates or creates a candle for the given symbol and resolution.
func (ca *CandleAggregator) updateCandle(symbol string, resolution Resolution, price float64, timestamp int64, volume float64) {
	candleTime := ca.getCandleStartTime(timestamp, resolution)

	var completedCandle *Candle
	var storeAs1m bool

	ca.candlesMu.Lock()

	// Initialize symbol map if needed
	if ca.candles[symbol] == nil {
		ca.candles[symbol] = make(map[Resolution]*Candle)
	}

	candle := ca.candles[symbol][resolution]

	// Check if we need to complete the current candle and start a new one
	if candle != nil && candle.Time != candleTime {
		completedCandle = candle
		storeAs1m = (resolution == Resolution1m)
		candle = nil
	}

	// Create new candle if needed
	if candle == nil {
		candle = &Candle{
			Symbol:     symbol,
			Resolution: resolution,
			Time:       candleTime,
			Open:       price,
			High:       price,
			Low:        price,
			Close:      price,
			Volume:     volume,
			TickCount:  1,
			LastUpdate: timestamp,
		}
		ca.candles[symbol][resolution] = candle
		ca.candlesMu.Unlock()

		// Queue completed candle outside the lock
		if completedCandle != nil {
			ca.queueForFlush(completedCandle)
			if storeAs1m {
				ca.storeRecentCandle(symbol, completedCandle)
			}
		}
		return
	}

	// Update existing candle
	if price > candle.High {
		candle.High = price
	}
	if price < candle.Low {
		candle.Low = price
	}
	candle.Close = price
	candle.Volume += volume
	candle.TickCount++
	candle.LastUpdate = timestamp

	ca.candlesMu.Unlock()

	// Queue completed candle outside the lock
	if completedCandle != nil {
		ca.queueForFlush(completedCandle)
		if storeAs1m {
			ca.storeRecentCandle(symbol, completedCandle)
		}
	}
}

// getCandleStartTime returns the start timestamp for a candle given tick timestamp and resolution.
func (ca *CandleAggregator) getCandleStartTime(timestamp int64, resolution Resolution) int64 {
	t := time.Unix(timestamp, 0).UTC()

	switch resolution {
	case Resolution1m:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC).Unix()
	case Resolution5m:
		minute := (t.Minute() / 5) * 5
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, time.UTC).Unix()
	case Resolution15m:
		minute := (t.Minute() / 15) * 15
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, time.UTC).Unix()
	case Resolution30m:
		minute := (t.Minute() / 30) * 30
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, time.UTC).Unix()
	case Resolution1h:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC).Unix()
	case Resolution4h:
		hour := (t.Hour() / 4) * 4
		return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, time.UTC).Unix()
	case Resolution1d:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
	case Resolution1w:
		daysSinceMonday := (int(t.Weekday()) + 6) % 7
		monday := t.AddDate(0, 0, -daysSinceMonday)
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC).Unix()
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC).Unix()
	}
}

// queueForFlush adds a completed candle to the flush queue.
func (ca *CandleAggregator) queueForFlush(candle *Candle) {
	ca.pendingFlushMu.Lock()
	defer ca.pendingFlushMu.Unlock()

	// Create a copy to avoid race conditions
	candleCopy := &Candle{
		Symbol:     candle.Symbol,
		Resolution: candle.Resolution,
		Time:       candle.Time,
		Open:       candle.Open,
		High:       candle.High,
		Low:        candle.Low,
		Close:      candle.Close,
		Volume:     candle.Volume,
		TickCount:  candle.TickCount,
		LastUpdate: candle.LastUpdate,
	}
	ca.pendingFlush = append(ca.pendingFlush, candleCopy)
	ca.candlesGenerated.WithLabelValues(string(candle.Resolution)).Inc()
}

// storeRecentCandle stores a completed 1m candle for higher timeframe aggregation.
func (ca *CandleAggregator) storeRecentCandle(symbol string, candle *Candle) {
	ca.recentCandlesMu.Lock()
	defer ca.recentCandlesMu.Unlock()

	candleCopy := &Candle{
		Symbol:     candle.Symbol,
		Resolution: candle.Resolution,
		Time:       candle.Time,
		Open:       candle.Open,
		High:       candle.High,
		Low:        candle.Low,
		Close:      candle.Close,
		Volume:     candle.Volume,
		TickCount:  candle.TickCount,
		LastUpdate: candle.LastUpdate,
	}

	ca.recentCandles[symbol] = append(ca.recentCandles[symbol], candleCopy)

	// Keep only the last 1440 candles (24 hours of 1m candles)
	if len(ca.recentCandles[symbol]) > 1440 {
		ca.recentCandles[symbol] = ca.recentCandles[symbol][len(ca.recentCandles[symbol])-1440:]
	}
}

// flushWorker periodically flushes completed candles to the database.
func (ca *CandleAggregator) flushWorker() {
	defer ca.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			ca.logger.Error("CandleAggregator.flushWorker panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(ca.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ca.ctx.Done():
			return
		case <-ticker.C:
			ca.flushPending()
		}
	}
}

// minuteBoundaryChecker checks for completed candles at minute boundaries.
func (ca *CandleAggregator) minuteBoundaryChecker() {
	defer ca.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			ca.logger.Error("CandleAggregator.minuteBoundaryChecker panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	// Wait until the start of the next minute, respecting cancellation.
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	select {
	case <-time.After(time.Until(nextMinute)):
	case <-ca.ctx.Done():
		return
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ca.ctx.Done():
			return
		case <-ticker.C:
			ca.checkCompletedCandles()
		}
	}
}

// checkpointWorker periodically saves active candles to Redis for crash recovery.
func (ca *CandleAggregator) checkpointWorker() {
	defer ca.wg.Done()

	if ca.redis == nil {
		ca.logger.Warn("Redis not available, candle checkpointing disabled")
		return
	}

	interval := ca.config.CheckpointInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ca.ctx.Done():
			return
		case <-ticker.C:
			ca.checkpointActiveCandles()
		}
	}
}

// checkpointActiveCandles saves all active candles to Redis.
func (ca *CandleAggregator) checkpointActiveCandles() {
	type entry struct {
		key    string
		candle *Candle
		ttl    time.Duration
	}

	ca.candlesMu.RLock()
	entries := make([]entry, 0)
	for _, resolutions := range ca.candles {
		for _, candle := range resolutions {
			if candle != nil && candle.TickCount > 0 {
				entries = append(entries, entry{
					key:    checkpointKey(candle.Symbol, candle.Resolution),
					candle: candle,
					ttl:    checkpointTTL(candle.Resolution),
				})
			}
		}
	}
	ca.candlesMu.RUnlock()

	if len(entries) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ca.ctx, 5*time.Second)
	defer cancel()

	pipe := ca.redis.Pipeline()
	for _, e := range entries {
		data, err := json.Marshal(e.candle)
		if err != nil {
			ca.logger.Error("Failed to marshal candle for checkpoint",
				zap.String("key", e.key), zap.Error(err))
			continue
		}
		pipe.Set(ctx, e.key, data, e.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		ca.logger.Warn("Failed to checkpoint candles to Redis", zap.Error(err))
	} else {
		ca.logger.Debug("Checkpointed active candles", zap.Int("count", len(entries)))
	}
}

// restoreFromCheckpoint loads previously checkpointed candles from Redis on startup.
func (ca *CandleAggregator) restoreFromCheckpoint() {
	if ca.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ca.ctx, 10*time.Second)
	defer cancel()

	var cursor uint64
	var restored int

	for {
		keys, nextCursor, err := ca.redis.Scan(ctx, cursor, checkpointScanPattern, 100).Result()
		if err != nil {
			ca.logger.Warn("Failed to scan Redis for candle checkpoints", zap.Error(err))
			return
		}

		for _, key := range keys {
			data, err := ca.redis.Get(ctx, key).Bytes()
			if err != nil {
				ca.logger.Warn("Failed to read candle checkpoint",
					zap.String("key", key), zap.Error(err))
				continue
			}

			var candle Candle
			if err := json.Unmarshal(data, &candle); err != nil {
				ca.logger.Warn("Failed to unmarshal candle checkpoint",
					zap.String("key", key), zap.Error(err))
				continue
			}

			// Only restore if the candle period has not yet ended
			duration := ResolutionDurations[candle.Resolution]
			candleEnd := candle.Time + int64(duration.Seconds())
			if time.Now().Unix() < candleEnd {
				ca.candlesMu.Lock()
				if ca.candles[candle.Symbol] == nil {
					ca.candles[candle.Symbol] = make(map[Resolution]*Candle)
				}
				ca.candles[candle.Symbol][candle.Resolution] = &candle
				ca.candlesMu.Unlock()
				restored++
			}

			// Delete the checkpoint key after processing
			ca.redis.Del(ctx, key)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if restored > 0 {
		ca.logger.Info("Restored candles from checkpoint", zap.Int("count", restored))
	}
}

// checkCompletedCandles checks all in-progress candles and queues completed ones.
func (ca *CandleAggregator) checkCompletedCandles() {
	now := time.Now().Unix()

	type completedEntry struct {
		candle *Candle
		symbol string
		is1m   bool
	}

	var completed []completedEntry

	ca.candlesMu.Lock()

	activeCount := 0
	for symbol, resolutions := range ca.candles {
		for resolution, candle := range resolutions {
			if candle == nil {
				continue
			}

			// Check if the candle period has ended
			duration := ResolutionDurations[resolution]
			candleEnd := candle.Time + int64(duration.Seconds())

			if now >= candleEnd {
				completed = append(completed, completedEntry{
					candle: candle,
					symbol: symbol,
					is1m:   resolution == Resolution1m,
				})
				delete(resolutions, resolution)
			} else {
				activeCount++
			}
		}
	}

	ca.activeCandleGauge.Set(float64(activeCount))
	ca.candlesMu.Unlock()

	// Queue completed candles outside the lock
	for _, entry := range completed {
		ca.queueForFlush(entry.candle)
		if entry.is1m {
			ca.storeRecentCandle(entry.symbol, entry.candle)
		}
	}
}

// flushPending flushes pending candles to the database.
func (ca *CandleAggregator) flushPending() {
	ca.pendingFlushMu.Lock()
	if len(ca.pendingFlush) == 0 {
		ca.pendingFlushMu.Unlock()
		return
	}

	// Take all pending candles
	candles := ca.pendingFlush
	ca.pendingFlush = make([]*Candle, 0, ca.config.BatchSize)
	ca.pendingFlushMu.Unlock()

	// Flush in batches
	for i := 0; i < len(candles); i += ca.config.BatchSize {
		end := i + ca.config.BatchSize
		if end > len(candles) {
			end = len(candles)
		}
		batch := candles[i:end]
		ca.flushBatch(batch)
	}
}

// flushBatch inserts a batch of candles to the database.
func (ca *CandleAggregator) flushBatch(candles []*Candle) {
	if len(candles) == 0 || ca.db == nil {
		return
	}

	start := time.Now()
	defer func() {
		ca.flushDuration.Observe(time.Since(start).Seconds())
	}()

	ctx, cancel := context.WithTimeout(ca.ctx, 10*time.Second)
	defer cancel()

	// Build batch insert query using ON CONFLICT DO UPDATE (upsert)
	// This handles the case where we might have duplicate candles
	query := `
		INSERT INTO candles (symbol, resolution, time, open, high, low, close, volume)
		VALUES `

	values := make([]interface{}, 0, len(candles)*8)
	placeholders := make([]string, 0, len(candles))

	for i, c := range candles {
		offset := i * 8
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8))
		values = append(values, c.Symbol, string(c.Resolution), c.Time, c.Open, c.High, c.Low, c.Close, c.Volume)
	}

	query += joinStrings(placeholders, ", ")
	query += `
		ON CONFLICT (symbol, resolution, time) DO UPDATE SET
			open = candles.open,
			high = GREATEST(candles.high, EXCLUDED.high),
			low = LEAST(candles.low, EXCLUDED.low),
			close = EXCLUDED.close,
			volume = candles.volume + EXCLUDED.volume`

	// P2-5: Retry up to 3 times with exponential backoff
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			retryDelay := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s
			select {
			case <-time.After(retryDelay):
			case <-ca.ctx.Done():
				return
			}
			ca.logger.Warn("Retrying candle flush",
				zap.Int("attempt", attempt+1),
				zap.Int("batch_size", len(candles)))
		}

		_, lastErr = ca.db.ExecContext(ctx, query, values...)
		if lastErr == nil {
			ca.logger.Debug("Flushed candles to database",
				zap.Int("count", len(candles)),
				zap.Duration("duration", time.Since(start)))

			// Clean up checkpoint keys for flushed candles
			if ca.redis != nil {
				delCtx, delCancel := context.WithTimeout(ca.ctx, 3*time.Second)
				keys := make([]string, 0, len(candles))
				for _, c := range candles {
					keys = append(keys, checkpointKey(c.Symbol, c.Resolution))
				}
				ca.redis.Del(delCtx, keys...)
				delCancel()
			}
			return
		}
	}

	ca.flushErrors.Inc()
	ca.logger.Error("Failed to flush candles after 3 attempts",
		zap.Error(lastErr),
		zap.Int("batch_size", len(candles)))
}

// flushAll flushes all in-progress and pending candles to the database.
func (ca *CandleAggregator) flushAll() {
	ca.logger.Info("Flushing all candles")

	// Collect all in-progress candles while holding candlesMu
	var toFlush []*Candle

	ca.candlesMu.Lock()
	for symbol, resolutions := range ca.candles {
		for resolution, candle := range resolutions {
			if candle != nil && candle.TickCount > 0 {
				toFlush = append(toFlush, candle)
			}
			delete(resolutions, resolution)
		}
		delete(ca.candles, symbol)
	}
	ca.candlesMu.Unlock()

	// Queue collected candles outside the lock
	for _, candle := range toFlush {
		ca.queueForFlush(candle)
	}

	// Then flush all pending
	ca.flushPending()
}

// AggregateHigherTimeframe aggregates 1m candles into a higher timeframe candle.
// This can be used to rebuild higher timeframe candles from stored 1m data.
func (ca *CandleAggregator) AggregateHigherTimeframe(symbol string, resolution Resolution, startTime int64) *Candle {
	if resolution == Resolution1m {
		return nil // 1m is the base resolution
	}

	ca.recentCandlesMu.RLock()
	defer ca.recentCandlesMu.RUnlock()

	candles1m := ca.recentCandles[symbol]
	if len(candles1m) == 0 {
		return nil
	}

	duration := ResolutionDurations[resolution]
	endTime := startTime + int64(duration.Seconds())

	var result *Candle
	for _, c := range candles1m {
		if c.Time >= startTime && c.Time < endTime {
			if result == nil {
				result = &Candle{
					Symbol:     symbol,
					Resolution: resolution,
					Time:       startTime,
					Open:       c.Open,
					High:       c.High,
					Low:        c.Low,
					Close:      c.Close,
					Volume:     c.Volume,
					TickCount:  c.TickCount,
					LastUpdate: c.LastUpdate,
				}
			} else {
				if c.High > result.High {
					result.High = c.High
				}
				if c.Low < result.Low {
					result.Low = c.Low
				}
				result.Close = c.Close
				result.Volume += c.Volume
				result.TickCount += c.TickCount
				result.LastUpdate = c.LastUpdate
			}
		}
	}

	return result
}

// GetActiveCandles returns a snapshot of all active candles for monitoring.
func (ca *CandleAggregator) GetActiveCandles() map[string]map[Resolution]*Candle {
	ca.candlesMu.RLock()
	defer ca.candlesMu.RUnlock()

	result := make(map[string]map[Resolution]*Candle)
	for symbol, resolutions := range ca.candles {
		result[symbol] = make(map[Resolution]*Candle)
		for resolution, candle := range resolutions {
			if candle != nil {
				// Create a copy
				result[symbol][resolution] = &Candle{
					Symbol:     candle.Symbol,
					Resolution: candle.Resolution,
					Time:       candle.Time,
					Open:       candle.Open,
					High:       candle.High,
					Low:        candle.Low,
					Close:      candle.Close,
					Volume:     candle.Volume,
					TickCount:  candle.TickCount,
					LastUpdate: candle.LastUpdate,
				}
			}
		}
	}
	return result
}

// GetPendingFlushCount returns the number of candles waiting to be flushed.
func (ca *CandleAggregator) GetPendingFlushCount() int {
	ca.pendingFlushMu.Lock()
	defer ca.pendingFlushMu.Unlock()
	return len(ca.pendingFlush)
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	n := len(sep) * (len(strs) - 1)
	for _, s := range strs {
		n += len(s)
	}

	result := make([]byte, n)
	pos := copy(result, strs[0])
	for _, s := range strs[1:] {
		pos += copy(result[pos:], sep)
		pos += copy(result[pos:], s)
	}
	return string(result)
}

// roundFloat rounds a float to the specified number of decimal places.
func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
