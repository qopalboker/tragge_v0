package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SchedulerHealthChecker interface for the scheduler health status.
type SchedulerHealthChecker interface {
	Health() interface{}
	IsHealthy() bool
}

// CalendarHealthChecker interface for the calendar processor health status.
type CalendarHealthChecker interface {
	Health() interface{}
	IsHealthy() bool
}

// Handler provides health check HTTP handlers.
type Handler struct {
	pool      *db.Pool
	redis     redis.UniversalClient
	scheduler SchedulerHealthChecker
	calendar  CalendarHealthChecker
	logger    *zap.Logger
}

// NewHandler creates a new health check handler.
func NewHandler(pool *db.Pool, redis redis.UniversalClient, scheduler SchedulerHealthChecker, logger *zap.Logger) *Handler {
	return &Handler{
		pool:      pool,
		redis:     redis,
		scheduler: scheduler,
		logger:    logger,
	}
}

// SetCalendarProcessor sets the calendar processor for health checking.
func (h *Handler) SetCalendarProcessor(calendar CalendarHealthChecker) {
	h.calendar = calendar
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status      string      `json:"status"`
	Service     string      `json:"service"`
	Timestamp   string      `json:"timestamp"`
	Database    string      `json:"database,omitempty"`
	Redis       string      `json:"redis,omitempty"`
	Scheduler   interface{} `json:"scheduler,omitempty"`
	Calendar    interface{} `json:"calendar,omitempty"`
	Message     string      `json:"message,omitempty"`
	Environment string      `json:"environment,omitempty"`
}

// HandleHealthz handles liveness probe.
func (h *Handler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleReadyz handles readiness probe.
func (h *Handler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:    "ready",
		Service:   "contest-scheduler",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check scheduler health
	if h.scheduler != nil {
		if !h.scheduler.IsHealthy() {
			response.Status = "unavailable"
			response.Message = "scheduler is not healthy"
			httpStatus = http.StatusServiceUnavailable
		}
		response.Scheduler = h.scheduler.Health()
	}

	// Check calendar processor health (non-critical - informational only)
	if h.calendar != nil {
		response.Calendar = h.calendar.Health()
	}

	// Check database connection (critical)
	if h.pool != nil {
		if err := h.pool.HealthCheck(ctx); err != nil {
			response.Status = "unavailable"
			response.Database = "unavailable"
			response.Message = "database connectivity check failed"
			httpStatus = http.StatusServiceUnavailable
		} else {
			response.Database = "healthy"
		}
	}

	// Check Redis connection (critical for distributed locking)
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			response.Status = "unavailable"
			response.Redis = "unavailable"
			response.Message = "redis connectivity check failed"
			httpStatus = http.StatusServiceUnavailable
		} else {
			response.Redis = "healthy"
		}
	}

	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// HandleSchedulerHealth returns detailed scheduler health status.
func (h *Handler) HandleSchedulerHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.scheduler == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "unavailable",
			"message": "scheduler not initialized",
		})
		return
	}

	status := http.StatusOK
	if !h.scheduler.IsHealthy() {
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(h.scheduler.Health())
}

// HandleCalendarHealth returns detailed calendar processor health status.
func (h *Handler) HandleCalendarHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.calendar == nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "disabled",
			"message": "calendar processor not enabled",
		})
		return
	}

	status := http.StatusOK
	if !h.calendar.IsHealthy() {
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(h.calendar.Health())
}
