package server

import (
	"sync"
	"time"
)

// ====================
// Candle Aggregator
// ====================

// Candle represents an OHLCV candle for a specific time window
type Candle struct {
	Time   int64   `json:"time"`   // Unix seconds (start of window)
	Open   float64 `json:"open"`   // Opening price
	High   float64 `json:"high"`   // Highest price
	Low    float64 `json:"low"`    // Lowest price
	Close  float64 `json:"close"`  // Closing price
	Volume float64 `json:"volume"` // Aggregated volume from provider (or tick count fallback)
}

// CandleKey identifies a candle by symbol and resolution
type CandleKey struct {
	Symbol     string
	Resolution string // "1m", "5m", "15m", "1h", "1d"
}

// CandleAggregator aggregates ticks into OHLC candles in memory.
// Completed candles are discarded since market-ingestor persists them to PostgreSQL.
type CandleAggregator struct {
	mu           sync.RWMutex
	candles      map[CandleKey]*Candle // Current in-progress candles
	resolutions  []string
	resolutionMs map[string]int64
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewCandleAggregator creates a new candle aggregator
func NewCandleAggregator() *CandleAggregator {
	return &CandleAggregator{
		candles:     make(map[CandleKey]*Candle),
		resolutions: []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w"},
		resolutionMs: map[string]int64{
			"1m":  60_000,
			"5m":  300_000,
			"15m": 900_000,
			"30m": 1_800_000,
			"1h":  3_600_000,
			"4h":  14_400_000,
			"1d":  86_400_000,
			"1w":  604_800_000,
		},
		stopCh: make(chan struct{}),
	}
}

// Stop gracefully shuts down the candle aggregator
func (ca *CandleAggregator) Stop() {
	ca.stopOnce.Do(func() { close(ca.stopCh) })
}

// getCandleTime returns the start time of the candle window for a given timestamp
func (ca *CandleAggregator) getCandleTime(tsMs int64, resolution string) int64 {
	if resolution == "1w" {
		// Weekly candles align to Monday 00:00 UTC, matching market-ingestor's algorithm.
		t := time.Unix(tsMs/1000, 0).UTC()
		daysSinceMonday := (int(t.Weekday()) + 6) % 7
		monday := t.AddDate(0, 0, -daysSinceMonday)
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC).Unix()
	}
	resMs := ca.resolutionMs[resolution]
	if resMs == 0 {
		resMs = 60 * 1000 // Default to 1m
	}
	// Floor to resolution boundary (in seconds for TradingView compatibility)
	return (tsMs / resMs) * resMs / 1000
}

// ProcessTick processes a tick and updates all resolution candles.
// When a candle completes (time boundary crossed), it is replaced with a new one.
// Completed candles are not persisted here — market-ingestor writes them to PostgreSQL.
// volume is the real trade volume from the tick; pass 1.0 as fallback if unavailable.
func (ca *CandleAggregator) ProcessTick(symbol string, price float64, tsMs int64, volume float64) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	for _, resolution := range ca.resolutions {
		key := CandleKey{Symbol: symbol, Resolution: resolution}
		candleTime := ca.getCandleTime(tsMs, resolution)

		candle, exists := ca.candles[key]
		if !exists || candle.Time != candleTime {
			// Start new candle (completed candle is discarded)
			ca.candles[key] = &Candle{
				Time:   candleTime,
				Open:   price,
				High:   price,
				Low:    price,
				Close:  price,
				Volume: volume,
			}
		} else {
			// Update existing candle
			if price > candle.High {
				candle.High = price
			}
			if price < candle.Low {
				candle.Low = price
			}
			candle.Close = price
			candle.Volume += volume
		}
	}
}

// GetInProgressCandle returns a copy of the current in-progress candle for the
// given symbol and resolution, or nil if no candle is being aggregated.
func (ca *CandleAggregator) GetInProgressCandle(symbol, resolution string) *Candle {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	c, ok := ca.candles[CandleKey{Symbol: symbol, Resolution: resolution}]
	if !ok || c == nil {
		return nil
	}
	copy := *c
	return &copy
}
