package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/go-chi/chi/v5"
)

// DrawingsPayload is the request/response body for chart drawings.
type DrawingsPayload struct {
	Drawings json.RawMessage `json:"drawings"`
}

// handleGetDrawings returns saved chart drawings for a user/contest/symbol.
// GET /api/trade/drawings/{contest_id}?symbol=...
func (a *App) handleGetDrawings(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	contestID := chi.URLParam(r, "contest_id")
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.SymbolParamRequired})
		return
	}

	var drawings json.RawMessage
	err := a.pool.Replica().QueryRowContext(r.Context(),
		`SELECT drawings FROM chart_drawings
		 WHERE user_id = $1 AND contest_id = $2 AND symbol = $3`,
		userID, contestID, symbol,
	).Scan(&drawings)

	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, DrawingsPayload{
				Drawings: json.RawMessage("[]"),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": tradeMsg.DrawingsLoadFailed})
		return
	}

	writeJSON(w, http.StatusOK, DrawingsPayload{Drawings: drawings})
}

// handleSaveDrawings persists chart drawings for a user/contest/symbol.
// PUT /api/trade/drawings/{contest_id}?symbol=...
func (a *App) handleSaveDrawings(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	contestID := chi.URLParam(r, "contest_id")
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.SymbolParamRequired})
		return
	}

	var payload DrawingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.InvalidJSON})
		return
	}

	// Validate size (max 500KB per symbol)
	if len(payload.Drawings) > 512_000 {
		writeJSON(w, http.StatusRequestEntityTooLarge,
			map[string]string{"error": tradeMsg.DrawingsPayloadLarge})
		return
	}

	_, err := a.pool.Primary().ExecContext(r.Context(), `
		INSERT INTO chart_drawings (user_id, contest_id, symbol, drawings, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, contest_id, symbol)
		DO UPDATE SET drawings = $4, updated_at = NOW()
	`, userID, contestID, symbol, payload.Drawings)

	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": tradeMsg.DrawingsSaveFailed})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": tradeMsg.DrawingsSaved})
}

// handleDeleteDrawings removes chart drawings for a user/contest/symbol.
// DELETE /api/trade/drawings/{contest_id}?symbol=...
func (a *App) handleDeleteDrawings(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	contestID := chi.URLParam(r, "contest_id")
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.SymbolParamRequired})
		return
	}

	_, _ = a.pool.Primary().ExecContext(r.Context(), `
		DELETE FROM chart_drawings
		WHERE user_id = $1 AND contest_id = $2 AND symbol = $3
	`, userID, contestID, symbol)

	writeJSON(w, http.StatusOK, map[string]string{"status": tradeMsg.DrawingsDeleted})
}
