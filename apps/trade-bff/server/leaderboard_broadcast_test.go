package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"go.uber.org/zap"
)

// newTestAppForBroadcast creates a minimal App wired to a test Hub suitable for
// leaderboard broadcast / debounce tests. The LeaderboardBroadcastDebounce is
// configurable so tests can use short intervals without waiting 2 real seconds.
func newTestAppForBroadcast(t *testing.T, hub *Hub, debounce time.Duration) *App {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create a no-op observability so a.log() doesn't panic
	obs := &observability.Observability{
		Logger: &observability.Logger{Logger: zap.NewNop()},
	}

	return &App{
		config: &Config{
			LeaderboardBroadcastDebounce: debounce,
		},
		obs:              obs,
		hub:              hub,
		lbDebounceTimers: make(map[string]*time.Timer),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// --------------------------------------------------------------------------
// Test 4: TestSmartLeaderboardBroadcast
// Trigger a leaderboard update. Verify the WebSocket message includes
// top_entries with 10 entries and total_participants.
// --------------------------------------------------------------------------

func TestSmartLeaderboardBroadcast(t *testing.T) {
	hub := newTestHub()

	// Register a client in "contest-lb"
	client := newTestClient(hub, "user-observer", "contest-lb")
	hub.addClient(client)

	app := newTestAppForBroadcast(t, hub, 100*time.Millisecond)

	// Create a LeaderboardManager with pre-populated cache (no Redis needed).
	// Since the test is in package main, unexported fields are accessible.
	usernameCache, _ := lru.New[string, string](1000)
	lm := &LeaderboardManager{
		usernameCache: usernameCache,
		top10Cache:    make(map[string]*top10CacheEntry),
	}

	// Populate cache with exactly 10 entries
	entries := make([]LeaderboardEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = LeaderboardEntry{
			Rank:            i + 1,
			UserID:          fmt.Sprintf("user-%d", i+1),
			Username:        fmt.Sprintf("player%d", i+1),
			TotalScore:      float64(1000 - i*100),
			RealizedScore:   float64(600 - i*50),
			UnrealizedScore: float64(400 - i*50),
		}
	}
	lm.top10CacheMu.Lock()
	lm.top10Cache["contest-lb"] = &top10CacheEntry{
		entries:           entries,
		totalParticipants: 250,
		expiresAt:         time.Now().Add(5 * time.Minute),
	}
	lm.top10CacheMu.Unlock()

	app.leaderboardMgr = lm

	// Trigger broadcast directly (bypassing debounce for payload verification)
	app.fireLeaderboardBroadcast("contest-lb")

	// Allow time for the message to be queued
	time.Sleep(20 * time.Millisecond)

	// Drain messages from the client
	msgs := drainMessages(client)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 broadcast message, got %d", len(msgs))
	}

	// Parse the received message
	var wsMsg struct {
		Type    string                 `json:"type"`
		Payload map[string]interface{} `json:"payload"`
	}
	if err := json.Unmarshal(msgs[0].Data, &wsMsg); err != nil {
		t.Fatalf("failed to unmarshal broadcast message: %v", err)
	}

	// Verify message type
	if wsMsg.Type != "leaderboard_updated" {
		t.Errorf("expected message type 'leaderboard_updated', got %q", wsMsg.Type)
	}

	// Verify contest_id
	if cid, ok := wsMsg.Payload["contest_id"].(string); !ok || cid != "contest-lb" {
		t.Errorf("expected contest_id 'contest-lb', got %v", wsMsg.Payload["contest_id"])
	}

	// Verify top_entries is present and has 10 entries
	topEntriesRaw, ok := wsMsg.Payload["top_entries"]
	if !ok {
		t.Fatal("payload missing 'top_entries' field")
	}
	topEntries, ok := topEntriesRaw.([]interface{})
	if !ok {
		t.Fatalf("top_entries is not an array, type: %T", topEntriesRaw)
	}
	if len(topEntries) != 10 {
		t.Errorf("expected 10 top_entries, got %d", len(topEntries))
	}

	// Verify each entry has the expected fields
	for i, raw := range topEntries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("entry %d is not an object", i)
			continue
		}
		if _, ok := entry["rank"]; !ok {
			t.Errorf("entry %d missing 'rank' field", i)
		}
		if _, ok := entry["user_id"]; !ok {
			t.Errorf("entry %d missing 'user_id' field", i)
		}
		if _, ok := entry["total_score"]; !ok {
			t.Errorf("entry %d missing 'total_score' field", i)
		}
	}

	// Verify total_participants is present and correct
	totalParticipantsRaw, ok := wsMsg.Payload["total_participants"]
	if !ok {
		t.Fatal("payload missing 'total_participants' field")
	}
	// JSON numbers unmarshal as float64
	totalParticipants, ok := totalParticipantsRaw.(float64)
	if !ok {
		t.Fatalf("total_participants is not a number, type: %T", totalParticipantsRaw)
	}
	if int64(totalParticipants) != 250 {
		t.Errorf("expected total_participants 250, got %v", totalParticipants)
	}

	// Verify timestamp is present
	if _, ok := wsMsg.Payload["timestamp"]; !ok {
		t.Error("payload missing 'timestamp' field")
	}
}

// --------------------------------------------------------------------------
// Test 5: TestLeaderboardDebounce
// Send 10 rapid PnL deltas for the same contest within 500ms.
// Verify only 1 broadcast is emitted (after the debounce window).
// --------------------------------------------------------------------------

func TestLeaderboardDebounce(t *testing.T) {
	hub := newTestHub()

	// Register a client in the target contest
	client := newTestClient(hub, "user-observer", "contest-debounce")
	hub.addClient(client)

	// Use a short debounce interval for fast testing (200ms)
	app := newTestAppForBroadcast(t, hub, 200*time.Millisecond)
	// leaderboardMgr is nil — broadcasts will still be sent but without top_entries

	// Rapidly schedule 10 leaderboard broadcasts for the same contest
	for i := 0; i < 10; i++ {
		app.scheduleLeaderboardBroadcast("contest-debounce")
	}

	// Verify only one timer is active (debounce coalesced the 10 calls)
	app.lbDebounceMu.Lock()
	timerCount := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if timerCount != 1 {
		t.Errorf("expected 1 active debounce timer, got %d", timerCount)
	}

	// Wait for the debounce window to fire (200ms + buffer)
	time.Sleep(400 * time.Millisecond)

	// Drain messages — should be exactly 1
	msgs := drainMessages(client)
	if len(msgs) != 1 {
		t.Errorf("expected exactly 1 broadcast after debounce, got %d", len(msgs))
	}

	// Verify the timer was cleaned up after firing
	app.lbDebounceMu.Lock()
	timerCountAfter := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if timerCountAfter != 0 {
		t.Errorf("expected 0 active timers after debounce fired, got %d", timerCountAfter)
	}

	// Verify the broadcast message is for the correct contest
	if len(msgs) == 1 {
		var wsMsg struct {
			Type    string                 `json:"type"`
			Payload map[string]interface{} `json:"payload"`
		}
		if err := json.Unmarshal(msgs[0].Data, &wsMsg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if wsMsg.Type != "leaderboard_updated" {
			t.Errorf("expected type 'leaderboard_updated', got %q", wsMsg.Type)
		}
		if cid, _ := wsMsg.Payload["contest_id"].(string); cid != "contest-debounce" {
			t.Errorf("expected contest_id 'contest-debounce', got %q", cid)
		}
	}
}

// --------------------------------------------------------------------------
// Test 6: TestDebouncePerContest
// Send deltas for contest A and contest B. Verify each contest gets its own
// independent debounce timer and broadcast.
// --------------------------------------------------------------------------

func TestDebouncePerContest(t *testing.T) {
	hub := newTestHub()

	// Register clients in two different contests
	clientA := newTestClient(hub, "user-A", "contest-A")
	clientB := newTestClient(hub, "user-B", "contest-B")
	hub.addClient(clientA)
	hub.addClient(clientB)

	// Use a short debounce interval
	app := newTestAppForBroadcast(t, hub, 150*time.Millisecond)

	// Schedule broadcasts for both contests
	app.scheduleLeaderboardBroadcast("contest-A")
	app.scheduleLeaderboardBroadcast("contest-B")

	// Verify two independent timers are active
	app.lbDebounceMu.Lock()
	timerCount := len(app.lbDebounceTimers)
	_, hasTimerA := app.lbDebounceTimers["contest-A"]
	_, hasTimerB := app.lbDebounceTimers["contest-B"]
	app.lbDebounceMu.Unlock()

	if timerCount != 2 {
		t.Errorf("expected 2 active debounce timers, got %d", timerCount)
	}
	if !hasTimerA {
		t.Error("expected timer for contest-A")
	}
	if !hasTimerB {
		t.Error("expected timer for contest-B")
	}

	// Schedule more broadcasts rapidly for contest-A only
	// These should be absorbed by the existing timer (no new timer created)
	for i := 0; i < 5; i++ {
		app.scheduleLeaderboardBroadcast("contest-A")
	}

	// Timer count should still be 2
	app.lbDebounceMu.Lock()
	timerCountAfterMore := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if timerCountAfterMore != 2 {
		t.Errorf("expected 2 timers after additional schedules, got %d", timerCountAfterMore)
	}

	// Wait for both debounce windows to fire
	time.Sleep(300 * time.Millisecond)

	// Client A should have received exactly 1 broadcast (for contest-A)
	msgsA := drainMessages(clientA)
	if len(msgsA) != 1 {
		t.Errorf("contest-A client: expected 1 broadcast, got %d", len(msgsA))
	}

	// Client B should have received exactly 1 broadcast (for contest-B)
	msgsB := drainMessages(clientB)
	if len(msgsB) != 1 {
		t.Errorf("contest-B client: expected 1 broadcast, got %d", len(msgsB))
	}

	// Verify contest isolation — client A's message should be for contest-A
	if len(msgsA) == 1 {
		var wsMsg struct {
			Payload map[string]interface{} `json:"payload"`
		}
		json.Unmarshal(msgsA[0].Data, &wsMsg)
		if cid, _ := wsMsg.Payload["contest_id"].(string); cid != "contest-A" {
			t.Errorf("client A received broadcast for %q, expected 'contest-A'", cid)
		}
	}

	// Client B's message should be for contest-B
	if len(msgsB) == 1 {
		var wsMsg struct {
			Payload map[string]interface{} `json:"payload"`
		}
		json.Unmarshal(msgsB[0].Data, &wsMsg)
		if cid, _ := wsMsg.Payload["contest_id"].(string); cid != "contest-B" {
			t.Errorf("client B received broadcast for %q, expected 'contest-B'", cid)
		}
	}

	// All timers should be cleaned up
	app.lbDebounceMu.Lock()
	finalTimerCount := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if finalTimerCount != 0 {
		t.Errorf("expected 0 timers after all debounces fired, got %d", finalTimerCount)
	}
}

// TestStopAllLeaderboardDebounceTimers verifies that pending debounce timers
// are properly stopped during shutdown, preventing goroutine leaks.
func TestStopAllLeaderboardDebounceTimers(t *testing.T) {
	hub := newTestHub()

	clientA := newTestClient(hub, "user-A", "contest-A")
	clientB := newTestClient(hub, "user-B", "contest-B")
	hub.addClient(clientA)
	hub.addClient(clientB)

	// Use a long debounce so timers are still pending when we stop them
	app := newTestAppForBroadcast(t, hub, 10*time.Second)

	// Schedule broadcasts for two contests
	app.scheduleLeaderboardBroadcast("contest-A")
	app.scheduleLeaderboardBroadcast("contest-B")

	// Verify timers are active
	app.lbDebounceMu.Lock()
	if len(app.lbDebounceTimers) != 2 {
		t.Fatalf("expected 2 pending timers, got %d", len(app.lbDebounceTimers))
	}
	app.lbDebounceMu.Unlock()

	// Stop all timers (simulates shutdown)
	app.stopAllLeaderboardDebounceTimers()

	// Verify all timers are cleaned up
	app.lbDebounceMu.Lock()
	remaining := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 timers after stop, got %d", remaining)
	}

	// Verify no broadcasts were sent (timers were stopped before firing)
	time.Sleep(50 * time.Millisecond) // Brief wait to catch any leaked goroutines
	msgsA := drainMessages(clientA)
	msgsB := drainMessages(clientB)
	if len(msgsA) != 0 {
		t.Errorf("client A should have 0 messages after timer stop, got %d", len(msgsA))
	}
	if len(msgsB) != 0 {
		t.Errorf("client B should have 0 messages after timer stop, got %d", len(msgsB))
	}
}

// TestDebounceNewCycleAfterFire verifies that after a debounce timer fires,
// a new broadcast can be scheduled immediately (fresh debounce cycle).
func TestDebounceNewCycleAfterFire(t *testing.T) {
	hub := newTestHub()

	client := newTestClient(hub, "user-1", "contest-cycle")
	hub.addClient(client)

	app := newTestAppForBroadcast(t, hub, 100*time.Millisecond)

	// Counter to track number of broadcasts received
	var broadcastCount atomic.Int32

	// First cycle: schedule and wait for fire
	app.scheduleLeaderboardBroadcast("contest-cycle")
	time.Sleep(200 * time.Millisecond)

	msgs1 := drainMessages(client)
	broadcastCount.Add(int32(len(msgs1)))

	// Second cycle: schedule again — this should start a new timer
	app.scheduleLeaderboardBroadcast("contest-cycle")
	time.Sleep(200 * time.Millisecond)

	msgs2 := drainMessages(client)
	broadcastCount.Add(int32(len(msgs2)))

	// Should have received 2 broadcasts total (one per cycle)
	total := broadcastCount.Load()
	if total != 2 {
		t.Errorf("expected 2 broadcasts across 2 cycles, got %d", total)
	}
}

// TestDebounceRaceCondition spawns multiple goroutines scheduling broadcasts
// concurrently for the same contest. Run with -race to verify no data races.
func TestDebounceRaceCondition(t *testing.T) {
	hub := newTestHub()

	client := newTestClient(hub, "user-1", "contest-race")
	hub.addClient(client)

	app := newTestAppForBroadcast(t, hub, 200*time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app.scheduleLeaderboardBroadcast("contest-race")
		}()
	}
	wg.Wait()

	// Only 1 timer should be active (first goroutine created it, rest were no-ops)
	app.lbDebounceMu.Lock()
	tc := len(app.lbDebounceTimers)
	app.lbDebounceMu.Unlock()
	if tc != 1 {
		t.Errorf("expected 1 timer after concurrent schedules, got %d", tc)
	}

	// Wait for debounce to fire
	time.Sleep(400 * time.Millisecond)

	msgs := drainMessages(client)
	if len(msgs) != 1 {
		t.Errorf("expected 1 broadcast after concurrent schedules, got %d", len(msgs))
	}
}
