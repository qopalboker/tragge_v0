package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Entry Tier Types
// =============================================================================

// TierResponse represents an entry tier in API responses.
type TierResponse struct {
	ID                      string                      `json:"id"`
	TemplateID              string                      `json:"template_id"`
	EntryFee                int64                       `json:"entry_fee"`
	Label                   *string                     `json:"label,omitempty"`
	SortOrder               int                         `json:"sort_order"`
	IsActive                bool                        `json:"is_active"`
	IsFree                  bool                        `json:"is_free"`
	QtyTotalOverride        *int64                      `json:"qty_total_override,omitempty"`
	MaxParticipantsOverride *int                        `json:"max_participants_override,omitempty"`
	CommissionRateOverride  *float64                    `json:"commission_rate_override,omitempty"`
	HasPrizeOverride        bool                        `json:"has_prize_override"`
	PrizeDistributions      []PrizeDistributionResponse `json:"prize_distributions,omitempty"`
	CreatedAt               time.Time                   `json:"created_at"`
	UpdatedAt               time.Time                   `json:"updated_at"`
}

// CreateTierRequest represents a request to create an entry tier.
type CreateTierRequest struct {
	EntryFee                int64                    `json:"entry_fee"`
	Label                   *string                  `json:"label,omitempty"`
	SortOrder               int                      `json:"sort_order"`
	IsFree                  bool                     `json:"is_free"`
	QtyTotalOverride        *int64                   `json:"qty_total_override,omitempty"`
	MaxParticipantsOverride *int                     `json:"max_participants_override,omitempty"`
	CommissionRateOverride  *float64                 `json:"commission_rate_override,omitempty"`
	HasPrizeOverride        *bool                    `json:"has_prize_override,omitempty"`
	PrizeDistributions      []PrizeDistributionInput `json:"prize_distributions,omitempty"`
}

// UpdateTierRequest represents a request to update an entry tier.
type UpdateTierRequest struct {
	EntryFee                *int64                   `json:"entry_fee,omitempty"`
	Label                   *string                  `json:"label,omitempty"`
	SortOrder               *int                     `json:"sort_order,omitempty"`
	IsActive                *bool                    `json:"is_active,omitempty"`
	IsFree                  *bool                    `json:"is_free,omitempty"`
	QtyTotalOverride        *int64                   `json:"qty_total_override,omitempty"`
	MaxParticipantsOverride *int                     `json:"max_participants_override,omitempty"`
	CommissionRateOverride  *float64                 `json:"commission_rate_override,omitempty"`
	HasPrizeOverride        *bool                    `json:"has_prize_override,omitempty"`
	PrizeDistributions      []PrizeDistributionInput `json:"prize_distributions,omitempty"`
}

// BulkCreateTiersRequest for creating multiple tiers at once.
type BulkCreateTiersRequest struct {
	Tiers []CreateTierRequest `json:"tiers"`
}

// =============================================================================
// Tier Handlers
// =============================================================================

// handleListTiers lists all tiers for a template.
func (a *App) handleListTiers(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequiredTier})
		return
	}

	rows, err := a.pool.Replica().QueryContext(r.Context(), `
		SELECT t.id, t.template_id, t.entry_fee, t.label, t.sort_order,
		       t.is_active, t.is_free, t.qty_total_override,
		       t.max_participants_override, t.commission_rate_override,
		       t.has_prize_override, t.created_at, t.updated_at
		FROM template_entry_tiers t
		WHERE t.template_id = $1
		ORDER BY t.sort_order ASC, t.entry_fee ASC
	`, templateID)
	if err != nil {
		a.log().Error("Failed to query tiers", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}
	defer rows.Close()

	tiers := []TierResponse{}
	for rows.Next() {
		tier, scanErr := scanTierRow(rows)
		if scanErr != nil {
			a.log().Error("Failed to scan tier row", zap.Error(scanErr))
			continue
		}
		tiers = append(tiers, tier)
	}

	// Fetch prize distributions for tiers with overrides
	for i, tier := range tiers {
		if tier.HasPrizeOverride {
			tiers[i].PrizeDistributions = a.fetchTierPrizeDistributions(r.Context(), tier.ID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tiers": tiers,
		"total": len(tiers),
	})
}

// handleCreateTier creates a new tier for a template.
func (a *App) handleCreateTier(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequiredTier})
		return
	}

	var req CreateTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if errMsg := validateTierRequest(req); errMsg != "" {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, errMsg), http.StatusBadRequest)
		return
	}

	// Verify template exists
	var exists bool
	a.pool.Primary().QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`, templateID).Scan(&exists)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	tier, err := a.insertTier(r.Context(), templateID, req)
	if err != nil {
		a.log().Error("Failed to create tier", zap.Error(err))
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.DuplicateEntryFee})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}

	userID := auth.GetUserID(r.Context())
	a.logAuditEvent(r.Context(), userID, "tier.created", "tier", tier.ID, map[string]interface{}{
		"template_id": templateID, "entry_fee": req.EntryFee,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tier)
}

// handleBulkCreateTiers creates multiple tiers at once.
func (a *App) handleBulkCreateTiers(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "templateID")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequiredTier})
		return
	}

	var req BulkCreateTiersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if len(req.Tiers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.AtLeastOneTier})
		return
	}
	if len(req.Tiers) > 20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.MaxTiers})
		return
	}

	for i, t := range req.Tiers {
		if errMsg := validateTierRequest(t); errMsg != "" {
			http.Error(w, fmt.Sprintf(`{"error":"سطح %d: %s"}`, i, errMsg), http.StatusBadRequest)
			return
		}
	}

	// Verify template exists
	var exists bool
	a.pool.Primary().QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`, templateID).Scan(&exists)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	tx, err := a.pool.Primary().BeginTx(r.Context(), nil)
	if err != nil {
		a.log().Error("Failed to begin transaction for bulk tier creation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}
	defer tx.Rollback()

	created := []TierResponse{}
	for _, tierReq := range req.Tiers {
		tier, err := a.insertTierTx(r.Context(), tx, templateID, tierReq)
		if err != nil {
			if isUniqueViolation(err) {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, adminMsg.DuplicateTierEntryFee), http.StatusConflict)
				return
			}
			a.log().Error("Failed to create tier in bulk", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
			return
		}
		created = append(created, tier)
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit bulk tier creation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}

	userID := auth.GetUserID(r.Context())
	a.logAuditEvent(r.Context(), userID, "tiers.bulk_created", "template", templateID, map[string]interface{}{
		"count": len(created),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tiers":   created,
		"created": len(created),
	})
}

// handleUpdateTier updates a tier.
func (a *App) handleUpdateTier(w http.ResponseWriter, r *http.Request) {
	tierID := chi.URLParam(r, "tierID")
	if tierID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TierIDRequired})
		return
	}

	var req UpdateTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if req.EntryFee != nil && *req.EntryFee < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.EntryFeeNonNegative})
		return
	}
	if req.CommissionRateOverride != nil && (*req.CommissionRateOverride < 0 || *req.CommissionRateOverride > 50) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CommissionRange})
		return
	}
	if req.IsFree != nil && *req.IsFree && req.EntryFee != nil && *req.EntryFee != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.FreeTierEntryFee})
		return
	}

	// Build dynamic UPDATE
	sets := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.EntryFee != nil {
		sets = append(sets, fmt.Sprintf("entry_fee = $%d", argIdx))
		args = append(args, *req.EntryFee)
		argIdx++
	}
	if req.Label != nil {
		sets = append(sets, fmt.Sprintf("label = $%d", argIdx))
		args = append(args, *req.Label)
		argIdx++
	}
	if req.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}
	if req.IsFree != nil {
		sets = append(sets, fmt.Sprintf("is_free = $%d", argIdx))
		args = append(args, *req.IsFree)
		argIdx++
	}
	if req.QtyTotalOverride != nil {
		sets = append(sets, fmt.Sprintf("qty_total_override = $%d", argIdx))
		args = append(args, *req.QtyTotalOverride)
		argIdx++
	}
	if req.MaxParticipantsOverride != nil {
		sets = append(sets, fmt.Sprintf("max_participants_override = $%d", argIdx))
		args = append(args, *req.MaxParticipantsOverride)
		argIdx++
	}
	if req.CommissionRateOverride != nil {
		sets = append(sets, fmt.Sprintf("commission_rate_override = $%d", argIdx))
		args = append(args, *req.CommissionRateOverride)
		argIdx++
	}
	if req.HasPrizeOverride != nil {
		sets = append(sets, fmt.Sprintf("has_prize_override = $%d", argIdx))
		args = append(args, *req.HasPrizeOverride)
		argIdx++
	}

	if len(sets) == 0 && len(req.PrizeDistributions) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	ctx := r.Context()

	if len(sets) > 0 {
		// SAFETY: sets[] contains only hardcoded column names from the if-blocks above
		query := fmt.Sprintf("UPDATE template_entry_tiers SET %s WHERE id = $%d",
			strings.Join(sets, ", "), argIdx)
		args = append(args, tierID)

		result, err := a.pool.Primary().ExecContext(ctx, query, args...)
		if err != nil {
			if isUniqueViolation(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.DuplicateEntryFee})
				return
			}
			a.log().Error("Failed to update tier", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
			return
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TierNotFound})
			return
		}
	}

	// Update prize distributions if provided
	if len(req.PrizeDistributions) > 0 {
		if err := a.replaceTierPrizeDistributions(ctx, tierID, req.PrizeDistributions); err != nil {
			a.log().Error("Failed to update tier prize distributions", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
			return
		}
	}

	// Fetch and return updated tier
	tier, err := a.fetchTierByID(ctx, tierID)
	if err != nil {
		a.log().Error("Failed to fetch updated tier", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}

	userID := auth.GetUserID(ctx)
	a.logAuditEvent(ctx, userID, "tier.updated", "tier", tierID, nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tier)
}

// handleDeleteTier deactivates a tier (soft delete).
func (a *App) handleDeleteTier(w http.ResponseWriter, r *http.Request) {
	tierID := chi.URLParam(r, "tierID")
	if tierID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TierIDRequired})
		return
	}

	result, err := a.pool.Primary().ExecContext(r.Context(),
		`UPDATE template_entry_tiers SET is_active = FALSE WHERE id = $1`, tierID)
	if err != nil {
		a.log().Error("Failed to deactivate tier", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TiersFailed})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TierNotFound})
		return
	}

	userID := auth.GetUserID(r.Context())
	a.logAuditEvent(r.Context(), userID, "tier.deleted", "tier", tierID, nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deactivated"})
}

// =============================================================================
// Validation
// =============================================================================

func validateTierRequest(req CreateTierRequest) string {
	v := validation.New()

	if req.EntryFee < 0 {
		v.AddError("entry_fee", "invalid", "هزینه ورودی نمی‌تواند منفی باشد")
	}
	if req.SortOrder < 0 {
		v.AddError("sort_order", "invalid", "ترتیب نمایش نمی‌تواند منفی باشد")
	}
	if req.IsFree && req.EntryFee != 0 {
		v.AddError("entry_fee", "invalid", "سطح رایگان باید هزینه ورودی صفر داشته باشد")
	}
	if req.QtyTotalOverride != nil && *req.QtyTotalOverride <= 0 {
		v.AddError("qty_total_override", "invalid", "باید مثبت باشد")
	}
	if req.MaxParticipantsOverride != nil && *req.MaxParticipantsOverride <= 0 {
		v.AddError("max_participants_override", "invalid", "باید مثبت باشد")
	}
	if req.CommissionRateOverride != nil && (*req.CommissionRateOverride < 0 || *req.CommissionRateOverride > 50) {
		v.AddError("commission_rate_override", "invalid", "باید بین ۰ تا ۵۰ باشد")
	}

	// Validate prize distributions
	if len(req.PrizeDistributions) > 0 {
		var totalPct float64
		ranks := make(map[int]bool)
		for _, pd := range req.PrizeDistributions {
			if pd.Rank <= 0 {
				v.AddError("prize_distributions.rank", "invalid", "رتبه باید مثبت باشد")
			}
			if pd.Percentage <= 0 || pd.Percentage > 100 {
				v.AddError("prize_distributions.percentage", "invalid", "درصد باید بین ۰ تا ۱۰۰ باشد")
			}
			if ranks[pd.Rank] {
				v.AddError("prize_distributions.rank", "duplicate", fmt.Sprintf("رتبه %d تکراری است", pd.Rank))
			}
			ranks[pd.Rank] = true
			totalPct += pd.Percentage
		}
		if totalPct > 100 {
			v.AddError("prize_distributions", "invalid", "مجموع درصدها نباید بیشتر از ۱۰۰ باشد")
		}
	}

	if v.HasErrors() {
		errs := v.Errors()
		return errs[0].Message
	}
	return ""
}

// =============================================================================
// Database Helpers
// =============================================================================

func (a *App) insertTier(ctx context.Context, templateID string, req CreateTierRequest) (TierResponse, error) {
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		return TierResponse{}, err
	}
	defer tx.Rollback()

	tier, err := a.insertTierTx(ctx, tx, templateID, req)
	if err != nil {
		return TierResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return TierResponse{}, err
	}
	return tier, nil
}

// insertTierTx inserts a tier within an existing transaction.
func (a *App) insertTierTx(ctx context.Context, tx *sql.Tx, templateID string, req CreateTierRequest) (TierResponse, error) {
	tierID := uuid.New().String()
	hasPrizeOverride := req.HasPrizeOverride != nil && *req.HasPrizeOverride

	var tier TierResponse
	err := tx.QueryRowContext(ctx, `
		INSERT INTO template_entry_tiers (
			id, template_id, entry_fee, label, sort_order, is_active, is_free,
			qty_total_override, max_participants_override, commission_rate_override,
			has_prize_override
		) VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7, $8, $9, $10)
		RETURNING id, template_id, entry_fee, label, sort_order, is_active, is_free,
		          qty_total_override, max_participants_override, commission_rate_override,
		          has_prize_override, created_at, updated_at
	`, tierID, templateID, req.EntryFee, req.Label, req.SortOrder, req.IsFree,
		req.QtyTotalOverride, req.MaxParticipantsOverride, req.CommissionRateOverride,
		hasPrizeOverride,
	).Scan(
		&tier.ID, &tier.TemplateID, &tier.EntryFee, &tier.Label, &tier.SortOrder,
		&tier.IsActive, &tier.IsFree, &tier.QtyTotalOverride, &tier.MaxParticipantsOverride,
		&tier.CommissionRateOverride, &tier.HasPrizeOverride, &tier.CreatedAt, &tier.UpdatedAt,
	)
	if err != nil {
		return TierResponse{}, err
	}

	// Insert prize distributions if provided
	if hasPrizeOverride && len(req.PrizeDistributions) > 0 {
		if pdErr := a.replaceTierPrizeDistributionsTx(ctx, tx, tierID, req.PrizeDistributions); pdErr != nil {
			return tier, pdErr
		}
		tier.PrizeDistributions = a.fetchTierPrizeDistributionsTx(ctx, tx, tierID)
	}

	return tier, nil
}

func (a *App) fetchTierByID(ctx context.Context, tierID string) (TierResponse, error) {
	row := a.pool.Primary().QueryRowContext(ctx, `
		SELECT id, template_id, entry_fee, label, sort_order, is_active, is_free,
		       qty_total_override, max_participants_override, commission_rate_override,
		       has_prize_override, created_at, updated_at
		FROM template_entry_tiers WHERE id = $1
	`, tierID)

	var tier TierResponse
	err := row.Scan(
		&tier.ID, &tier.TemplateID, &tier.EntryFee, &tier.Label, &tier.SortOrder,
		&tier.IsActive, &tier.IsFree, &tier.QtyTotalOverride, &tier.MaxParticipantsOverride,
		&tier.CommissionRateOverride, &tier.HasPrizeOverride, &tier.CreatedAt, &tier.UpdatedAt,
	)
	if err != nil {
		return TierResponse{}, err
	}

	if tier.HasPrizeOverride {
		tier.PrizeDistributions = a.fetchTierPrizeDistributions(ctx, tierID)
	}

	return tier, nil
}

func (a *App) fetchTiersForTemplate(ctx context.Context, templateID string) []TierResponse {
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT id, template_id, entry_fee, label, sort_order, is_active, is_free,
		       qty_total_override, max_participants_override, commission_rate_override,
		       has_prize_override, created_at, updated_at
		FROM template_entry_tiers
		WHERE template_id = $1
		ORDER BY sort_order ASC, entry_fee ASC
	`, templateID)
	if err != nil {
		a.log().Error("Failed to fetch tiers", zap.String("template_id", templateID), zap.Error(err))
		return nil
	}
	defer rows.Close()

	var tiers []TierResponse
	for rows.Next() {
		tier, scanErr := scanTierRow(rows)
		if scanErr != nil {
			continue
		}
		if tier.HasPrizeOverride {
			tier.PrizeDistributions = a.fetchTierPrizeDistributions(ctx, tier.ID)
		}
		tiers = append(tiers, tier)
	}
	return tiers
}

func (a *App) fetchTierPrizeDistributions(ctx context.Context, tierID string) []PrizeDistributionResponse {
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT id, rank, percentage, min_participants
		FROM tier_prize_distributions
		WHERE tier_id = $1
		ORDER BY rank ASC
	`, tierID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dists []PrizeDistributionResponse
	for rows.Next() {
		var d PrizeDistributionResponse
		if err := rows.Scan(&d.ID, &d.Rank, &d.Percentage, &d.MinParticipants); err != nil {
			continue
		}
		dists = append(dists, d)
	}
	return dists
}

func (a *App) fetchTierPrizeDistributionsTx(ctx context.Context, tx *sql.Tx, tierID string) []PrizeDistributionResponse {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, rank, percentage, min_participants
		FROM tier_prize_distributions
		WHERE tier_id = $1
		ORDER BY rank ASC
	`, tierID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dists []PrizeDistributionResponse
	for rows.Next() {
		var d PrizeDistributionResponse
		if err := rows.Scan(&d.ID, &d.Rank, &d.Percentage, &d.MinParticipants); err != nil {
			continue
		}
		dists = append(dists, d)
	}
	return dists
}

func (a *App) replaceTierPrizeDistributions(ctx context.Context, tierID string, dists []PrizeDistributionInput) error {
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := a.replaceTierPrizeDistributionsTx(ctx, tx, tierID, dists); err != nil {
		return err
	}

	return tx.Commit()
}

// replaceTierPrizeDistributionsTx replaces prize distributions within an existing transaction.
func (a *App) replaceTierPrizeDistributionsTx(ctx context.Context, tx *sql.Tx, tierID string, dists []PrizeDistributionInput) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM tier_prize_distributions WHERE tier_id = $1`, tierID); err != nil {
		return err
	}

	for _, d := range dists {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO tier_prize_distributions (tier_id, rank, percentage, min_participants)
			VALUES ($1, $2, $3, $4)
		`, tierID, d.Rank, d.Percentage, d.MinParticipants)
		if err != nil {
			return err
		}
	}

	return nil
}

func scanTierRow(scanner interface{ Scan(...interface{}) error }) (TierResponse, error) {
	var tier TierResponse
	err := scanner.Scan(
		&tier.ID, &tier.TemplateID, &tier.EntryFee, &tier.Label, &tier.SortOrder,
		&tier.IsActive, &tier.IsFree, &tier.QtyTotalOverride, &tier.MaxParticipantsOverride,
		&tier.CommissionRateOverride, &tier.HasPrizeOverride, &tier.CreatedAt, &tier.UpdatedAt,
	)
	return tier, err
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "23505")
}
