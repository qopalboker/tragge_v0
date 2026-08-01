package server

import (
	"encoding/json"
	"net/http"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"go.uber.org/zap"
)

// handleGetNotificationPreferences returns the full notification preferences matrix for the current user.
// GET /api/user/me/notification-preferences
func (a *App) handleGetNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	preferences, err := prefs.GetFullPreferences(ctx, a.pool.Replica(), userID)
	if err != nil {
		a.log().Error("Failed to get notification preferences", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"preferences": preferences,
		"categories":  prefs.AllCategories,
		"channels":    prefs.AllChannels,
	})
}

// handleUpdateNotificationPreferences updates one or more notification preferences for the current user.
// PUT /api/user/me/notification-preferences
// Body: { "preferences": [{ "category": "transactions", "channel": "email", "enabled": false }] }
func (a *App) handleUpdateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req struct {
		Preferences []prefs.Preference `json:"preferences"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	// Validate
	validCats := make(map[string]bool)
	for _, c := range prefs.AllCategories {
		validCats[c] = true
	}
	validChans := make(map[string]bool)
	for _, c := range prefs.AllChannels {
		validChans[c] = true
	}

	for _, p := range req.Preferences {
		if !validCats[p.Category] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid category: " + p.Category})
			return
		}
		if !validChans[p.Channel] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel: " + p.Channel})
			return
		}
	}

	if err := prefs.SetBulkPreferences(ctx, a.pool.Primary(), userID, req.Preferences); err != nil {
		a.log().Error("Failed to update notification preferences", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Return updated full preferences
	preferences, _ := prefs.GetFullPreferences(ctx, a.pool.Replica(), userID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"preferences": preferences,
		"success":     true,
	})
}
