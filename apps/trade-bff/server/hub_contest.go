package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmihailenco/msgpack/v5"
)

// ====================
// Contest Symbol Cache
// ====================

// contestSymbolCacheEntry holds a cached symbol set with TTL expiration.
type contestSymbolCacheEntry struct {
	symbols   map[string]bool
	expiresAt time.Time
}

// contestSymbolCache holds the cached symbol sets per contest with TTL-based expiration.
// A nil symbols value in the entry means "not loaded / query failed → send all symbols".
// An empty map means "loaded, but no symbols enabled for this contest".
type contestSymbolCache struct {
	entries map[string]contestSymbolCacheEntry // contestID → cached entry
	ttl     time.Duration
	mu      sync.RWMutex
}

func newContestSymbolCache(ttl time.Duration) *contestSymbolCache {
	return &contestSymbolCache{
		entries: make(map[string]contestSymbolCacheEntry),
		ttl:     ttl,
	}
}

// get returns the cached symbol set for a contest, and whether it was found (and not expired).
func (c *contestSymbolCache) get(contestID string) (map[string]bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[contestID]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.delete(contestID) // Lazy eviction of expired entry
		return nil, false
	}
	return entry.symbols, true
}

// set stores a symbol set for a contest with TTL expiration.
func (c *contestSymbolCache) set(contestID string, syms map[string]bool) {
	c.mu.Lock()
	c.entries[contestID] = contestSymbolCacheEntry{
		symbols:   syms,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// delete removes a contest's cached symbols.
func (c *contestSymbolCache) delete(contestID string) {
	c.mu.Lock()
	delete(c.entries, contestID)
	c.mu.Unlock()
}

// count returns the number of cached symbols for a contest, or -1 if not cached or expired.
func (c *contestSymbolCache) count(contestID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[contestID]
	if !ok {
		return -1
	}
	if time.Now().After(entry.expiresAt) {
		return -1
	}
	return len(entry.symbols)
}

// ====================
// Contest Asset Class Cache
// ====================

// contestAssetClassEntry holds a cached asset class value with TTL expiration.
type contestAssetClassEntry struct {
	assetClass string
	expiresAt  time.Time
}

// contestAssetClassCache caches the asset_class value for each contest.
type contestAssetClassCache struct {
	entries map[string]contestAssetClassEntry // contestID → cached entry
	ttl     time.Duration
	mu      sync.RWMutex
}

func newContestAssetClassCache(ttl time.Duration) *contestAssetClassCache {
	return &contestAssetClassCache{
		entries: make(map[string]contestAssetClassEntry),
		ttl:     ttl,
	}
}

// get returns the cached asset class for a contest, and whether it was found (and not expired).
func (c *contestAssetClassCache) get(contestID string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[contestID]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		c.delete(contestID)
		return "", false
	}
	return entry.assetClass, true
}

// set stores an asset class for a contest with TTL expiration.
func (c *contestAssetClassCache) set(contestID string, assetClass string) {
	c.mu.Lock()
	c.entries[contestID] = contestAssetClassEntry{
		assetClass: assetClass,
		expiresAt:  time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// delete removes a contest's cached asset class.
func (c *contestAssetClassCache) delete(contestID string) {
	c.mu.Lock()
	delete(c.entries, contestID)
	c.mu.Unlock()
}

// SetApp sets the App reference on the Hub so it can access the database for symbol loading.
// Must be called after App initialization and before the first client connects.
func (h *Hub) SetApp(app *App) {
	h.app = app
}

// loadContestAssetClass loads the asset class for a contest from the database.
// Returns the cached value if available. Returns "mixed" as fallback if the query fails.
func (h *Hub) loadContestAssetClass(contestID string) string {
	if ac, ok := h.contestAssetClasses.get(contestID); ok {
		return ac
	}

	if h.app == nil || h.app.pool == nil {
		return "mixed"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	replica := h.app.pool.Replica()
	if replica == nil {
		return "mixed"
	}

	var assetClass string
	err := replica.QueryRowContext(ctx,
		"SELECT asset_class FROM contests WHERE id = $1",
		contestID,
	).Scan(&assetClass)
	if err != nil {
		log.Printf("Contest asset class cache: failed to query asset_class for contest=%s: %v", contestID, err)
		return "mixed"
	}

	h.contestAssetClasses.set(contestID, assetClass)
	return assetClass
}

// loadContestSymbols loads the allowed symbols for a contest from the database.
// Returns the symbol set from cache if already loaded.
// Returns nil if the query fails (fallback: send all symbols).
func (h *Hub) loadContestSymbols(contestID string) map[string]bool {
	// Check cache first
	if syms, ok := h.contestSymbols.get(contestID); ok {
		return syms
	}

	// No app reference means no database access — return nil (no filter)
	if h.app == nil || h.app.pool == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	replica := h.app.pool.Replica()
	if replica == nil {
		log.Printf("Contest symbol cache: no replica available for contest=%s, falling back to all symbols", contestID)
		return nil
	}

	rows, err := replica.QueryContext(ctx,
		"SELECT symbol FROM contest_symbols WHERE contest_id = $1 AND enabled = true",
		contestID,
	)
	if err != nil {
		log.Printf("Contest symbol cache: failed to query symbols for contest=%s: %v", contestID, err)
		return nil
	}
	defer rows.Close()

	syms := make(map[string]bool)
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			log.Printf("Contest symbol cache: failed to scan symbol for contest=%s: %v", contestID, err)
			return nil
		}
		syms[symbol] = true
	}
	if err := rows.Err(); err != nil {
		log.Printf("Contest symbol cache: row iteration error for contest=%s: %v", contestID, err)
		return nil
	}

	h.contestSymbols.set(contestID, syms)
	log.Printf("Contest symbol cache: loaded %d symbols for contest=%s", len(syms), contestID)
	return syms
}

// InvalidateContestSymbolsCache removes a contest's symbols from the cache,
// forcing a reload on the next access.
func (h *Hub) InvalidateContestSymbolsCache(contestID string) {
	h.contestSymbols.delete(contestID)
	log.Printf("Contest symbol cache: invalidated cache for contest=%s", contestID)
}

// ====================
// Contest-Aware Price Broadcasting
// ====================

// symbolSetKey returns a cache key for a filtered set of symbols.
// Used to deduplicate serialization when multiple contests share the same symbol set.
func symbolSetKey(symbols []contracts.SymbolTick) string {
	if len(symbols) == 1 {
		return symbols[0].Symbol
	}
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Symbol
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// filterSymbolsForContest returns only the symbols that belong to the given contest.
// If allowedSymbols is nil, returns the full list (no filter / graceful degradation).
func filterSymbolsForContest(symbols []contracts.SymbolTick, allowedSymbols map[string]bool) []contracts.SymbolTick {
	if allowedSymbols == nil {
		return symbols
	}
	filtered := make([]contracts.SymbolTick, 0, len(allowedSymbols))
	for _, tick := range symbols {
		if allowedSymbols[tick.Symbol] {
			filtered = append(filtered, tick)
		}
	}
	return filtered
}

// broadcastBatchedUpdatesContestAware sends batched tick updates filtered per contest.
// Each contest only receives the symbols that belong to it.
// The sequence number comes from the global batcher and is shared across all contests.
func (h *Hub) broadcastBatchedUpdatesContestAware() {
	start := time.Now()

	// Get latest prices and add to batcher
	snapshot := h.priceBook.GetSnapshot(h.maxSymbolsPerTick)
	if len(snapshot.Symbols) == 0 {
		return
	}

	// Add ticks to batcher buffer
	h.batcher.AddTickSnapshot(snapshot)

	// Flush the batch — this creates a single batched message with a global sequence number
	tickBatch := h.batcher.FlushTicks()
	if tickBatch == nil {
		return
	}

	// Get all active contests
	h.contestClientsMu.RLock()
	numContests := len(h.contestClients)

	if numContests == 0 {
		h.contestClientsMu.RUnlock()
		return
	}

	// Snapshot the contest → clients mapping
	type contestEntry struct {
		contestID string
		clients   []*Client
	}
	contests := make([]contestEntry, 0, numContests)
	for contestID, clientSet := range h.contestClients {
		clients := make([]*Client, 0, len(clientSet))
		for client := range clientSet {
			clients = append(clients, client)
		}
		contests = append(contests, contestEntry{contestID: contestID, clients: clients})
	}
	h.contestClientsMu.RUnlock()

	totalClients := 0

	// Extract the full symbol list from the batched data
	batchData, ok := tickBatch.Data.(TickBatchData)
	if !ok {
		log.Printf("broadcastBatchedUpdatesContestAware: unexpected batch data type")
		return
	}
	allSymbols := batchData.Symbols

	// Optimization: single contest path — avoid per-contest overhead
	if len(contests) == 1 {
		entry := contests[0]
		allowedSymbols := h.loadContestSymbols(entry.contestID)
		filtered := filterSymbolsForContest(allSymbols, allowedSymbols)
		if len(filtered) == 0 {
			return
		}

		contestBatch := &BatchedMessage{
			Type:     tickBatch.Type,
			Sequence: tickBatch.Sequence,
			Count:    len(filtered),
			Data:     TickBatchData{Symbols: filtered},
			Ts:       tickBatch.Ts,
		}

		jsonData, err := json.Marshal(contestBatch)
		if err != nil {
			log.Printf("Failed to marshal tick batch to JSON for contest=%s: %v", entry.contestID, err)
			return
		}
		msgpackData, err := msgpack.Marshal(contestBatch)
		if err != nil {
			log.Printf("Failed to marshal tick batch to MessagePack for contest=%s: %v", entry.contestID, err)
			msgpackData = nil
		}

		totalClients = len(entry.clients)
		if h.workerPool != nil && totalClients > 10 {
			h.workerPool.BroadcastTickBatchToAll(entry.clients, jsonData, msgpackData)
		} else {
			for _, client := range entry.clients {
				client.SendTickBatch(jsonData, msgpackData)
			}
		}
	} else {
		// Multiple contests: filter per contest, deduplicate serialization for identical symbol sets
		type serializedPayload struct {
			jsonData    []byte
			msgpackData []byte
		}
		cache := make(map[string]*serializedPayload)

		for _, entry := range contests {
			allowedSymbols := h.loadContestSymbols(entry.contestID)
			filtered := filterSymbolsForContest(allSymbols, allowedSymbols)
			if len(filtered) == 0 {
				continue
			}

			key := symbolSetKey(filtered)
			cached, ok := cache[key]
			if !ok {
				contestBatch := &BatchedMessage{
					Type:     tickBatch.Type,
					Sequence: tickBatch.Sequence,
					Count:    len(filtered),
					Data:     TickBatchData{Symbols: filtered},
					Ts:       tickBatch.Ts,
				}

				jsonData, err := json.Marshal(contestBatch)
				if err != nil {
					log.Printf("Failed to marshal tick batch to JSON for contest=%s: %v", entry.contestID, err)
					continue
				}
				msgpackData, err := msgpack.Marshal(contestBatch)
				if err != nil {
					log.Printf("Failed to marshal tick batch to MessagePack for contest=%s: %v", entry.contestID, err)
					msgpackData = nil
				}

				cached = &serializedPayload{jsonData: jsonData, msgpackData: msgpackData}
				cache[key] = cached
			}

			totalClients += len(entry.clients)
			if h.workerPool != nil && len(entry.clients) > 10 {
				h.workerPool.BroadcastTickBatchToAll(entry.clients, cached.jsonData, cached.msgpackData)
			} else {
				for _, client := range entry.clients {
					client.SendTickBatch(cached.jsonData, cached.msgpackData)
				}
			}
		}
	}

	elapsed := time.Since(start).Milliseconds()
	h.metrics.RecordBroadcast(elapsed)

	// Track bandwidth savings
	h.metrics.wsBandwidthSaved.Add(int64(len(snapshot.Symbols)-1) * 50)

	if totalClients > 0 && elapsed > 50 {
		log.Printf("Broadcast tick_batch seq=%d to %d clients across %d contests in %dms (symbols=%d, workers=%d)",
			tickBatch.Sequence, totalClients, len(contests), elapsed, len(allSymbols), h.broadcastWorkers)
	}
}

// sendFullSyncToClientFiltered sends initial state to a newly connected client,
// filtered to only include symbols belonging to the client's contest.
func (h *Hub) sendFullSyncToClientFiltered(client *Client) {
	snapshot := h.priceBook.GetSnapshot(h.maxSymbolsPerTick)
	if len(snapshot.Symbols) == 0 {
		return
	}

	// Filter symbols for the client's contest
	allowedSymbols := h.loadContestSymbols(client.contestID)
	filtered := filterSymbolsForContest(snapshot.Symbols, allowedSymbols)
	if len(filtered) == 0 {
		return
	}

	initialBatch := &BatchedMessage{
		Type:     "tick_batch",
		Sequence: h.batcher.GetSequence(),
		Count:    len(filtered),
		Data:     TickBatchData{Symbols: filtered},
		Ts:       snapshot.Ts,
	}

	jsonData, err := json.Marshal(initialBatch)
	if err != nil {
		log.Printf("Failed to marshal initial sync to JSON: %v", err)
		return
	}

	var msgpackData []byte
	if client.encoding == EncodingMsgPack {
		msgpackData, err = msgpack.Marshal(initialBatch)
		if err != nil {
			log.Printf("Failed to marshal initial sync to MessagePack: %v", err)
			msgpackData = nil
		}
	}

	client.SendTickBatch(jsonData, msgpackData)
	encoding := "json"
	if client.encoding == EncodingMsgPack {
		encoding = "msgpack"
	}
	log.Printf("Sent initial sync to user=%s contest=%s (symbols=%d/%d, seq=%d, encoding=%s)",
		client.userID, client.contestID, len(filtered), len(snapshot.Symbols), initialBatch.Sequence, encoding)
}

// ====================
// Admin/Debug Endpoint
// ====================

// HubStatusResponse is the JSON response for GET /admin/hub/status.
type HubStatusResponse struct {
	TotalConnections  int64                        `json:"total_connections"`
	ActiveContests    int                          `json:"active_contests"`
	BroadcastWorkers  int                          `json:"broadcast_workers"`
	BroadcastInterval string                       `json:"broadcast_interval"`
	Contests          map[string]ContestStatusEntry `json:"contests"`
}

// ContestStatusEntry holds per-contest status info.
type ContestStatusEntry struct {
	ClientCount      int `json:"client_count"`
	CachedSymbolCount int `json:"cached_symbol_count"` // -1 if not cached
}

// GetHubStatus returns the current Hub status for the admin endpoint.
func (h *Hub) GetHubStatus() *HubStatusResponse {
	h.contestClientsMu.RLock()
	contests := make(map[string]ContestStatusEntry, len(h.contestClients))
	for contestID, clientSet := range h.contestClients {
		contests[contestID] = ContestStatusEntry{
			ClientCount:      len(clientSet),
			CachedSymbolCount: h.contestSymbols.count(contestID),
		}
	}
	h.contestClientsMu.RUnlock()

	return &HubStatusResponse{
		TotalConnections:  h.metrics.wsConnections.Load(),
		ActiveContests:    len(contests),
		BroadcastWorkers:  h.broadcastWorkers,
		BroadcastInterval: h.broadcastInterval.String(),
		Contests:          contests,
	}
}

// handleHubStatus handles GET /admin/hub/status.
func (a *App) handleHubStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.hub.GetHubStatus())
}

// ====================
// Prometheus Per-Contest Metrics
// ====================

// ContestMetrics holds Prometheus metrics for per-contest monitoring.
type ContestMetrics struct {
	wsConnectionsPerContest *prometheus.GaugeVec
}

// NewContestMetrics creates and returns a new ContestMetrics.
// The caller must register the metrics with the Prometheus registry.
func NewContestMetrics() *ContestMetrics {
	return &ContestMetrics{
		wsConnectionsPerContest: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "tragge",
				Subsystem: "trade_bff",
				Name:      "ws_connections_per_contest",
				Help:      "Number of active WebSocket connections per contest",
			},
			[]string{"contest_id"},
		),
	}
}

// Collectors returns the prometheus.Collector instances for registration.
func (cm *ContestMetrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{cm.wsConnectionsPerContest}
}

// Inc increments the connection count for a contest.
func (cm *ContestMetrics) Inc(contestID string) {
	if cm == nil || cm.wsConnectionsPerContest == nil {
		return
	}
	cm.wsConnectionsPerContest.WithLabelValues(contestID).Inc()
}

// Dec decrements the connection count for a contest.
func (cm *ContestMetrics) Dec(contestID string) {
	if cm == nil || cm.wsConnectionsPerContest == nil {
		return
	}
	cm.wsConnectionsPerContest.WithLabelValues(contestID).Dec()
}

// DeleteContest removes the metric series for a contest (when last client leaves).
func (cm *ContestMetrics) DeleteContest(contestID string) {
	if cm == nil || cm.wsConnectionsPerContest == nil {
		return
	}
	cm.wsConnectionsPerContest.DeleteLabelValues(contestID)
}

