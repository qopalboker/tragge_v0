package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// newTestHub creates a Hub suitable for testing (no real deps).
func newTestHub() *Hub {
	metrics := NewMetrics()
	priceBook := NewPriceBook()
	contestMetrics := NewContestMetrics()
	hub := NewHub(priceBook, metrics, time.Second, 30, 2)
	hub.contestMetrics = contestMetrics
	return hub
}

// newTestClient creates a minimal Client for hub-level tests.
// The send and criticalSend channels are buffered so tests can inspect messages without a writePump.
func newTestClient(hub *Hub, userID, contestID string) *Client {
	return &Client{
		hub:          hub,
		userID:       userID,
		contestID:    contestID,
		send:         make(chan MessageEnvelope, 64),
		criticalSend: make(chan MessageEnvelope, 64),
		metrics:      hub.metrics,
		encoding:     EncodingJSON,
		done:         make(chan struct{}),
	}
}

// drainMessages reads all pending messages from both the client's send and criticalSend channels.
func drainMessages(c *Client) []MessageEnvelope {
	var msgs []MessageEnvelope
	for {
		select {
		case msg := <-c.criticalSend:
			msgs = append(msgs, msg)
		case msg := <-c.send:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// TestContestClientIndex_AddRemove verifies that the contest client index
// correctly tracks clients across multiple contests and cleans up properly.
func TestContestClientIndex_AddRemove(t *testing.T) {
	hub := newTestHub()

	// Register clients across multiple contests
	clientA1 := newTestClient(hub, "user-1", "contest-A")
	clientA2 := newTestClient(hub, "user-2", "contest-A")
	clientB1 := newTestClient(hub, "user-3", "contest-B")
	clientB2 := newTestClient(hub, "user-4", "contest-B")
	clientC1 := newTestClient(hub, "user-5", "contest-C")

	hub.addClient(clientA1)
	hub.addClient(clientA2)
	hub.addClient(clientB1)
	hub.addClient(clientB2)
	hub.addClient(clientC1)

	// Verify counts
	if got := hub.GetContestClientCount("contest-A"); got != 2 {
		t.Errorf("contest-A client count: got %d, want 2", got)
	}
	if got := hub.GetContestClientCount("contest-B"); got != 2 {
		t.Errorf("contest-B client count: got %d, want 2", got)
	}
	if got := hub.GetContestClientCount("contest-C"); got != 1 {
		t.Errorf("contest-C client count: got %d, want 1", got)
	}
	if got := len(hub.GetActiveContests()); got != 3 {
		t.Errorf("active contests: got %d, want 3", got)
	}

	// Remove one client from contest-A
	hub.removeClient(clientA1)
	if got := hub.GetContestClientCount("contest-A"); got != 1 {
		t.Errorf("contest-A after remove: got %d, want 1", got)
	}

	// Remove both clients from contest-B
	hub.removeClient(clientB1)
	hub.removeClient(clientB2)

	// contest-B should be completely gone
	if got := hub.GetContestClientCount("contest-B"); got != 0 {
		t.Errorf("contest-B after full remove: got %d, want 0", got)
	}

	// Verify contest-B is no longer in active contests
	actives := hub.GetActiveContests()
	for _, c := range actives {
		if c == "contest-B" {
			t.Error("contest-B should not be in active contests after all clients removed")
		}
	}

	// Remaining: 1 in contest-A, 1 in contest-C
	if got := len(hub.GetActiveContests()); got != 2 {
		t.Errorf("active contests after removals: got %d, want 2", got)
	}

	// Total connections should be 2
	if got := hub.metrics.wsConnections.Load(); got != 2 {
		t.Errorf("total connections: got %d, want 2", got)
	}
}

// TestSendToContest_Isolation verifies that SendToContest only delivers
// messages to clients in the target contest.
func TestSendToContest_Isolation(t *testing.T) {
	hub := newTestHub()

	clientA := newTestClient(hub, "user-1", "contest-A")
	clientB := newTestClient(hub, "user-2", "contest-B")

	hub.addClient(clientA)
	hub.addClient(clientB)

	// Send a message to contest-A only
	hub.SendToContest("contest-A", &WSMessage{
		Type:    "test",
		Payload: map[string]string{"target": "contest-A"},
	})

	// Give the worker pool a moment to process if it's being used
	time.Sleep(10 * time.Millisecond)

	// Client A should have received the message
	msgsA := drainMessages(clientA)
	if len(msgsA) != 1 {
		t.Errorf("contest-A client: got %d messages, want 1", len(msgsA))
	}

	// Client B should NOT have received any message
	msgsB := drainMessages(clientB)
	if len(msgsB) != 0 {
		t.Errorf("contest-B client: got %d messages, want 0", len(msgsB))
	}
}

// TestContestCleanup_LastClientDisconnects verifies that when the last client
// in a contest disconnects, both the contest index and symbol cache are cleaned up.
func TestContestCleanup_LastClientDisconnects(t *testing.T) {
	hub := newTestHub()

	client := newTestClient(hub, "user-1", "contest-X")
	hub.addClient(client)

	// Pre-populate the contest symbols cache
	hub.contestSymbols.set("contest-X", map[string]bool{
		"AAPL": true,
		"GOOG": true,
	})

	// Verify cache is populated
	if got := hub.contestSymbols.count("contest-X"); got != 2 {
		t.Errorf("cached symbols before remove: got %d, want 2", got)
	}

	// Remove the only client
	hub.removeClient(client)

	// Contest index should be cleaned up
	if got := hub.GetContestClientCount("contest-X"); got != 0 {
		t.Errorf("contest-X client count after remove: got %d, want 0", got)
	}

	// Symbol cache should be cleaned up
	if _, found := hub.contestSymbols.get("contest-X"); found {
		t.Error("contest-X symbols should be deleted from cache after last client disconnects")
	}
	if got := hub.contestSymbols.count("contest-X"); got != -1 {
		t.Errorf("cached symbols after remove: got %d, want -1 (not cached)", got)
	}
}

// TestConcurrentRegistration_NoRace spawns 100 goroutines each registering a
// client to one of 5 random contests. Run with -race to verify no data races.
func TestConcurrentRegistration_NoRace(t *testing.T) {
	hub := newTestHub()

	contests := []string{"contest-1", "contest-2", "contest-3", "contest-4", "contest-5"}
	numGoroutines := 100

	var wg sync.WaitGroup
	clients := make([]*Client, numGoroutines)

	// Register concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			contestID := contests[rand.Intn(len(contests))]
			c := newTestClient(hub, fmt.Sprintf("user-%d", idx), contestID)
			clients[idx] = c
			hub.addClient(c)
		}(i)
	}
	wg.Wait()

	// Verify total count
	if got := hub.metrics.wsConnections.Load(); got != int64(numGoroutines) {
		t.Errorf("total connections: got %d, want %d", got, numGoroutines)
	}

	// Verify all contests have consistent counts
	totalFromContests := 0
	for _, contestID := range contests {
		totalFromContests += hub.GetContestClientCount(contestID)
	}
	if totalFromContests != numGoroutines {
		t.Errorf("sum of contest counts: got %d, want %d", totalFromContests, numGoroutines)
	}

	// Remove all concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.removeClient(clients[idx])
		}(i)
	}
	wg.Wait()

	// After all removed, should be zero
	if got := hub.metrics.wsConnections.Load(); got != 0 {
		t.Errorf("total connections after full removal: got %d, want 0", got)
	}
	if got := len(hub.GetActiveContests()); got != 0 {
		t.Errorf("active contests after full removal: got %d, want 0", got)
	}
}

// TestSymbolFiltering_PerContest verifies that each contest's clients receive
// only the symbols belonging to their contest during a broadcast.
func TestSymbolFiltering_PerContest(t *testing.T) {
	hub := newTestHub()

	// Set up two contests with different symbol sets
	hub.contestSymbols.set("contest-tech", map[string]bool{
		"AAPL": true,
		"GOOG": true,
		"MSFT": true,
	})
	hub.contestSymbols.set("contest-finance", map[string]bool{
		"JPM":  true,
		"GS":   true,
	})

	// Populate the PriceBook with ticks for all symbols
	allSymbols := []string{"AAPL", "GOOG", "MSFT", "JPM", "GS", "TSLA"}
	for _, sym := range allSymbols {
		hub.priceBook.Update(contracts.SymbolTick{
			Symbol: sym,
			Bid:    100.0,
			Ask:    101.0,
			Last:   100.5,
		})
	}

	// Register clients
	clientTech := newTestClient(hub, "user-tech", "contest-tech")
	clientFinance := newTestClient(hub, "user-finance", "contest-finance")

	hub.addClient(clientTech)
	hub.addClient(clientFinance)

	// Trigger a contest-aware broadcast
	hub.broadcastBatchedUpdatesContestAware()

	// Wait a moment for messages to be delivered
	time.Sleep(20 * time.Millisecond)

	// Check tech client's messages
	techMsgs := drainMessages(clientTech)
	if len(techMsgs) != 1 {
		t.Fatalf("tech client: got %d messages, want 1", len(techMsgs))
	}
	techBatch := parseBatchedMessage(t, techMsgs[0].Data)
	techSymbols := extractSymbols(techBatch)

	// Tech contest should only have AAPL, GOOG, MSFT
	for _, sym := range []string{"AAPL", "GOOG", "MSFT"} {
		if !techSymbols[sym] {
			t.Errorf("tech client missing symbol %s", sym)
		}
	}
	for _, sym := range []string{"JPM", "GS", "TSLA"} {
		if techSymbols[sym] {
			t.Errorf("tech client should not have symbol %s", sym)
		}
	}

	// Check finance client's messages
	financeMsgs := drainMessages(clientFinance)
	if len(financeMsgs) != 1 {
		t.Fatalf("finance client: got %d messages, want 1", len(financeMsgs))
	}
	financeBatch := parseBatchedMessage(t, financeMsgs[0].Data)
	financeSymbols := extractSymbols(financeBatch)

	// Finance contest should only have JPM, GS
	for _, sym := range []string{"JPM", "GS"} {
		if !financeSymbols[sym] {
			t.Errorf("finance client missing symbol %s", sym)
		}
	}
	for _, sym := range []string{"AAPL", "GOOG", "MSFT", "TSLA"} {
		if financeSymbols[sym] {
			t.Errorf("finance client should not have symbol %s", sym)
		}
	}
}

// TestFilterSymbolsForContest_NilAllowedReturnsAll verifies that a nil
// allowedSymbols set (query failure fallback) returns all symbols unfiltered.
func TestFilterSymbolsForContest_NilAllowedReturnsAll(t *testing.T) {
	symbols := []contracts.SymbolTick{
		{Symbol: "AAPL", Bid: 100, Ask: 101, Last: 100.5},
		{Symbol: "GOOG", Bid: 200, Ask: 201, Last: 200.5},
	}

	filtered := filterSymbolsForContest(symbols, nil)
	if len(filtered) != len(symbols) {
		t.Errorf("nil filter: got %d symbols, want %d", len(filtered), len(symbols))
	}
}

// TestHubStatus verifies the admin hub status endpoint returns correct data.
func TestHubStatus(t *testing.T) {
	hub := newTestHub()

	clientA := newTestClient(hub, "user-1", "contest-A")
	clientB := newTestClient(hub, "user-2", "contest-B")
	clientA2 := newTestClient(hub, "user-3", "contest-A")

	hub.addClient(clientA)
	hub.addClient(clientB)
	hub.addClient(clientA2)

	// Pre-populate symbol cache for contest-A
	hub.contestSymbols.set("contest-A", map[string]bool{"AAPL": true, "GOOG": true})

	status := hub.GetHubStatus()

	if status.TotalConnections != 3 {
		t.Errorf("total connections: got %d, want 3", status.TotalConnections)
	}
	if status.ActiveContests != 2 {
		t.Errorf("active contests: got %d, want 2", status.ActiveContests)
	}
	if status.BroadcastWorkers != 2 {
		t.Errorf("broadcast workers: got %d, want 2", status.BroadcastWorkers)
	}

	contestA, ok := status.Contests["contest-A"]
	if !ok {
		t.Fatal("contest-A missing from status")
	}
	if contestA.ClientCount != 2 {
		t.Errorf("contest-A client count: got %d, want 2", contestA.ClientCount)
	}
	if contestA.CachedSymbolCount != 2 {
		t.Errorf("contest-A cached symbols: got %d, want 2", contestA.CachedSymbolCount)
	}

	contestB, ok := status.Contests["contest-B"]
	if !ok {
		t.Fatal("contest-B missing from status")
	}
	if contestB.ClientCount != 1 {
		t.Errorf("contest-B client count: got %d, want 1", contestB.ClientCount)
	}
	// contest-B has no cached symbols
	if contestB.CachedSymbolCount != -1 {
		t.Errorf("contest-B cached symbols: got %d, want -1", contestB.CachedSymbolCount)
	}
}

// TestInvalidateContestSymbolsCache verifies cache invalidation works correctly.
func TestInvalidateContestSymbolsCache(t *testing.T) {
	hub := newTestHub()

	// Populate cache
	hub.contestSymbols.set("contest-X", map[string]bool{"AAPL": true})
	if _, found := hub.contestSymbols.get("contest-X"); !found {
		t.Fatal("cache should be populated")
	}

	// Invalidate
	hub.InvalidateContestSymbolsCache("contest-X")

	if _, found := hub.contestSymbols.get("contest-X"); found {
		t.Error("cache should be empty after invalidation")
	}
}

// parseBatchedMessage decodes a JSON-encoded BatchedMessage from raw bytes.
func parseBatchedMessage(t *testing.T, data []byte) *BatchedMessage {
	t.Helper()

	// Decode into a raw structure since BatchedMessage.Data is interface{}
	var raw struct {
		Type     string          `json:"type"`
		Sequence uint64          `json:"seq"`
		Count    int             `json:"n"`
		Data     json.RawMessage `json:"data"`
		Ts       int64           `json:"ts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal batched message: %v", err)
	}

	var batchData TickBatchData
	if err := json.Unmarshal(raw.Data, &batchData); err != nil {
		t.Fatalf("failed to unmarshal tick batch data: %v", err)
	}

	return &BatchedMessage{
		Type:     raw.Type,
		Sequence: raw.Sequence,
		Count:    raw.Count,
		Data:     batchData,
		Ts:       raw.Ts,
	}
}

// TestContestSymbolCache_TTLExpiration verifies that cache entries expire after TTL.
func TestContestSymbolCache_TTLExpiration(t *testing.T) {
	cache := newContestSymbolCache(100 * time.Millisecond)

	cache.set("contest-X", map[string]bool{"AAPL": true, "GOOG": true})

	// Should be found immediately
	syms, found := cache.get("contest-X")
	if !found {
		t.Fatal("cache entry should be found before TTL expires")
	}
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(syms))
	}
	if cache.count("contest-X") != 2 {
		t.Errorf("count should be 2, got %d", cache.count("contest-X"))
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	_, found = cache.get("contest-X")
	if found {
		t.Error("cache entry should be expired after TTL")
	}
	if cache.count("contest-X") != -1 {
		t.Errorf("count should be -1 (expired), got %d", cache.count("contest-X"))
	}
}

// TestCriticalMessageNotDroppedUnderBackpressure verifies that critical messages
// (sent via SendToUser) are delivered even when the regular send queue is full.
func TestCriticalMessageNotDroppedUnderBackpressure(t *testing.T) {
	hub := newTestHub()

	// Create a client with a tiny regular send buffer to simulate backpressure
	client := &Client{
		hub:          hub,
		userID:       "user-1",
		contestID:    "contest-A",
		send:         make(chan MessageEnvelope, 1),
		criticalSend: make(chan MessageEnvelope, 16),
		metrics:      hub.metrics,
		encoding:     EncodingJSON,
		done:         make(chan struct{}),
	}
	hub.addClient(client)

	// Fill the regular send queue with a tick
	client.SendTickBatch([]byte(`{"type":"tick_batch"}`), nil)

	// Send a critical message via SendToUser — should go to criticalSend, not blocked by send
	hub.SendToUser("user-1", &WSMessage{
		Type:    "fill",
		Payload: map[string]string{"fill_id": "f-123"},
	})

	// The critical message should be in criticalSend
	select {
	case msg := <-client.criticalSend:
		var wsMsg WSMessage
		if err := json.Unmarshal(msg.Data, &wsMsg); err != nil {
			t.Fatalf("failed to unmarshal critical message: %v", err)
		}
		if wsMsg.Type != "fill" {
			t.Errorf("expected fill message, got %s", wsMsg.Type)
		}
	default:
		t.Error("critical message was not delivered to criticalSend channel")
	}

	// The regular send channel should still have the tick
	select {
	case msg := <-client.send:
		if msg.MsgType != "tick_batch" {
			t.Errorf("expected tick_batch in send channel, got %s", msg.MsgType)
		}
	default:
		t.Error("tick_batch should still be in the regular send channel")
	}
}

// TestCriticalMessageViaContestBroadcast verifies that SendToContest
// routes through the critical channel.
func TestCriticalMessageViaContestBroadcast(t *testing.T) {
	hub := newTestHub()

	client := newTestClient(hub, "user-1", "contest-A")
	hub.addClient(client)

	hub.SendToContest("contest-A", &WSMessage{
		Type:    "contest_state",
		Payload: map[string]string{"status": "running"},
	})

	// Give worker pool time to process if used
	time.Sleep(10 * time.Millisecond)

	// Should arrive in criticalSend
	select {
	case msg := <-client.criticalSend:
		var wsMsg WSMessage
		if err := json.Unmarshal(msg.Data, &wsMsg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if wsMsg.Type != "contest_state" {
			t.Errorf("expected contest_state, got %s", wsMsg.Type)
		}
	default:
		t.Error("contest broadcast should be delivered via criticalSend")
	}
}

// extractSymbols returns a set of symbol names from a batched message.
func extractSymbols(batch *BatchedMessage) map[string]bool {
	result := make(map[string]bool)
	if batchData, ok := batch.Data.(TickBatchData); ok {
		for _, tick := range batchData.Symbols {
			result[tick.Symbol] = true
		}
	}
	return result
}
