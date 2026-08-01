package resilience

import (
	"encoding/json"
	"net/http"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status       string                      `json:"status"`
	Healthy      bool                        `json:"healthy"`
	Dependencies map[string]DependencyStatus `json:"dependencies"`
}

// HandleHealth returns an HTTP handler for health checks
func (r *Resilience) HandleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status := r.GetStatus()
		healthy := r.IsHealthy()

		response := HealthResponse{
			Status:       "ok",
			Healthy:      healthy,
			Dependencies: status,
		}

		w.Header().Set("Content-Type", "application/json")
		if !healthy {
			response.Status = "degraded"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(response)
	}
}

// HandleResetWithAuth returns an HTTP handler for resetting all circuit breakers,
// wrapped with the provided authentication middleware.
func (r *Resilience) HandleResetWithAuth(authMiddleware func(http.Handler) http.Handler) http.Handler {
	return authMiddleware(r.HandleReset())
}

// HandleResetDependencyWithAuth returns an HTTP handler for resetting a specific
// circuit breaker, wrapped with the provided authentication middleware.
func (r *Resilience) HandleResetDependencyWithAuth(authMiddleware func(http.Handler) http.Handler) http.Handler {
	return authMiddleware(r.HandleResetDependency())
}

// HandleReset returns an HTTP handler for resetting all circuit breakers.
// WARNING: This handler has no authentication. Use HandleResetWithAuth() or wrap with auth middleware.
func (r *Resilience) HandleReset() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		r.Reset()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"message": "All circuit breakers reset",
		})
	}
}

// HandleResetDependency returns an HTTP handler for resetting a specific circuit breaker.
// WARNING: This handler has no authentication. Use HandleResetDependencyWithAuth() or wrap with auth middleware.
func (r *Resilience) HandleResetDependency() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		depName := req.URL.Query().Get("name")
		if depName == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "dependency name is required",
			})
			return
		}

		if err := r.ResetDependency(depName); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"message": "Circuit breaker reset: " + depName,
		})
	}
}

// HandleStatus returns an HTTP handler for getting detailed status
func (r *Resilience) HandleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status := r.GetStatus()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(status)
	}
}
