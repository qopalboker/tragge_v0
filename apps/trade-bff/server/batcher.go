package server

import (
	"sync"
	"sync/atomic"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// MessageBatcher batches multiple messages into single payloads
// This reduces the number of WebSocket messages sent, improving performance
// for high-concurrency scenarios (1000+ users)
type MessageBatcher struct {
	sequence atomic.Uint64

	// Tick batching - accumulates ticks until flush
	tickBuffer map[string]contracts.SymbolTick
	tickMu     sync.RWMutex

	// Configuration
	maxBatchSize  int
	flushInterval time.Duration

	// Metrics
	metrics *BatcherMetrics
}

// BatcherMetrics tracks batching statistics
type BatcherMetrics struct {
	TickBatchesSent    atomic.Int64
	TotalTicksBuffered atomic.Int64
	TotalTicksFlushed  atomic.Int64
	AvgTicksPerBatch   atomic.Int64
	LastBatchSize      atomic.Int64
	LastBatchTimestamp atomic.Int64
}

// BatchedMessage wraps batched data with metadata for client processing
type BatchedMessage struct {
	Type     string      `json:"type"`
	Sequence uint64      `json:"seq"`  // Sequence number for gap detection
	Count    int         `json:"n"`    // Number of items in batch
	Data     interface{} `json:"data"` // The batched data
	Ts       int64       `json:"ts"`   // Timestamp of batch creation
}

// TickBatchData is the data structure for batched ticks
type TickBatchData struct {
	Symbols []contracts.SymbolTick `json:"symbols"`
}

// NewMessageBatcher creates a new MessageBatcher
func NewMessageBatcher(maxBatchSize int, flushInterval time.Duration) *MessageBatcher {
	return &MessageBatcher{
		tickBuffer:    make(map[string]contracts.SymbolTick),
		maxBatchSize:  maxBatchSize,
		flushInterval: flushInterval,
		metrics:       &BatcherMetrics{},
	}
}

// AddTick adds a tick to the batch buffer
// This is called when ticks are received from Kafka
func (b *MessageBatcher) AddTick(tick contracts.SymbolTick) {
	b.tickMu.Lock()
	b.tickBuffer[tick.Symbol] = tick
	b.tickMu.Unlock()
	b.metrics.TotalTicksBuffered.Add(1)
}

// AddTickSnapshot adds multiple ticks from a snapshot to the buffer
func (b *MessageBatcher) AddTickSnapshot(snapshot *contracts.TickSnapshot) {
	if snapshot == nil {
		return
	}
	b.tickMu.Lock()
	for _, tick := range snapshot.Symbols {
		b.tickBuffer[tick.Symbol] = tick
	}
	b.tickMu.Unlock()
	b.metrics.TotalTicksBuffered.Add(int64(len(snapshot.Symbols)))
}

// FlushTicks returns batched ticks and clears buffer
// Returns nil if buffer is empty
func (b *MessageBatcher) FlushTicks() *BatchedMessage {
	b.tickMu.Lock()
	defer b.tickMu.Unlock()

	if len(b.tickBuffer) == 0 {
		return nil
	}

	ticks := make([]contracts.SymbolTick, 0, len(b.tickBuffer))
	for _, tick := range b.tickBuffer {
		ticks = append(ticks, tick)
	}

	// Clear buffer
	b.tickBuffer = make(map[string]contracts.SymbolTick)

	// Update metrics
	b.metrics.TickBatchesSent.Add(1)
	b.metrics.TotalTicksFlushed.Add(int64(len(ticks)))
	b.metrics.LastBatchSize.Store(int64(len(ticks)))
	b.metrics.LastBatchTimestamp.Store(time.Now().UnixMilli())

	// Calculate running average
	totalBatches := b.metrics.TickBatchesSent.Load()
	totalFlushed := b.metrics.TotalTicksFlushed.Load()
	if totalBatches > 0 {
		b.metrics.AvgTicksPerBatch.Store(totalFlushed / totalBatches)
	}

	return &BatchedMessage{
		Type:     "tick_batch",
		Sequence: b.sequence.Add(1),
		Count:    len(ticks),
		Data:     TickBatchData{Symbols: ticks},
		Ts:       time.Now().UnixMilli(),
	}
}

// GetPendingTickCount returns the number of ticks waiting in the buffer
func (b *MessageBatcher) GetPendingTickCount() int {
	b.tickMu.RLock()
	defer b.tickMu.RUnlock()
	return len(b.tickBuffer)
}

// GetSequence returns current sequence number (for debugging/monitoring)
func (b *MessageBatcher) GetSequence() uint64 {
	return b.sequence.Load()
}

// GetMetrics returns current batcher metrics
func (b *MessageBatcher) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"tick_batches_sent":    b.metrics.TickBatchesSent.Load(),
		"total_ticks_buffered": b.metrics.TotalTicksBuffered.Load(),
		"total_ticks_flushed":  b.metrics.TotalTicksFlushed.Load(),
		"avg_ticks_per_batch":  b.metrics.AvgTicksPerBatch.Load(),
		"last_batch_size":      b.metrics.LastBatchSize.Load(),
		"last_batch_timestamp": b.metrics.LastBatchTimestamp.Load(),
		"current_sequence":     b.sequence.Load(),
		"pending_ticks":        b.GetPendingTickCount(),
	}
}

// Reset clears the batcher state (useful for testing)
func (b *MessageBatcher) Reset() {
	b.tickMu.Lock()
	b.tickBuffer = make(map[string]contracts.SymbolTick)
	b.tickMu.Unlock()
	b.sequence.Store(0)
}

