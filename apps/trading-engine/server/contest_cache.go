package server

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ContestCacheConfig holds configuration for the contest cache.
type ContestCacheConfig struct {
	TTL             time.Duration // How long entries stay valid (default: 30s)
	CleanupInterval time.Duration // How often to run eviction (default: 60s)
}

// DefaultContestCacheConfig returns the default contest cache configuration.
func DefaultContestCacheConfig() ContestCacheConfig {
	return ContestCacheConfig{
		TTL:             30 * time.Second,
		CleanupInterval: 60 * time.Second,
	}
}

// CacheStats holds cache hit/miss statistics for Prometheus metrics.
type CacheStats struct {
	Hits    uint64
	Misses  uint64
	Entries int
}

// contestCacheEntry wraps a cached DBContest with its expiration time.
type contestCacheEntry struct {
	contest   *DBContest
	expiresAt time.Time
}

// ContestCache provides an in-memory cache for contest data keyed by contest ID.
// It is safe for concurrent access from multiple goroutines.
type ContestCache struct {
	entries map[string]*contestCacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	hits    atomic.Uint64
	misses  atomic.Uint64
	stopCh  chan struct{}
}

// NewContestCache creates a new ContestCache and starts a background cleanup goroutine.
func NewContestCache(cfg ContestCacheConfig) *ContestCache {
	cc := &ContestCache{
		entries: make(map[string]*contestCacheEntry),
		ttl:     cfg.TTL,
		stopCh:  make(chan struct{}),
	}

	go cc.cleanupLoop(cfg.CleanupInterval)

	return cc
}

// Get returns a cached contest and whether it was a cache hit.
// It does NOT perform a database lookup on miss — the caller decides what to do.
// Expired entries are treated as misses and lazily removed.
func (cc *ContestCache) Get(contestID string) (*DBContest, bool) {
	cc.mu.RLock()
	entry, exists := cc.entries[contestID]
	cc.mu.RUnlock()

	if !exists {
		cc.misses.Add(1)
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Expired — lazily remove and report miss
		cc.mu.Lock()
		// Double-check: another goroutine may have already replaced the entry
		if e, ok := cc.entries[contestID]; ok && time.Now().After(e.expiresAt) {
			delete(cc.entries, contestID)
		}
		cc.mu.Unlock()
		cc.misses.Add(1)
		return nil, false
	}

	cc.hits.Add(1)
	// Return a copy to prevent callers from mutating cached data
	contestCopy := *entry.contest
	if entry.contest.Rules != nil {
		rulesCopy := *entry.contest.Rules
		contestCopy.Rules = &rulesCopy
	}
	return &contestCopy, true
}

// Set stores a contest in the cache with the configured TTL.
func (cc *ContestCache) Set(contestID string, contest *DBContest) {
	// Store a copy so external mutations don't affect cached data
	contestCopy := *contest
	if contest.Rules != nil {
		rulesCopy := *contest.Rules
		contestCopy.Rules = &rulesCopy
	}

	cc.mu.Lock()
	cc.entries[contestID] = &contestCacheEntry{
		contest:   &contestCopy,
		expiresAt: time.Now().Add(cc.ttl),
	}
	cc.mu.Unlock()
}

// Invalidate removes a specific contest from the cache.
// Used when a contest state change event arrives.
func (cc *ContestCache) Invalidate(contestID string) {
	cc.mu.Lock()
	delete(cc.entries, contestID)
	cc.mu.Unlock()
}

// InvalidateAll clears the entire cache.
// Used for emergency or admin operations.
func (cc *ContestCache) InvalidateAll() {
	cc.mu.Lock()
	cc.entries = make(map[string]*contestCacheEntry)
	cc.mu.Unlock()
}

// Stats returns current cache statistics for Prometheus metrics.
func (cc *ContestCache) Stats() CacheStats {
	cc.mu.RLock()
	entryCount := len(cc.entries)
	cc.mu.RUnlock()

	return CacheStats{
		Hits:    cc.hits.Load(),
		Misses:  cc.misses.Load(),
		Entries: entryCount,
	}
}

// Stop signals the background cleanup goroutine to exit.
func (cc *ContestCache) Stop() {
	close(cc.stopCh)
}

// cleanupLoop periodically evicts expired entries to prevent memory leaks
// from contests that have ended and are no longer queried.
func (cc *ContestCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cc.evictExpired()
		case <-cc.stopCh:
			return
		}
	}
}

// evictExpired removes all entries that have passed their expiration time.
func (cc *ContestCache) evictExpired() {
	now := time.Now()

	cc.mu.Lock()
	for id, entry := range cc.entries {
		if now.After(entry.expiresAt) {
			delete(cc.entries, id)
		}
	}
	cc.mu.Unlock()
}

// ---------------------------------------------------------------------------
// ParticipantCache — in-memory cache for participant validation
// ---------------------------------------------------------------------------

// ParticipantCacheConfig holds configuration for the participant cache.
type ParticipantCacheConfig struct {
	TTL             time.Duration // How long entries stay valid (default: 60s)
	CleanupInterval time.Duration // How often to run eviction (default: 120s)
}

// DefaultParticipantCacheConfig returns the default participant cache configuration.
func DefaultParticipantCacheConfig() ParticipantCacheConfig {
	return ParticipantCacheConfig{
		TTL:             60 * time.Second,
		CleanupInterval: 120 * time.Second,
	}
}

// participantCacheKey builds the composite key "contestID:userID".
func participantCacheKey(contestID, userID string) string {
	return contestID + ":" + userID
}

// participantCacheEntry wraps a cached DBParticipant with its expiration time.
type participantCacheEntry struct {
	participant *DBParticipant
	expiresAt   time.Time
}

// ParticipantCache provides an in-memory cache for participant data keyed by
// the composite key contestID:userID. It is safe for concurrent access.
type ParticipantCache struct {
	entries map[string]*participantCacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	hits    atomic.Uint64
	misses  atomic.Uint64
	stopCh  chan struct{}
}

// NewParticipantCache creates a new ParticipantCache and starts a background
// cleanup goroutine.
func NewParticipantCache(cfg ParticipantCacheConfig) *ParticipantCache {
	pc := &ParticipantCache{
		entries: make(map[string]*participantCacheEntry),
		ttl:     cfg.TTL,
		stopCh:  make(chan struct{}),
	}

	go pc.cleanupLoop(cfg.CleanupInterval)

	return pc
}

// Get returns a cached participant and whether it was a cache hit.
// Expired entries are treated as misses and lazily removed.
func (pc *ParticipantCache) Get(contestID, userID string) (*DBParticipant, bool) {
	key := participantCacheKey(contestID, userID)

	pc.mu.RLock()
	entry, exists := pc.entries[key]
	pc.mu.RUnlock()

	if !exists {
		pc.misses.Add(1)
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Expired — lazily remove and report miss
		pc.mu.Lock()
		if e, ok := pc.entries[key]; ok && time.Now().After(e.expiresAt) {
			delete(pc.entries, key)
		}
		pc.mu.Unlock()
		pc.misses.Add(1)
		return nil, false
	}

	pc.hits.Add(1)
	// Return a copy to prevent callers from mutating cached data
	pCopy := *entry.participant
	return &pCopy, true
}

// Set stores a participant in the cache with the configured TTL.
func (pc *ParticipantCache) Set(contestID, userID string, participant *DBParticipant) {
	key := participantCacheKey(contestID, userID)

	// Store a copy so external mutations don't affect cached data
	pCopy := *participant

	pc.mu.Lock()
	pc.entries[key] = &participantCacheEntry{
		participant: &pCopy,
		expiresAt:   time.Now().Add(pc.ttl),
	}
	pc.mu.Unlock()
}

// Invalidate removes cache entries. When called with both contestID and userID
// it removes the single entry. When called with only contestID (userID == ""),
// it removes ALL participants for that contest.
func (pc *ParticipantCache) Invalidate(contestID, userID string) {
	if userID != "" {
		// Single entry invalidation
		key := participantCacheKey(contestID, userID)
		pc.mu.Lock()
		delete(pc.entries, key)
		pc.mu.Unlock()
		return
	}

	// Invalidate all participants for a contest
	prefix := contestID + ":"
	pc.mu.Lock()
	for key := range pc.entries {
		if strings.HasPrefix(key, prefix) {
			delete(pc.entries, key)
		}
	}
	pc.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (pc *ParticipantCache) InvalidateAll() {
	pc.mu.Lock()
	pc.entries = make(map[string]*participantCacheEntry)
	pc.mu.Unlock()
}

// Stats returns current cache statistics for Prometheus metrics.
func (pc *ParticipantCache) Stats() CacheStats {
	pc.mu.RLock()
	entryCount := len(pc.entries)
	pc.mu.RUnlock()

	return CacheStats{
		Hits:    pc.hits.Load(),
		Misses:  pc.misses.Load(),
		Entries: entryCount,
	}
}

// Stop signals the background cleanup goroutine to exit.
func (pc *ParticipantCache) Stop() {
	close(pc.stopCh)
}

// cleanupLoop periodically evicts expired entries.
func (pc *ParticipantCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pc.evictExpired()
		case <-pc.stopCh:
			return
		}
	}
}

// evictExpired removes all entries that have passed their expiration time.
func (pc *ParticipantCache) evictExpired() {
	now := time.Now()

	pc.mu.Lock()
	for key, entry := range pc.entries {
		if now.After(entry.expiresAt) {
			delete(pc.entries, key)
		}
	}
	pc.mu.Unlock()
}

// ---------------------------------------------------------------------------
// SymbolCache — in-memory cache for contest symbol validation (P0-1)
// ---------------------------------------------------------------------------

// symbolCacheEntry wraps cached symbol set with its expiration time.
type symbolCacheEntry struct {
	symbols   map[string]bool
	expiresAt time.Time
}

// SymbolCache provides an in-memory cache for contest symbols keyed by contest ID.
// It is safe for concurrent access.
type SymbolCache struct {
	entries map[string]*symbolCacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	stopCh  chan struct{}
}

// NewSymbolCache creates a new SymbolCache and starts a background cleanup goroutine.
func NewSymbolCache(ttl, cleanupInterval time.Duration) *SymbolCache {
	sc := &SymbolCache{
		entries: make(map[string]*symbolCacheEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go sc.cleanupLoop(cleanupInterval)
	return sc
}

// Get returns the cached symbol set for a contest and whether it was a cache hit.
func (sc *SymbolCache) Get(contestID string) (map[string]bool, bool) {
	sc.mu.RLock()
	entry, exists := sc.entries[contestID]
	sc.mu.RUnlock()

	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.symbols, true
}

// Set stores a symbol set in the cache.
func (sc *SymbolCache) Set(contestID string, symbols map[string]bool) {
	// Store a copy
	symbolsCopy := make(map[string]bool, len(symbols))
	for k, v := range symbols {
		symbolsCopy[k] = v
	}

	sc.mu.Lock()
	sc.entries[contestID] = &symbolCacheEntry{
		symbols:   symbolsCopy,
		expiresAt: time.Now().Add(sc.ttl),
	}
	sc.mu.Unlock()
}

// Invalidate removes a contest's symbol cache entry.
func (sc *SymbolCache) Invalidate(contestID string) {
	sc.mu.Lock()
	delete(sc.entries, contestID)
	sc.mu.Unlock()
}

// Stop signals the background cleanup goroutine to exit.
func (sc *SymbolCache) Stop() {
	close(sc.stopCh)
}

func (sc *SymbolCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			sc.mu.Lock()
			for id, entry := range sc.entries {
				if now.After(entry.expiresAt) {
					delete(sc.entries, id)
				}
			}
			sc.mu.Unlock()
		case <-sc.stopCh:
			return
		}
	}
}
