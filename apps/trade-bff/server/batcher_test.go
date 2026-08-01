package server

import (
	"encoding/json"
	"testing"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

func TestNewMessageBatcher(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	if batcher == nil {
		t.Fatal("NewMessageBatcher returned nil")
	}
	if batcher.tickBuffer == nil {
		t.Error("tickBuffer not initialized")
	}
	if batcher.maxBatchSize != 30 {
		t.Errorf("Expected maxBatchSize 30, got %d", batcher.maxBatchSize)
	}
	if batcher.flushInterval != time.Second {
		t.Errorf("Expected flushInterval 1s, got %v", batcher.flushInterval)
	}
}

func TestAddTick(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	tick := contracts.SymbolTick{
		Symbol: "AAPL",
		Bid:    149.50,
		Ask:    150.00,
		Last:   149.75,
	}

	batcher.AddTick(tick)

	if batcher.GetPendingTickCount() != 1 {
		t.Errorf("Expected 1 pending tick, got %d", batcher.GetPendingTickCount())
	}

	// Add same symbol - should overwrite
	tick2 := contracts.SymbolTick{
		Symbol: "AAPL",
		Bid:    149.60,
		Ask:    150.10,
		Last:   149.85,
	}
	batcher.AddTick(tick2)

	if batcher.GetPendingTickCount() != 1 {
		t.Errorf("Expected 1 pending tick after overwrite, got %d", batcher.GetPendingTickCount())
	}

	// Add different symbol
	tick3 := contracts.SymbolTick{
		Symbol: "GOOG",
		Bid:    2800.00,
		Ask:    2805.00,
		Last:   2802.50,
	}
	batcher.AddTick(tick3)

	if batcher.GetPendingTickCount() != 2 {
		t.Errorf("Expected 2 pending ticks, got %d", batcher.GetPendingTickCount())
	}
}

func TestAddTickSnapshot(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	snapshot := &contracts.TickSnapshot{
		Ts: time.Now().UnixMilli(),
		Symbols: []contracts.SymbolTick{
			{Symbol: "AAPL", Bid: 149.50, Ask: 150.00, Last: 149.75},
			{Symbol: "GOOG", Bid: 2800.00, Ask: 2805.00, Last: 2802.50},
			{Symbol: "MSFT", Bid: 350.00, Ask: 350.50, Last: 350.25},
		},
	}

	batcher.AddTickSnapshot(snapshot)

	if batcher.GetPendingTickCount() != 3 {
		t.Errorf("Expected 3 pending ticks, got %d", batcher.GetPendingTickCount())
	}
}

func TestFlushTicks(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	// Add some ticks
	ticks := []contracts.SymbolTick{
		{Symbol: "AAPL", Bid: 149.50, Ask: 150.00, Last: 149.75},
		{Symbol: "GOOG", Bid: 2800.00, Ask: 2805.00, Last: 2802.50},
	}

	for _, tick := range ticks {
		batcher.AddTick(tick)
	}

	batch := batcher.FlushTicks()

	if batch == nil {
		t.Fatal("FlushTicks returned nil")
	}
	if batch.Type != "tick_batch" {
		t.Errorf("Expected type 'tick_batch', got '%s'", batch.Type)
	}
	if batch.Count != 2 {
		t.Errorf("Expected count 2, got %d", batch.Count)
	}
	if batch.Sequence != 1 {
		t.Errorf("Expected sequence 1, got %d", batch.Sequence)
	}
	if batch.Ts == 0 {
		t.Error("Timestamp should not be 0")
	}

	// Verify data
	data, ok := batch.Data.(TickBatchData)
	if !ok {
		t.Fatal("Data is not TickBatchData")
	}
	if len(data.Symbols) != 2 {
		t.Errorf("Expected 2 symbols in data, got %d", len(data.Symbols))
	}

	// Buffer should be empty after flush
	if batcher.GetPendingTickCount() != 0 {
		t.Errorf("Expected 0 pending ticks after flush, got %d", batcher.GetPendingTickCount())
	}
}

func TestFlushTicks_Empty(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	batch := batcher.FlushTicks()

	if batch != nil {
		t.Error("Expected nil batch when buffer is empty")
	}
}

func TestFlushTicks_SequenceIncrement(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	// First flush
	batcher.AddTick(contracts.SymbolTick{Symbol: "AAPL", Bid: 149.50, Ask: 150.00, Last: 149.75})
	batch1 := batcher.FlushTicks()

	if batch1.Sequence != 1 {
		t.Errorf("Expected sequence 1, got %d", batch1.Sequence)
	}

	// Second flush
	batcher.AddTick(contracts.SymbolTick{Symbol: "GOOG", Bid: 2800.00, Ask: 2805.00, Last: 2802.50})
	batch2 := batcher.FlushTicks()

	if batch2.Sequence != 2 {
		t.Errorf("Expected sequence 2, got %d", batch2.Sequence)
	}

	// Third flush
	batcher.AddTick(contracts.SymbolTick{Symbol: "MSFT", Bid: 350.00, Ask: 350.50, Last: 350.25})
	batch3 := batcher.FlushTicks()

	if batch3.Sequence != 3 {
		t.Errorf("Expected sequence 3, got %d", batch3.Sequence)
	}
}

func TestGetSequence(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	if batcher.GetSequence() != 0 {
		t.Errorf("Expected initial sequence 0, got %d", batcher.GetSequence())
	}

	batcher.AddTick(contracts.SymbolTick{Symbol: "AAPL"})
	batcher.FlushTicks()

	if batcher.GetSequence() != 1 {
		t.Errorf("Expected sequence 1 after flush, got %d", batcher.GetSequence())
	}
}

func TestGetMetrics(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	// Add and flush some ticks
	for i := 0; i < 3; i++ {
		batcher.AddTick(contracts.SymbolTick{Symbol: "AAPL"})
		batcher.AddTick(contracts.SymbolTick{Symbol: "GOOG"})
		batcher.FlushTicks()
	}

	metrics := batcher.GetMetrics()

	if metrics["tick_batches_sent"].(int64) != 3 {
		t.Errorf("Expected 3 batches sent, got %v", metrics["tick_batches_sent"])
	}
	if metrics["total_ticks_buffered"].(int64) != 6 {
		t.Errorf("Expected 6 total ticks buffered, got %v", metrics["total_ticks_buffered"])
	}
	if metrics["total_ticks_flushed"].(int64) != 6 {
		t.Errorf("Expected 6 total ticks flushed, got %v", metrics["total_ticks_flushed"])
	}
	if metrics["avg_ticks_per_batch"].(int64) != 2 {
		t.Errorf("Expected avg 2 ticks per batch, got %v", metrics["avg_ticks_per_batch"])
	}
	if metrics["current_sequence"].(uint64) != 3 {
		t.Errorf("Expected current sequence 3, got %v", metrics["current_sequence"])
	}
	if metrics["pending_ticks"].(int) != 0 {
		t.Errorf("Expected 0 pending ticks, got %v", metrics["pending_ticks"])
	}
}

func TestReset(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	// Add some state
	batcher.AddTick(contracts.SymbolTick{Symbol: "AAPL"})
	batcher.FlushTicks()
	batcher.AddTick(contracts.SymbolTick{Symbol: "GOOG"})

	batcher.Reset()

	if batcher.GetPendingTickCount() != 0 {
		t.Errorf("Expected 0 pending ticks after reset, got %d", batcher.GetPendingTickCount())
	}
	if batcher.GetSequence() != 0 {
		t.Errorf("Expected sequence 0 after reset, got %d", batcher.GetSequence())
	}
}

func TestBatchedMessage_JSON(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	batcher.AddTick(contracts.SymbolTick{
		Symbol: "AAPL",
		Bid:    149.50,
		Ask:    150.00,
		Last:   149.75,
	})

	batch := batcher.FlushTicks()

	jsonBytes, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal batch: %v", err)
	}

	// Verify JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal batch: %v", err)
	}

	if parsed["type"] != "tick_batch" {
		t.Errorf("Expected type 'tick_batch', got '%v'", parsed["type"])
	}
	if parsed["seq"].(float64) != 1 {
		t.Errorf("Expected seq 1, got %v", parsed["seq"])
	}
	if parsed["n"].(float64) != 1 {
		t.Errorf("Expected n 1, got %v", parsed["n"])
	}

	data := parsed["data"].(map[string]interface{})
	symbols := data["symbols"].([]interface{})
	if len(symbols) != 1 {
		t.Errorf("Expected 1 symbol, got %d", len(symbols))
	}
}

func TestBatchingBandwidthSavings(t *testing.T) {
	batcher := NewMessageBatcher(30, time.Second)

	// Simulate 30 ticks (typical scenario)
	for i := 0; i < 30; i++ {
		batcher.AddTick(contracts.SymbolTick{
			Symbol: string(rune('A' + i%26)),
			Bid:    float64(100 + i),
			Ask:    float64(100 + i + 1),
			Last:   float64(100+i) + 0.5,
		})
	}

	batch := batcher.FlushTicks()
	batchedBytes, _ := json.Marshal(batch)

	// Calculate what 30 individual messages would be
	var individualTotal int
	for i := 0; i < 30; i++ {
		individual := map[string]interface{}{
			"type": "tick_snapshot",
			"payload": contracts.SymbolTick{
				Symbol: string(rune('A' + i%26)),
				Bid:    float64(100 + i),
				Ask:    float64(100 + i + 1),
				Last:   float64(100+i) + 0.5,
			},
		}
		bytes, _ := json.Marshal(individual)
		individualTotal += len(bytes)
	}

	t.Logf("Batched message size: %d bytes", len(batchedBytes))
	t.Logf("Individual messages total: %d bytes", individualTotal)
	t.Logf("Savings: %d bytes (%.1f%%)", individualTotal-len(batchedBytes),
		float64(individualTotal-len(batchedBytes))/float64(individualTotal)*100)

	// Batched should be smaller due to reduced overhead
	// (no repeated "type" fields, single JSON envelope)
	if len(batchedBytes) > individualTotal {
		t.Errorf("Batched size (%d) should not exceed individual total (%d)",
			len(batchedBytes), individualTotal)
	}
}

func TestConcurrentBatching(t *testing.T) {
	batcher := NewMessageBatcher(100, time.Second)

	done := make(chan bool)
	numGoroutines := 10
	ticksPerGoroutine := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < ticksPerGoroutine; j++ {
				batcher.AddTick(contracts.SymbolTick{
					Symbol: string(rune('A' + (id*ticksPerGoroutine+j)%26)),
					Bid:    float64(100 + j),
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Flush should work without panicking
	batch := batcher.FlushTicks()

	if batch == nil {
		t.Error("Expected non-nil batch")
	}
	// Note: exact count may vary due to overwrites from same symbols
	if batch.Count == 0 {
		t.Error("Expected non-zero tick count")
	}
}
