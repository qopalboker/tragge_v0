package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Handlers contains all HTTP handlers for the shard-router service.
type Handlers struct {
	router       *ShardRouter
	cachedRouter *CachedRouter
	cache        *ShardCache
	db           *db.Pool
	logger       *zap.Logger
	notifier     *Notifier
	circuits     *CircuitBreakers
	isDev        bool

	// Cached readiness check result
	readyCacheMu    sync.RWMutex
	lastReadyResult []byte
	lastReadyStatus int
	lastReadyCheck  time.Time
	readyCacheTTL   time.Duration
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(
	router *ShardRouter,
	cachedRouter *CachedRouter,
	cache *ShardCache,
	db *db.Pool,
	logger *zap.Logger,
	notifier *Notifier,
	circuits *CircuitBreakers,
	isDev bool,
) *Handlers {
	return &Handlers{
		router:        router,
		cachedRouter:  cachedRouter,
		cache:         cache,
		db:            db,
		logger:        logger.With(zap.String("component", "handlers")),
		notifier:      notifier,
		circuits:      circuits,
		isDev:         isDev,
		readyCacheTTL: 5 * time.Second,
	}
}

// HealthzHandler handles liveness probe requests.
func (h *Handlers) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ReadyzHandler handles readiness probe requests.
// Results are cached for 5 seconds to reduce I/O overhead from frequent k8s probes.
func (h *Handlers) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	// Check cache first
	h.readyCacheMu.RLock()
	if time.Since(h.lastReadyCheck) < h.readyCacheTTL && h.lastReadyResult != nil {
		result := h.lastReadyResult
		status := h.lastReadyStatus
		h.readyCacheMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(result)
		return
	}
	h.readyCacheMu.RUnlock()

	// Perform actual health checks
	statusCode, responseBytes := h.performReadyCheck(r)

	// Cache the result
	h.readyCacheMu.Lock()
	h.lastReadyResult = responseBytes
	h.lastReadyStatus = statusCode
	h.lastReadyCheck = time.Now()
	h.readyCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(responseBytes)
}

// performReadyCheck executes the actual readiness checks and returns status code and response body.
func (h *Handlers) performReadyCheck(r *http.Request) (int, []byte) {
	ctx := r.Context()

	// Check circuit breaker health first
	if !h.circuits.IsHealthy() {
		h.logger.Warn("circuit breakers unhealthy")
		resp, _ := json.Marshal(map[string]interface{}{
			"status":   "not ready",
			"reason":   "circuit breakers unhealthy",
			"circuits": h.circuits.GetHealth(),
		})
		return http.StatusServiceUnavailable, resp
	}

	// Check database connection
	if err := h.db.HealthCheck(ctx); err != nil {
		h.logger.Warn("database health check failed", zap.Error(err))
		resp, _ := json.Marshal(ErrorResponse{Error: "database not ready"})
		return http.StatusServiceUnavailable, resp
	}

	// Check Redis connection
	if err := h.cache.HealthCheck(ctx); err != nil {
		h.logger.Warn("cache health check failed", zap.Error(err))
		resp, _ := json.Marshal(ErrorResponse{Error: "cache not ready"})
		return http.StatusServiceUnavailable, resp
	}

	// Check that we have at least one shard
	if !h.router.IsHealthy() {
		h.logger.Warn("no active shards available")
		// Send critical alert
		if h.notifier != nil {
			h.notifier.sendNoHealthyShardsAvailable(len(h.router.ListShards()))
		}
		resp, _ := json.Marshal(ErrorResponse{Error: "no active shards"})
		return http.StatusServiceUnavailable, resp
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"status":        "ready",
		"active_shards": h.router.ShardCount(),
		"cache":         h.cache.Stats(ctx),
	})
	return http.StatusOK, resp
}

// CircuitHealthHandler returns the health status of all circuit breakers.
func (h *Handlers) CircuitHealthHandler(w http.ResponseWriter, r *http.Request) {
	health := h.circuits.GetHealth()

	status := http.StatusOK
	if health.Overall == "unhealthy" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

// GetShardHandler handles requests to get the shard for a contest.
func (h *Handlers) GetShardHandler(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "contestID")
	if contestID == "" {
		h.respondError(w, http.StatusBadRequest, "contestID is required", nil)
		return
	}

	ctx := r.Context()

	// Extract user ID from header if present (for error tracking)
	userID := r.Header.Get("X-User-ID")

	assignment, err := h.cachedRouter.RouteTo(ctx, contestID)
	if err != nil {
		// Send routing error notification
		if h.notifier != nil {
			h.notifier.sendRoutingError(userID, contestID, err)
		}

		switch err {
		case ErrNoShardsAvailable:
			// Critical: no shards available for routing
			if h.notifier != nil {
				h.notifier.sendNoHealthyShardsAvailable(len(h.router.ListShards()))
			}
			h.respondError(w, http.StatusServiceUnavailable, "no shards available", err)
		case ErrShardNotFound:
			h.respondError(w, http.StatusNotFound, "shard not found", err)
		default:
			h.logger.Error("failed to route contest",
				zap.String("contest_id", contestID),
				zap.Error(err),
			)
			h.respondError(w, http.StatusInternalServerError, "internal error", err)
		}
		return
	}

	h.respondJSON(w, http.StatusOK, assignment)
}

// ListShardsResponse represents the response for listing shards.
type ListShardsResponse struct {
	Shards      []*Shard `json:"shards"`
	TotalCount  int      `json:"total_count"`
	ActiveCount int      `json:"active_count"`
}

// ListShardsHandler handles requests to list all shards.
func (h *Handlers) ListShardsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Try cache first
	shards, err := h.cache.GetShardList(ctx)
	if err != nil {
		h.logger.Warn("failed to get shard list from cache", zap.Error(err))
	}

	if shards == nil {
		// Cache miss, get from router
		shards = h.router.ListShards()

		// Cache the result
		if cacheErr := h.cache.SetShardList(ctx, shards); cacheErr != nil {
			h.logger.Warn("failed to cache shard list", zap.Error(cacheErr))
		}
	}

	// Sort by shard ID
	sort.Slice(shards, func(i, j int) bool {
		return shards[i].ID < shards[j].ID
	})

	// Count active shards
	activeCount := 0
	for _, shard := range shards {
		if shard.Status == ShardStatusActive {
			activeCount++
		}
	}

	response := ListShardsResponse{
		Shards:      shards,
		TotalCount:  len(shards),
		ActiveCount: activeCount,
	}

	h.respondJSON(w, http.StatusOK, response)
}

// DrainShardHandler handles requests to drain a shard.
func (h *Handlers) DrainShardHandler(w http.ResponseWriter, r *http.Request) {
	shardID := chi.URLParam(r, "shardID")
	if shardID == "" {
		h.respondError(w, http.StatusBadRequest, "shardID is required", nil)
		return
	}

	ctx := r.Context()

	// Use primary for writes
	err := h.router.DrainShard(ctx, h.db.Primary(), shardID)
	if err != nil {
		switch err {
		case ErrShardNotFound:
			h.respondError(w, http.StatusNotFound, "shard not found", err)
		case ErrShardNotActive:
			h.respondError(w, http.StatusConflict, "shard is not active", err)
		default:
			h.logger.Error("failed to drain shard",
				zap.String("shard_id", shardID),
				zap.Error(err),
			)
			h.respondError(w, http.StatusInternalServerError, "internal error", err)
		}
		return
	}

	// Invalidate caches
	if err := h.cachedRouter.InvalidateAll(ctx); err != nil {
		h.logger.Warn("failed to invalidate caches after draining shard",
			zap.String("shard_id", shardID),
			zap.Error(err),
		)
	}

	h.logger.Info("shard drained",
		zap.String("shard_id", shardID),
	)

	// Send failover notification - traffic will be redistributed to other shards
	if h.notifier != nil {
		h.notifier.sendShardFailover(shardID, "redistributed", "Shard marked for draining, traffic redistributed to remaining shards")
	}

	response := map[string]interface{}{
		"status":   "draining",
		"shard_id": shardID,
		"message":  "shard has been marked for draining and removed from the hash ring",
	}

	h.respondJSON(w, http.StatusOK, response)
}

// AddShardRequest represents a request to add a new shard.
type AddShardRequest struct {
	ShardID string `json:"shard_id"`
	Address string `json:"shard_address"`
	Weight  int    `json:"weight,omitempty"`
}

// AddShardHandler handles requests to add a new shard.
func (h *Handlers) AddShardHandler(w http.ResponseWriter, r *http.Request) {
	var req AddShardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.ShardID == "" {
		h.respondError(w, http.StatusBadRequest, "shard_id is required", nil)
		return
	}
	if req.Address == "" {
		h.respondError(w, http.StatusBadRequest, "shard_address is required", nil)
		return
	}

	// Default weight to 1
	weight := req.Weight
	if weight <= 0 {
		weight = 1
	}

	ctx := r.Context()

	shard := &Shard{
		ID:      req.ShardID,
		Address: req.Address,
		Status:  ShardStatusActive,
		Weight:  weight,
	}

	err := h.router.AddShard(ctx, h.db.Primary(), shard)
	if err != nil {
		switch err {
		case ErrShardAlreadyExists:
			h.respondError(w, http.StatusConflict, "shard already exists", err)
		default:
			h.logger.Error("failed to add shard",
				zap.String("shard_id", req.ShardID),
				zap.Error(err),
			)
			h.respondError(w, http.StatusInternalServerError, "internal error", err)
		}
		return
	}

	// Invalidate shard list cache
	if err := h.cache.InvalidateShardList(ctx); err != nil {
		h.logger.Warn("failed to invalidate shard list cache",
			zap.Error(err),
		)
	}

	h.logger.Info("shard added",
		zap.String("shard_id", req.ShardID),
		zap.String("address", req.Address),
		zap.Int("weight", weight),
	)

	// Send notification about new shard
	if h.notifier != nil {
		h.notifier.sendShardAdded(req.ShardID, req.Address, len(h.router.ListShards()))
	}

	h.respondJSON(w, http.StatusCreated, shard)
}

// RemoveShardHandler handles requests to remove a shard.
func (h *Handlers) RemoveShardHandler(w http.ResponseWriter, r *http.Request) {
	shardID := chi.URLParam(r, "shardID")
	if shardID == "" {
		h.respondError(w, http.StatusBadRequest, "shardID is required", nil)
		return
	}

	ctx := r.Context()

	err := h.router.RemoveShard(ctx, h.db.Primary(), shardID)
	if err != nil {
		switch err {
		case ErrShardNotFound:
			h.respondError(w, http.StatusNotFound, "shard not found", err)
		default:
			h.logger.Error("failed to remove shard",
				zap.String("shard_id", shardID),
				zap.Error(err),
			)
			h.respondError(w, http.StatusInternalServerError, "internal error", err)
		}
		return
	}

	// Invalidate all caches
	if err := h.cachedRouter.InvalidateAll(ctx); err != nil {
		h.logger.Warn("failed to invalidate caches after removing shard",
			zap.String("shard_id", shardID),
			zap.Error(err),
		)
	}

	h.logger.Info("shard removed",
		zap.String("shard_id", shardID),
	)

	w.WriteHeader(http.StatusNoContent)
}

// GetShardInfoHandler handles requests to get information about a specific shard.
func (h *Handlers) GetShardInfoHandler(w http.ResponseWriter, r *http.Request) {
	shardID := chi.URLParam(r, "shardID")
	if shardID == "" {
		h.respondError(w, http.StatusBadRequest, "shardID is required", nil)
		return
	}

	ctx := r.Context()

	// Try cache first
	shard, err := h.cache.GetShardInfo(ctx, shardID)
	if err != nil {
		h.logger.Warn("failed to get shard info from cache",
			zap.String("shard_id", shardID),
			zap.Error(err),
		)
	}

	if shard == nil {
		// Cache miss, get from router
		shard, err = h.router.GetShard(shardID)
		if err != nil {
			switch err {
			case ErrShardNotFound:
				h.respondError(w, http.StatusNotFound, "shard not found", err)
			default:
				h.respondError(w, http.StatusInternalServerError, "internal error", err)
			}
			return
		}

		// Cache the result
		if cacheErr := h.cache.SetShardInfo(ctx, shard); cacheErr != nil {
			h.logger.Warn("failed to cache shard info",
				zap.String("shard_id", shardID),
				zap.Error(cacheErr),
			)
		}
	}

	h.respondJSON(w, http.StatusOK, shard)
}

// respondJSON sends a JSON response.
func (h *Handlers) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", zap.Error(err))
	}
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// respondError sends an error response.
// Internal error details are only included in development mode to prevent information leakage.
func (h *Handlers) respondError(w http.ResponseWriter, status int, message string, err error) {
	response := ErrorResponse{
		Error: message,
	}
	if err != nil {
		h.logger.Error("request error",
			zap.Int("status", status),
			zap.String("message", message),
			zap.Error(err))
		if h.isDev {
			response.Details = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}
