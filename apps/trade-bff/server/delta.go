package server

import (
	"encoding/json"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// priceEpsilon is the threshold for float64 comparison.
// Values differing by less than this are considered equal.
const priceEpsilon = 1e-8

// metricsSampleRate controls how often json.Marshal is called for byte-size metrics.
// Only every Nth delta triggers marshaling; byte counts are scaled accordingly.
const metricsSampleRate = 100

// floatChanged returns true if a and b differ by more than priceEpsilon.
func floatChanged(a, b float64) bool {
	return math.Abs(a-b) > priceEpsilon
}

// DeltaEncoder tracks previous state and generates deltas for position updates
// This reduces bandwidth by only sending changed fields instead of full state
type DeltaEncoder struct {
	// Per-user last sent state
	userState map[string]*UserStateSnapshot
	mu        sync.RWMutex

	// Metrics tracking
	metrics *DeltaMetrics
}

// DeltaMetrics tracks delta encoding statistics using atomic counters
// to avoid nested mutex overhead. The sampleCounter is a plain int64
// because it is only accessed while the DeltaEncoder's mu is held.
type DeltaMetrics struct {
	TotalEncodings     atomic.Int64
	FullSyncs          atomic.Int64
	DeltasGenerated    atomic.Int64
	BytesSaved         atomic.Int64
	TotalOriginalBytes atomic.Int64
	TotalDeltaBytes    atomic.Int64
	sampleCounter      int64 // only accessed under DeltaEncoder.mu
}

// UserStateSnapshot represents the complete state for a user at a point in time
type UserStateSnapshot struct {
	Positions map[string]*PositionSnapshot // positionID -> snapshot
	Balance   *BalanceSnapshot
	Timestamp int64
}

// PositionSnapshot represents a position's state for delta comparison
type PositionSnapshot struct {
	Symbol        string  `json:"s"`
	Side          string  `json:"side"`
	UnrealizedPnL float64 `json:"pnl"`
	CurrentPrice  float64 `json:"cp"`
	QtyOpen       int64   `json:"qty"`
	AvgPrice      float64 `json:"avg"`
}

// BalanceSnapshot represents account balance state
type BalanceSnapshot struct {
	Available int64   `json:"avail"`
	Total     int64   `json:"total"`
	Equity    float64 `json:"eq"`
}

// PositionDelta represents changes to a position
type PositionDelta struct {
	PositionID string                 `json:"id"`
	Changes    map[string]interface{} `json:"c"`
}

// StateDelta represents all changes for a user
type StateDelta struct {
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"ts"`
	Positions []PositionDelta        `json:"p,omitempty"`
	Balance   map[string]interface{} `json:"b,omitempty"`
	FullSync  bool                   `json:"full,omitempty"`
}

// NewDeltaEncoder creates a new DeltaEncoder instance
func NewDeltaEncoder() *DeltaEncoder {
	return &DeltaEncoder{
		userState: make(map[string]*UserStateSnapshot),
		metrics:   &DeltaMetrics{},
	}
}

// EncodeDelta compares current state with previous and returns delta
// Returns the delta and whether there are any changes to send
func (d *DeltaEncoder) EncodeDelta(userID string, current *UserStateSnapshot) (*StateDelta, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.metrics.TotalEncodings.Add(1)

	previous, exists := d.userState[userID]
	if !exists || previous == nil {
		// First time - send full state
		d.userState[userID] = current
		d.recordFullSync()
		return d.fullSync(current), true
	}

	delta := &StateDelta{
		Type:      "state_delta",
		Timestamp: current.Timestamp,
		Positions: make([]PositionDelta, 0),
	}

	hasChanges := false

	// Compare positions
	for posID, currPos := range current.Positions {
		prevPos, existed := previous.Positions[posID]

		if !existed {
			// New position
			delta.Positions = append(delta.Positions, PositionDelta{
				PositionID: posID,
				Changes: map[string]interface{}{
					"s":    currPos.Symbol,
					"side": currPos.Side,
					"pnl":  currPos.UnrealizedPnL,
					"cp":   currPos.CurrentPrice,
					"qty":  currPos.QtyOpen,
					"avg":  currPos.AvgPrice,
					"new":  true,
				},
			})
			hasChanges = true
			continue
		}

		// Compare fields and only include changed ones
		changes := make(map[string]interface{})

		if floatChanged(currPos.UnrealizedPnL, prevPos.UnrealizedPnL) {
			changes["pnl"] = currPos.UnrealizedPnL
		}
		if floatChanged(currPos.CurrentPrice, prevPos.CurrentPrice) {
			changes["cp"] = currPos.CurrentPrice
		}
		if currPos.QtyOpen != prevPos.QtyOpen {
			changes["qty"] = currPos.QtyOpen
		}
		if floatChanged(currPos.AvgPrice, prevPos.AvgPrice) {
			changes["avg"] = currPos.AvgPrice
		}
		if currPos.Side != prevPos.Side {
			changes["side"] = currPos.Side
		}

		if len(changes) > 0 {
			delta.Positions = append(delta.Positions, PositionDelta{
				PositionID: posID,
				Changes:    changes,
			})
			hasChanges = true
		}
	}

	// Check for closed positions (positions that existed before but not now)
	for posID := range previous.Positions {
		if _, exists := current.Positions[posID]; !exists {
			delta.Positions = append(delta.Positions, PositionDelta{
				PositionID: posID,
				Changes:    map[string]interface{}{"closed": true},
			})
			hasChanges = true
		}
	}

	// Compare balance
	if current.Balance != nil && previous.Balance != nil {
		balanceChanges := make(map[string]interface{})
		if current.Balance.Available != previous.Balance.Available {
			balanceChanges["avail"] = current.Balance.Available
		}
		if current.Balance.Total != previous.Balance.Total {
			balanceChanges["total"] = current.Balance.Total
		}
		if floatChanged(current.Balance.Equity, previous.Balance.Equity) {
			balanceChanges["eq"] = current.Balance.Equity
		}
		if len(balanceChanges) > 0 {
			delta.Balance = balanceChanges
			hasChanges = true
		}
	} else if current.Balance != nil && previous.Balance == nil {
		// Balance is newly available
		delta.Balance = map[string]interface{}{
			"avail": current.Balance.Available,
			"total": current.Balance.Total,
			"eq":    current.Balance.Equity,
		}
		hasChanges = true
	}

	// Update stored state
	d.userState[userID] = current

	if hasChanges {
		d.recordDeltaGenerated(previous, current, delta)
	}

	return delta, hasChanges
}

// fullSync creates a full state sync message
func (d *DeltaEncoder) fullSync(state *UserStateSnapshot) *StateDelta {
	delta := &StateDelta{
		Type:      "state_delta",
		Timestamp: state.Timestamp,
		FullSync:  true,
		Positions: make([]PositionDelta, 0, len(state.Positions)),
	}

	for posID, pos := range state.Positions {
		delta.Positions = append(delta.Positions, PositionDelta{
			PositionID: posID,
			Changes: map[string]interface{}{
				"s":    pos.Symbol,
				"side": pos.Side,
				"pnl":  pos.UnrealizedPnL,
				"cp":   pos.CurrentPrice,
				"qty":  pos.QtyOpen,
				"avg":  pos.AvgPrice,
			},
		})
	}

	if state.Balance != nil {
		delta.Balance = map[string]interface{}{
			"avail": state.Balance.Available,
			"total": state.Balance.Total,
			"eq":    state.Balance.Equity,
		}
	}

	return delta
}

// RemoveUser cleans up state for disconnected user
func (d *DeltaEncoder) RemoveUser(userID string) {
	d.mu.Lock()
	delete(d.userState, userID)
	d.mu.Unlock()
}

// ForceFullSync marks a user for full sync on next update
func (d *DeltaEncoder) ForceFullSync(userID string) {
	d.mu.Lock()
	delete(d.userState, userID)
	d.mu.Unlock()
}

// GetUserCount returns the number of users with tracked state
func (d *DeltaEncoder) GetUserCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.userState)
}

// recordFullSync updates metrics for a full sync
func (d *DeltaEncoder) recordFullSync() {
	d.metrics.FullSyncs.Add(1)
}

// recordDeltaGenerated updates metrics when a delta is generated.
// To avoid json.Marshal overhead on every call, byte-size estimation
// is only performed every metricsSampleRate-th invocation.
// Must be called while d.mu is held (sampleCounter is not atomic).
func (d *DeltaEncoder) recordDeltaGenerated(prev, curr *UserStateSnapshot, delta *StateDelta) {
	d.metrics.DeltasGenerated.Add(1)
	d.metrics.sampleCounter++

	if d.metrics.sampleCounter >= metricsSampleRate {
		d.metrics.sampleCounter = 0

		fullBytes, _ := json.Marshal(curr)
		deltaBytes, _ := json.Marshal(delta)

		d.metrics.TotalOriginalBytes.Add(int64(len(fullBytes)) * metricsSampleRate)
		d.metrics.TotalDeltaBytes.Add(int64(len(deltaBytes)) * metricsSampleRate)

		if len(fullBytes) > len(deltaBytes) {
			d.metrics.BytesSaved.Add(int64(len(fullBytes)-len(deltaBytes)) * metricsSampleRate)
		}
	}
}

// GetMetrics returns current delta encoding metrics
func (d *DeltaEncoder) GetMetrics() map[string]interface{} {
	totalOriginal := d.metrics.TotalOriginalBytes.Load()
	totalDelta := d.metrics.TotalDeltaBytes.Load()

	compressionRatio := float64(0)
	if totalOriginal > 0 {
		compressionRatio = 1.0 - (float64(totalDelta) / float64(totalOriginal))
	}

	return map[string]interface{}{
		"total_encodings":         d.metrics.TotalEncodings.Load(),
		"full_syncs":              d.metrics.FullSyncs.Load(),
		"deltas_generated":        d.metrics.DeltasGenerated.Load(),
		"bytes_saved":             d.metrics.BytesSaved.Load(),
		"total_original_bytes":    totalOriginal,
		"total_delta_bytes":       totalDelta,
		"delta_compression_ratio": compressionRatio,
	}
}

// NewUserStateSnapshot creates a new user state snapshot
func NewUserStateSnapshot() *UserStateSnapshot {
	return &UserStateSnapshot{
		Positions: make(map[string]*PositionSnapshot),
		Timestamp: time.Now().UnixMilli(),
	}
}

// Clone creates a deep copy of the snapshot
func (s *UserStateSnapshot) Clone() *UserStateSnapshot {
	clone := &UserStateSnapshot{
		Positions: make(map[string]*PositionSnapshot, len(s.Positions)),
		Timestamp: s.Timestamp,
	}

	for k, v := range s.Positions {
		posCopy := *v
		clone.Positions[k] = &posCopy
	}

	if s.Balance != nil {
		balCopy := *s.Balance
		clone.Balance = &balCopy
	}

	return clone
}
