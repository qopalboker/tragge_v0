package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// StateTransitionRequest represents a request to transition contest state.
type StateTransitionRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ContestStateResponse represents the response for contest state operations.
type ContestStateResponse struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Status              string  `json:"status"`
	PreviousStatus      string  `json:"previous_status,omitempty"`
	CurrentParticipants int     `json:"current_participants"`
	MinParticipants     int     `json:"min_participants"`
	MaxParticipants     *int    `json:"max_participants,omitempty"`
	Timestamp           int64   `json:"timestamp"`
	Reason              *string `json:"reason,omitempty"`
}

// StatusHistoryResponse represents a status history entry.
type StatusHistoryResponse struct {
	ID         string         `json:"id"`
	FromStatus *string        `json:"from_status,omitempty"`
	ToStatus   string         `json:"to_status"`
	ChangedBy  *string        `json:"changed_by,omitempty"`
	Reason     *string        `json:"reason,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  int64          `json:"created_at"`
}

// handlePublishContest publishes a contest (draft -> scheduled).
// POST /api/admin/contests/{id}/publish
func (a *App) handlePublishContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	sm := a.stateMachine
	result, err := sm.Publish(ctx, contestID, &actorUserID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Contest published",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// handleStartContest manually starts a contest (-> running).
// POST /api/admin/contests/{id}/start
func (a *App) handleStartContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	sm := a.stateMachine
	result, err := sm.Start(ctx, contestID, &actorUserID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Contest started",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// handleEndContest manually ends a contest (running -> settling).
// POST /api/admin/contests/{id}/end
func (a *App) handleEndContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	sm := a.stateMachine
	result, err := sm.End(ctx, contestID, &actorUserID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Contest ended",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// contestParticipantInfo holds participant info for cancellation emails.
type contestParticipantInfo struct {
	UserID string
	Email  string
	Name   string
}

// contestCancellationDetails holds contest details for cancellation processing.
type contestCancellationDetails struct {
	Name          string
	StartsAt      time.Time
	EntryFeeCents int64
}

// handleCancelContest cancels a contest (-> cancelled).
// POST /api/admin/contests/{id}/cancel
func (a *App) handleCancelContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Parse request body for reason
	var req StateTransitionRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
			return
		}
	}

	if req.Reason == "" {
		req.Reason = "Cancelled by admin"
	}

	sm := a.stateMachine
	result, err := sm.Cancel(ctx, contestID, &actorUserID, req.Reason)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	// Query participants for this contest
	participants, err := a.getContestParticipants(ctx, contestID)
	if err != nil {
		a.log().Error("Failed to query contest participants for cancellation emails",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Continue with the response - cancellation was successful
	}

	// Query contest details for the email
	contestDetails, err := a.getContestCancellationDetails(ctx, contestID)
	if err != nil {
		a.log().Error("Failed to query contest details for cancellation emails",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Use defaults from result
		contestDetails = &contestCancellationDetails{
			Name:          result.Contest.Name,
			StartsAt:      result.Contest.StartsAt,
			EntryFeeCents: 0,
		}
	}

	// Track refund totals for audit log
	var totalRefunded int64
	var refundCount int
	refundResults := make(map[string]int64) // userID -> newBalance

	// Process refunds if contest had an entry fee and there are participants
	if contestDetails.EntryFeeCents > 0 && len(participants) > 0 {
		refundResults, totalRefunded, refundCount = a.processContestRefunds(ctx, contestID, contestDetails.Name, participants, contestDetails.EntryFeeCents)
	}

	// Send cancellation emails asynchronously (don't block the response)
	if len(participants) > 0 {
		infra.SafeGo(a.log(), "contest-cancellation-emails", func() {
			a.sendContestCancellationEmails(ctx, contestID, contestDetails, participants, req.Reason, refundResults)
		})
	}

	// Create in-app notifications for each participant
	if len(participants) > 0 {
		infra.SafeGo(a.log(), "contest-cancellation-notifications", func() {
			a.createContestCancellationNotifications(ctx, contestID, contestDetails.Name, req.Reason, participants, contestDetails.EntryFeeCents)
		})
	}

	// Write audit log for the cancellation
	auditPayload := map[string]interface{}{
		"contest_id":            contestID,
		"contest_name":          contestDetails.Name,
		"reason":                req.Reason,
		"previous_status":       result.FromStatus.String(),
		"participants_affected": len(participants),
		"total_refunded_cents":  totalRefunded,
		"refund_count":          refundCount,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	auditErr := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx,
			`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
			 VALUES ($1, $2, $3, $4, $5)`,
			actorUserID, "contest.cancelled", "contest", contestID, auditPayloadJSON)
		return execErr
	})
	if auditErr != nil {
		a.log().Error("Failed to write audit log for contest cancellation", zap.Error(auditErr))
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
		Reason:              &req.Reason,
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Contest cancelled",
		zap.String("contest_id", contestID),
		zap.String("reason", req.Reason),
		zap.String("actor", actorUserID),
		zap.Int("participants_affected", len(participants)),
		zap.Int64("total_refunded_cents", totalRefunded))

	writeJSON(w, http.StatusOK, resp)
}

// getContestParticipants retrieves participants for a contest.
func (a *App) getContestParticipants(ctx context.Context, contestID string) ([]contestParticipantInfo, error) {
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT cp.user_id, u.email, COALESCE(u.display_name, u.username, u.email) as name
				FROM contest_participants cp
				JOIN users u ON cp.user_id = u.id
				WHERE cp.contest_id = $1
			`, contestID)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query participants: %w", err)
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var participants []contestParticipantInfo
	for rows.Next() {
		var p contestParticipantInfo
		if err := rows.Scan(&p.UserID, &p.Email, &p.Name); err != nil {
			continue // Skip participants with scan errors
		}
		participants = append(participants, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate participants: %w", err)
	}

	return participants, nil
}

// getContestCancellationDetails retrieves contest details for cancellation emails.
func (a *App) getContestCancellationDetails(ctx context.Context, contestID string) (*contestCancellationDetails, error) {
	var details contestCancellationDetails
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT name, starts_at, entry_fee_cents
			FROM contests
			WHERE id = $1
		`, contestID).Scan(&details.Name, &details.StartsAt, &details.EntryFeeCents)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query contest details: %w", err)
	}
	return &details, nil
}

// processContestRefunds processes refunds for all participants in a cancelled contest
// using a single atomic transaction. If any refund fails, the entire batch is rolled back.
// Uses the idempotent refund method to prevent double-refunds on retry.
// Returns a map of userID -> newBalance, total refunded amount, and count of successful refunds.
func (a *App) processContestRefunds(ctx context.Context, contestID string, contestName string, participants []contestParticipantInfo, entryFeeCents int64) (map[string]int64, int64, int) {
	refundResults := make(map[string]int64)
	var totalRefunded int64
	var refundCount int

	walletSvc := wallet.NewService(a.pool.Primary())

	// Begin a single transaction for all refunds (circuit breaker protected)
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		a.log().Error("Failed to begin transaction for contest refunds",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return refundResults, totalRefunded, refundCount
	}
	defer tx.Rollback()

	for _, p := range participants {
		// Process the refund using idempotent method to prevent double-refunds
		entry, err := walletSvc.RefundContestEntryFeeIdempotent(ctx, tx, p.UserID, contestID, contestName, entryFeeCents, wallet.ReasonCodeContestRefundAdmin)
		if err != nil {
			a.log().Error("Failed to refund entry fee, rolling back all refunds",
				zap.String("user_id", p.UserID),
				zap.String("contest_id", contestID),
				zap.Int64("amount_cents", entryFeeCents),
				zap.Error(err))
			return refundResults, 0, 0
		}

		// Track the successful refund
		if entry != nil {
			refundResults[p.UserID] = entry.BalanceAfterCents
			totalRefunded += entryFeeCents
			refundCount++

			a.log().Debug("Refunded entry fee",
				zap.String("user_id", p.UserID),
				zap.String("contest_id", contestID),
				zap.Int64("amount_cents", entryFeeCents),
				zap.Int64("new_balance", entry.BalanceAfterCents))
		}
	}

	// Commit all refunds atomically
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit contest refunds transaction",
			zap.String("contest_id", contestID),
			zap.Int("participants", len(participants)),
			zap.Error(err))
		return make(map[string]int64), 0, 0
	}

	a.log().Info("All contest refunds committed successfully",
		zap.String("contest_id", contestID),
		zap.Int("refund_count", refundCount),
		zap.Int64("total_refunded", totalRefunded))

	return refundResults, totalRefunded, refundCount
}

// sendContestCancellationEmails sends cancellation emails to all participants.
func (a *App) sendContestCancellationEmails(ctx context.Context, contestID string, details *contestCancellationDetails, participants []contestParticipantInfo, reason string, refundResults map[string]int64) {
	if a.emailNotifier == nil {
		a.log().Warn("Email notifier not configured, skipping cancellation emails",
			zap.String("contest_id", contestID))
		return
	}

	// Build frontend URL for contests page
	contestsURL := a.config.FrontendBaseURL + "/user/contests"

	// Filter participants by email notification preferences
	pUserIDs := make([]string, len(participants))
	for i, p := range participants {
		pUserIDs[i] = p.UserID
	}
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, a.pool.Replica(), pUserIDs, inapp.NotifTypeContestCancelled, "email")

	// Build recipients with personalized data
	var recipients []notification.ContestCancelledRecipient
	for _, p := range participants {
		if !emailEnabledMap[p.UserID] {
			continue
		}
		data := notification.ContestCancelledData{
			UserName:       p.Name,
			ContestID:      contestID,
			ContestName:    details.Name,
			Reason:         reason,
			ScheduledStart: details.StartsAt.Format("Jan 2, 2006 3:04 PM"),
			ContestsURL:    contestsURL,
		}

		// Add refund info if applicable
		if details.EntryFeeCents > 0 {
			data.RefundAmount = formatCurrency(details.EntryFeeCents)
			if newBalance, ok := refundResults[p.UserID]; ok {
				data.NewBalance = formatCurrency(newBalance)
			}
		}

		recipients = append(recipients, notification.ContestCancelledRecipient{
			Email: p.Email,
			Data:  data,
		})
	}

	// Use a fresh context with timeout for async email sending
	emailCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Send emails in batch
	result := a.emailNotifier.SendContestCancelledBatch(emailCtx, recipients)

	// Log results
	if len(result.Failed) > 0 {
		a.log().Warn("Some cancellation emails failed to send",
			zap.String("contest_id", contestID),
			zap.Int("successful", len(result.Successful)),
			zap.Int("failed", len(result.Failed)))
	} else {
		a.log().Info("All cancellation emails sent successfully",
			zap.String("contest_id", contestID),
			zap.Int("count", len(result.Successful)))
	}
}

// createContestCancellationNotifications creates in-app notifications for all participants.
func (a *App) createContestCancellationNotifications(ctx context.Context, contestID, contestName, reason string, participants []contestParticipantInfo, entryFeeCents int64) {
	// Use a fresh context with timeout for async notification creation
	notifCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Filter participants by their notification preferences
	userIDs := make([]string, len(participants))
	userMap := make(map[string]contestParticipantInfo)
	for i, p := range participants {
		userIDs[i] = p.UserID
		userMap[p.UserID] = p
	}
	enabledMap, _ := prefs.IsEnabledBatch(notifCtx, a.pool.Replica(), userIDs, inapp.NotifTypeContestCancelled, "in_app")

	created := 0
	for _, p := range participants {
		if !enabledMap[p.UserID] {
			continue
		}
		err := inapp.CreateContestCancelledNotification(notifCtx, a.pool.Primary(), p.UserID, contestID, contestName, reason, entryFeeCents)
		if err != nil {
			a.log().Error("Failed to create in-app notification for contest cancellation",
				zap.String("user_id", p.UserID),
				zap.String("contest_id", contestID),
				zap.Error(err))
			// Continue with other participants
		} else {
			created++
		}
	}

	a.log().Info("Created in-app notifications for contest cancellation",
		zap.String("contest_id", contestID),
		zap.Int("count", created),
		zap.Int("total_participants", len(participants)))
}

// formatCurrency formats cents to a currency string (e.g., "$10.50").
func formatCurrency(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

// handlePauseContest pauses a running contest.
// POST /api/admin/contests/{id}/pause
func (a *App) handlePauseContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var req StateTransitionRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
			return
		}
	}

	sm := a.stateMachine
	result, err := sm.Pause(ctx, contestID, &actorUserID, req.Reason)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}
	if req.Reason != "" {
		resp.Reason = &req.Reason
	}

	a.log().Info("Contest paused",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// handleResumeContest resumes a paused contest.
// POST /api/admin/contests/{id}/resume
func (a *App) handleResumeContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	sm := a.stateMachine
	result, err := sm.Resume(ctx, contestID, &actorUserID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Contest resumed",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// handleCloseRegistration closes registration for a contest.
// POST /api/admin/contests/{id}/close-registration
func (a *App) handleCloseRegistration(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var req StateTransitionRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
			return
		}
	}

	sm := a.stateMachine
	result, err := sm.CloseRegistration(ctx, contestID, &actorUserID, req.Reason)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := ContestStateResponse{
		ID:                  result.Contest.ID,
		Name:                result.Contest.Name,
		Status:              result.ToStatus.String(),
		PreviousStatus:      result.FromStatus.String(),
		CurrentParticipants: result.Contest.CurrentParticipants,
		MinParticipants:     result.Contest.MinParticipants,
		Timestamp:           result.Timestamp.UnixMilli(),
	}
	if result.Contest.MaxParticipants != nil {
		resp.MaxParticipants = result.Contest.MaxParticipants
	}

	a.log().Info("Registration closed",
		zap.String("contest_id", contestID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, resp)
}

// handleGetContestState retrieves the current state of a contest.
// GET /api/admin/contests/{id}/state
func (a *App) handleGetContestState(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()

	sm := a.stateMachine
	contest, err := sm.GetContest(ctx, contestID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	// Get allowed transitions
	allowedTransitions := statemachine.GetAllowedTransitions(contest.Status)
	transitionStrings := make([]string, len(allowedTransitions))
	for i, t := range allowedTransitions {
		transitionStrings[i] = t.String()
	}

	resp := map[string]any{
		"id":                   contest.ID,
		"name":                 contest.Name,
		"status":               contest.Status.String(),
		"starts_at":            contest.StartsAt,
		"ends_at":              contest.EndsAt,
		"current_participants": contest.CurrentParticipants,
		"min_participants":     contest.MinParticipants,
		"auto_start":           contest.AutoStart,
		"allowed_transitions":  transitionStrings,
		"is_final":             contest.Status.IsFinal(),
		"allows_trading":       contest.Status.AllowsTrading(),
		"allows_registration":  contest.Status.AllowsRegistration(),
	}

	if contest.MaxParticipants != nil {
		resp["max_participants"] = *contest.MaxParticipants
	}
	if contest.RegistrationDeadline != nil {
		resp["registration_deadline"] = *contest.RegistrationDeadline
	}
	if contest.PublishedAt != nil {
		resp["published_at"] = *contest.PublishedAt
	}
	if contest.StartedAt != nil {
		resp["started_at"] = *contest.StartedAt
	}
	if contest.EndedAt != nil {
		resp["ended_at"] = *contest.EndedAt
	}
	if contest.SettledAt != nil {
		resp["settled_at"] = *contest.SettledAt
	}
	if contest.CancelledAt != nil {
		resp["cancelled_at"] = *contest.CancelledAt
	}
	if contest.CancellationReason != nil {
		resp["cancellation_reason"] = *contest.CancellationReason
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetContestStatusHistory retrieves the status transition history.
// GET /api/admin/contests/{id}/status-history
func (a *App) handleGetContestStatusHistory(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()

	sm := a.stateMachine
	history, err := sm.GetStatusHistory(ctx, contestID)
	if err != nil {
		a.handleStateMachineError(w, err)
		return
	}

	resp := make([]StatusHistoryResponse, len(history))
	for i, entry := range history {
		resp[i] = StatusHistoryResponse{
			ID:        entry.ID,
			ToStatus:  entry.ToStatus.String(),
			ChangedBy: entry.ChangedBy,
			Reason:    entry.Reason,
			Metadata:  entry.Metadata,
			CreatedAt: entry.CreatedAt.UnixMilli(),
		}
		if entry.FromStatus != nil {
			s := entry.FromStatus.String()
			resp[i].FromStatus = &s
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contest_id": contestID,
		"history":    resp,
	})
}

// handleStateMachineError converts state machine errors to HTTP responses.
func (a *App) handleStateMachineError(w http.ResponseWriter, err error) {
	if errors.Is(err, statemachine.ErrContestNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
		return
	}

	var transitionErr *statemachine.TransitionError
	if errors.As(err, &transitionErr) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":       adminMsg.InvalidStateTransition,
			"from_status": transitionErr.FromStatus.String(),
			"to_status":   transitionErr.ToStatus.String(),
			"reason":      transitionErr.Reason,
		})
		return
	}

	if errors.Is(err, statemachine.ErrMinParticipants) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.MinParticipantsNotMet})
		return
	}

	if errors.Is(err, statemachine.ErrRegistrationClosed) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.RegistrationClosed})
		return
	}

	if errors.Is(err, statemachine.ErrContestInFinalState) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.ContestFinalState})
		return
	}

	a.log().Error("State machine error", zap.Error(err))
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
}

// initStateMachine initializes the shared state machine instance with Kafka event publishing.
// Called once at startup; handlers use a.stateMachine directly.
func (a *App) initStateMachine() {
	config := statemachine.DefaultConfig()
	config.Logger = a.log()

	// Wire up Kafka producer for publishing contest state events
	if a.kafkaProducer != nil {
		config.KafkaProducer = a.kafkaProducer
		config.ContestStateTopic = "contests.v1"

		// Register side effect handlers so state transitions trigger
		// downstream actions (trading engine, leaderboard, notifications)
		handlersConfig := statemachine.DefaultHandlersConfig()
		sideEffects := statemachine.NewSideEffectsWithConfig(
			a.pool, a.kafkaProducer, a.log(), handlersConfig,
		)
		if a.emailNotifier != nil {
			sideEffects.SetEmailNotifier(a.emailNotifier, a.config.FrontendBaseURL)
		}
		if a.walletService != nil {
			sideEffects.SetWalletService(a.walletService)
		}
		config.SideEffectHandlers = sideEffects.GetRegisteredHandlers()
	}

	a.stateMachine = statemachine.New(a.pool, config)
}
