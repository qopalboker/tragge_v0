package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// =============================================================================
// Tournament Template Types
// =============================================================================

// TemplateResponse represents a tournament template in API responses.
type TemplateResponse struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Description          *string   `json:"description,omitempty"`
	DurationMinutes      int       `json:"duration_minutes"`
	MarketType           *string   `json:"market_type,omitempty"`
	TemplateDurationType *string   `json:"template_duration_type,omitempty"`
	EntryFee             int64     `json:"entry_fee"`
	EntryFeeCents        int       `json:"entry_fee_cents"`
	QtyTotal             int64     `json:"qty_total"`
	SymbolsJSON          string    `json:"symbols_json"`
	HasPrize             bool      `json:"has_prize"`
	IsActive             bool      `json:"is_active"`
	IsFree               bool      `json:"is_free"`
	AssetClass           string    `json:"asset_class"`
	CommissionRate       float64   `json:"commission_rate"`
	MinParticipants      int       `json:"min_participants"`
	MaxParticipants      *int      `json:"max_participants,omitempty"`
	AutoCreate           bool      `json:"auto_create"`
	CreateCron           *string   `json:"create_cron,omitempty"`
	TemplateKey          *string   `json:"template_key,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// TemplateDetailResponse includes template with its schedules, prize distributions, and tiers.
type TemplateDetailResponse struct {
	TemplateResponse
	Schedules          []ScheduleResponse          `json:"schedules"`
	PrizeDistributions []PrizeDistributionResponse `json:"prize_distributions"`
	Tiers              []TierResponse              `json:"tiers"`
	TierCount          int                         `json:"tier_count"`
}

// PrizeDistributionResponse represents a prize distribution rule.
type PrizeDistributionResponse struct {
	ID              string  `json:"id"`
	Rank            int     `json:"rank"`
	Percentage      float64 `json:"percentage"`
	MinParticipants int     `json:"min_participants"`
}

// CreateTemplateRequest represents a request to create a tournament template.
type CreateTemplateRequest struct {
	Name                 string                   `json:"name"`
	Description          *string                  `json:"description,omitempty"`
	DurationMinutes      int                      `json:"duration_minutes"`
	MarketType           string                   `json:"market_type"`
	TemplateDurationType string                   `json:"template_duration_type"`
	EntryFee             int64                    `json:"entry_fee"`
	QtyTotal             int64                    `json:"qty_total"`
	Symbols              []string                 `json:"symbols"`
	HasPrize             bool                     `json:"has_prize"`
	IsFree               bool                     `json:"is_free"`
	AssetClass           string                   `json:"asset_class"`
	CommissionRate       float64                  `json:"commission_rate"`
	MinParticipants      int                      `json:"min_participants"`
	MaxParticipants      *int                     `json:"max_participants,omitempty"`
	TemplateKey          *string                  `json:"template_key,omitempty"`
	PrizeDistributions   []PrizeDistributionInput `json:"prize_distributions,omitempty"`
	EntryTiers           []CreateTierRequest      `json:"entry_tiers,omitempty"`
}

// PrizeDistributionInput represents a prize distribution rule in create/update requests.
type PrizeDistributionInput struct {
	Rank            int     `json:"rank"`
	Percentage      float64 `json:"percentage"`
	MinParticipants int     `json:"min_participants"`
}

// UpdateTemplateRequest represents a request to update a tournament template.
type UpdateTemplateRequest struct {
	Name                 *string                  `json:"name,omitempty"`
	Description          *string                  `json:"description,omitempty"`
	DurationMinutes      *int                     `json:"duration_minutes,omitempty"`
	MarketType           *string                  `json:"market_type,omitempty"`
	TemplateDurationType *string                  `json:"template_duration_type,omitempty"`
	EntryFee             *int64                   `json:"entry_fee,omitempty"`
	QtyTotal             *int64                   `json:"qty_total,omitempty"`
	Symbols              []string                 `json:"symbols,omitempty"`
	HasPrize             *bool                    `json:"has_prize,omitempty"`
	IsFree               *bool                    `json:"is_free,omitempty"`
	AssetClass           *string                  `json:"asset_class,omitempty"`
	CommissionRate       *float64                 `json:"commission_rate,omitempty"`
	MinParticipants      *int                     `json:"min_participants,omitempty"`
	MaxParticipants      *int                     `json:"max_participants,omitempty"`
	PrizeDistributions   []PrizeDistributionInput `json:"prize_distributions,omitempty"`
}

// =============================================================================
// Validation helpers
// =============================================================================

var validMarketTypes = map[string]bool{
	"crypto": true,
	"forex":  true,
}

var validTemplateDurationTypes = map[string]bool{
	"quick_30m": true,
	"free_1h":   true,
	"four_hour": true,
	"daily":     true,
	"weekly":    true,
	"special":   true,
}

var validAssetClasses = map[string]bool{
	"forex":  true,
	"crypto": true,
	"stocks": true,
	"mixed":  true,
}

func validatePrizeDistributions(v *validation.Validator, distributions []PrizeDistributionInput) {
	if len(distributions) == 0 {
		return
	}

	var totalPercentage float64
	ranks := make(map[int]bool)

	for i, pd := range distributions {
		prefix := fmt.Sprintf("prize_distributions[%d]", i)
		if pd.Rank <= 0 {
			v.AddError(prefix+".rank", "invalid", "rank must be a positive integer")
		}
		if ranks[pd.Rank] {
			v.AddError(prefix+".rank", "duplicate", fmt.Sprintf("duplicate rank %d", pd.Rank))
		}
		ranks[pd.Rank] = true

		if pd.Percentage <= 0 || pd.Percentage > 100 {
			v.AddError(prefix+".percentage", "invalid", "percentage must be between 0 (exclusive) and 100 (inclusive)")
		}
		totalPercentage += pd.Percentage

		if pd.MinParticipants < 1 {
			v.AddError(prefix+".min_participants", "invalid", "min_participants must be at least 1")
		}
	}

	if totalPercentage > 100 {
		v.AddError("prize_distributions", "invalid", fmt.Sprintf("total percentage %.2f exceeds 100%%", totalPercentage))
	}
}

// =============================================================================
// Template scan helpers
// =============================================================================

const templateSelectColumns = `
	t.id, t.name, t.description, t.duration_minutes,
	t.market_type, t.template_duration_type,
	t.entry_fee, t.entry_fee_cents, t.qty_total,
	t.symbols_json, t.has_prize, t.is_active, t.is_free,
	t.asset_class, t.commission_rate, t.min_participants,
	t.max_participants, t.auto_create, t.create_cron,
	t.template_key, t.created_at, t.updated_at
`

func scanTemplateRow(scanner interface {
	Scan(dest ...interface{}) error
}) (TemplateResponse, error) {
	var t TemplateResponse
	var symbolsRaw []byte
	err := scanner.Scan(
		&t.ID, &t.Name, &t.Description, &t.DurationMinutes,
		&t.MarketType, &t.TemplateDurationType,
		&t.EntryFee, &t.EntryFeeCents, &t.QtyTotal,
		&symbolsRaw, &t.HasPrize, &t.IsActive, &t.IsFree,
		&t.AssetClass, &t.CommissionRate, &t.MinParticipants,
		&t.MaxParticipants, &t.AutoCreate, &t.CreateCron,
		&t.TemplateKey, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return t, err
	}
	t.SymbolsJSON = string(symbolsRaw)
	return t, nil
}

// =============================================================================
// Template Handlers
// =============================================================================

// handleListTemplates lists tournament templates with filtering, pagination and sorting.
// GET /api/admin/templates
func (a *App) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	// Build WHERE clause
	var args []interface{}
	argIdx := 1
	whereClause := " WHERE 1=1"

	// Filter: market_type
	if mt := r.URL.Query().Get("market_type"); mt != "" {
		if !validMarketTypes[mt] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidMarketType})
			return
		}
		whereClause += fmt.Sprintf(" AND t.market_type = $%d", argIdx)
		args = append(args, mt)
		argIdx++
	}

	// Filter: duration_type (maps to template_duration_type)
	if dt := r.URL.Query().Get("duration_type"); dt != "" {
		if !validTemplateDurationTypes[dt] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidDurationType})
			return
		}
		whereClause += fmt.Sprintf(" AND t.template_duration_type = $%d", argIdx)
		args = append(args, dt)
		argIdx++
	}

	// Filter: is_active (default: show all; explicit "true"/"false" to filter)
	if ia := r.URL.Query().Get("is_active"); ia != "" {
		isActive, err := strconv.ParseBool(ia)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.IsActiveInvalid})
			return
		}
		whereClause += fmt.Sprintf(" AND t.is_active = $%d", argIdx)
		args = append(args, isActive)
		argIdx++
	}

	// Filter: search by name
	if search := r.URL.Query().Get("search"); search != "" {
		whereClause += fmt.Sprintf(" AND t.name ILIKE $%d", argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	// Filter: has_tiers
	if ht := r.URL.Query().Get("has_tiers"); ht != "" {
		hasTiers, parseErr := strconv.ParseBool(ht)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.HasTiersInvalid})
			return
		}
		if hasTiers {
			whereClause += " AND EXISTS (SELECT 1 FROM template_entry_tiers tet WHERE tet.template_id = t.id AND tet.is_active = TRUE)"
		} else {
			whereClause += " AND NOT EXISTS (SELECT 1 FROM template_entry_tiers tet WHERE tet.template_id = t.id AND tet.is_active = TRUE)"
		}
	}

	// Filter: tier_count_min
	if tcm := r.URL.Query().Get("tier_count_min"); tcm != "" {
		minCount, parseErr := strconv.Atoi(tcm)
		if parseErr != nil || minCount < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TierCountMinInvalid})
			return
		}
		whereClause += fmt.Sprintf(" AND (SELECT COUNT(*) FROM template_entry_tiers tet WHERE tet.template_id = t.id AND tet.is_active = TRUE) >= $%d", argIdx)
		args = append(args, minCount)
		argIdx++
	}

	baseQuery := " FROM tournament_templates t"

	// Count total
	var total int
	countQuery := "SELECT COUNT(*)" + baseQuery + whereClause
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count templates", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Determine sort order
	sortField := "t.created_at"
	sortDir := "DESC"
	if s := r.URL.Query().Get("sort"); s != "" {
		switch s {
		case "name_asc":
			sortField = "t.name"
			sortDir = "ASC"
		case "name_desc":
			sortField = "t.name"
			sortDir = "DESC"
		case "entry_fee_asc":
			sortField = "t.entry_fee"
			sortDir = "ASC"
		case "entry_fee_desc":
			sortField = "t.entry_fee"
			sortDir = "DESC"
		case "created_at_asc":
			sortField = "t.created_at"
			sortDir = "ASC"
		case "created_at_desc":
			sortField = "t.created_at"
			sortDir = "DESC"
		}
	}

	selectQuery := "SELECT " + templateSelectColumns + baseQuery + whereClause +
		fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortField, sortDir, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rowsResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, selectQuery, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query templates", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := rowsResult.(*sql.Rows)
	defer rows.Close()

	templates := []TemplateResponse{}
	for rows.Next() {
		t, err := scanTemplateRow(rows)
		if err != nil {
			a.log().Error("Failed to scan template", zap.Error(err))
			continue
		}
		templates = append(templates, t)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate templates", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": templates,
		"total":     total,
		"page":      page,
		"per_page":  perPage,
	})
}

// handleGetTemplate returns a single template with its schedules and prize distributions.
// GET /api/admin/templates/{id}
func (a *App) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequired})
		return
	}

	ctx := r.Context()

	// Fetch template
	var detail TemplateDetailResponse
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		row := a.pool.Replica().QueryRowContext(ctx,
			"SELECT "+templateSelectColumns+" FROM tournament_templates t WHERE t.id = $1",
			templateID,
		)
		t, scanErr := scanTemplateRow(row)
		if scanErr != nil {
			return scanErr
		}
		detail.TemplateResponse = t
		return nil
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
			return
		}
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to get template", zap.String("template_id", templateID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Fetch schedules
	schedulesResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT id, template_id, cron_expression, start_time_utc,
					active_days, weekend_behavior, is_active, created_at, updated_at
				FROM tournament_schedules
				WHERE template_id = $1
				ORDER BY created_at DESC
			`, templateID)
		},
	)
	if err != nil {
		a.log().Error("Failed to fetch template schedules", zap.Error(err))
		detail.Schedules = []ScheduleResponse{}
	} else {
		srows := schedulesResult.(*sql.Rows)
		defer srows.Close()
		detail.Schedules = []ScheduleResponse{}
		for srows.Next() {
			s, scanErr := scanScheduleRow(srows)
			if scanErr != nil {
				a.log().Error("Failed to scan schedule", zap.Error(scanErr))
				continue
			}
			detail.Schedules = append(detail.Schedules, s)
		}
	}

	// Fetch prize distributions
	prizeResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT id, rank, percentage, min_participants
				FROM template_prize_distributions
				WHERE template_id = $1
				ORDER BY rank ASC
			`, templateID)
		},
	)
	if err != nil {
		a.log().Error("Failed to fetch prize distributions", zap.Error(err))
		detail.PrizeDistributions = []PrizeDistributionResponse{}
	} else {
		prows := prizeResult.(*sql.Rows)
		defer prows.Close()
		detail.PrizeDistributions = []PrizeDistributionResponse{}
		for prows.Next() {
			var pd PrizeDistributionResponse
			if scanErr := prows.Scan(&pd.ID, &pd.Rank, &pd.Percentage, &pd.MinParticipants); scanErr != nil {
				a.log().Error("Failed to scan prize distribution", zap.Error(scanErr))
				continue
			}
			detail.PrizeDistributions = append(detail.PrizeDistributions, pd)
		}
	}

	// Fetch entry tiers for this template
	detail.Tiers = a.fetchTiersForTemplate(r.Context(), templateID)
	if detail.Tiers == nil {
		detail.Tiers = []TierResponse{}
	}
	detail.TierCount = len(detail.Tiers)

	writeJSON(w, http.StatusOK, detail)
}

// handleCreateTemplate creates a new tournament template.
// POST /api/admin/templates
func (a *App) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate input
	v := validation.New()

	if strings.TrimSpace(req.Name) == "" {
		v.AddError("name", "required", "name is required")
	}
	if req.DurationMinutes <= 0 {
		v.AddError("duration_minutes", "invalid", "duration_minutes must be greater than 0")
	}
	if !validMarketTypes[req.MarketType] {
		v.AddError("market_type", "invalid", "market_type must be one of: crypto, forex")
	}
	if !validTemplateDurationTypes[req.TemplateDurationType] {
		v.AddError("template_duration_type", "invalid", "template_duration_type must be one of: quick_30m, free_1h, four_hour, daily, weekly, special")
	}
	if req.EntryFee < 0 {
		v.AddError("entry_fee", "invalid", "entry_fee must be >= 0")
	}
	if req.IsFree && req.EntryFee != 0 {
		v.AddError("entry_fee", "invalid", "entry_fee must be 0 for free templates")
	}
	if req.QtyTotal <= 0 {
		v.AddError("qty_total", "invalid", "qty_total must be greater than 0")
	}
	if len(req.Symbols) == 0 {
		v.AddError("symbols", "required", "at least one symbol is required")
	}
	if !validAssetClasses[req.AssetClass] {
		v.AddError("asset_class", "invalid", "asset_class must be one of: forex, crypto, stocks, mixed")
	}
	if req.CommissionRate < 0 || req.CommissionRate > 50 {
		v.AddError("commission_rate", "invalid", "commission_rate must be between 0 and 50")
	}
	if req.MinParticipants < 1 {
		v.AddError("min_participants", "invalid", "min_participants must be at least 1")
	}
	if req.MaxParticipants != nil && *req.MaxParticipants <= 0 {
		v.AddError("max_participants", "invalid", "max_participants must be positive if set")
	}

	validatePrizeDistributions(v, req.PrizeDistributions)

	// Validate entry tiers
	if len(req.EntryTiers) > 20 {
		v.AddError("entry_tiers", "invalid", "maximum 20 tiers per template")
	}
	seenFees := make(map[int64]bool)
	for i, tier := range req.EntryTiers {
		if errMsg := validateTierRequest(tier); errMsg != "" {
			v.AddError("entry_tiers", "invalid", fmt.Sprintf("tier %d: %s", i, errMsg))
		}
		if seenFees[tier.EntryFee] {
			v.AddError("entry_tiers", "duplicate", fmt.Sprintf("tier %d: duplicate entry_fee %d", i, tier.EntryFee))
		}
		seenFees[tier.EntryFee] = true
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Marshal symbols to JSON
	symbolsJSON, _ := json.Marshal(req.Symbols)

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Insert template
	var templateID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tournament_templates (
			name, description, duration_minutes, market_type, template_duration_type,
			entry_fee, entry_fee_cents, qty_total, symbols_json, has_prize,
			is_active, is_free, asset_class, commission_rate, min_participants,
			max_participants, template_key
		) VALUES ($1, $2, $3, $4::market_type, $5::template_duration_type,
			$6, $7, $8, $9::jsonb, $10,
			TRUE, $11, $12::asset_class, $13, $14,
			$15, $16)
		RETURNING id`,
		req.Name, req.Description, req.DurationMinutes, req.MarketType, req.TemplateDurationType,
		req.EntryFee, int(req.EntryFee), req.QtyTotal, string(symbolsJSON), req.HasPrize,
		req.IsFree, req.AssetClass, req.CommissionRate, req.MinParticipants,
		req.MaxParticipants, req.TemplateKey,
	).Scan(&templateID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "template_key") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.TemplateKeyExists})
			return
		}
		a.log().Error("Failed to insert template", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Insert prize distributions
	for _, pd := range req.PrizeDistributions {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
			VALUES ($1, $2, $3, $4)`,
			templateID, pd.Rank, pd.Percentage, pd.MinParticipants,
		)
		if err != nil {
			a.log().Error("Failed to insert prize distribution", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Insert entry tiers if provided
	var createdTiers []TierResponse
	for i, tierReq := range req.EntryTiers {
		tier, tierErr := a.insertTierTx(ctx, tx, templateID, tierReq)
		if tierErr != nil {
			if isUniqueViolation(tierErr) {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": adminMsg.DuplicateTierEntryFee,
				})
				return
			}
			a.log().Error("Failed to insert entry tier", zap.Int("index", i), zap.Error(tierErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		createdTiers = append(createdTiers, tier)
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit template creation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Audit log
	a.logAuditEvent(ctx, actorUserID, "template.created", "tournament_template", templateID,
		map[string]interface{}{"name": req.Name, "market_type": req.MarketType, "tier_count": len(createdTiers)})

	a.log().Info("Tournament template created",
		zap.String("template_id", templateID),
		zap.String("name", req.Name),
		zap.Int("tier_count", len(createdTiers)),
		zap.String("actor", actorUserID))

	// Fetch the created template to return
	var detail TemplateDetailResponse
	row := a.pool.Primary().QueryRowContext(ctx,
		"SELECT "+templateSelectColumns+" FROM tournament_templates t WHERE t.id = $1", templateID,
	)
	t, scanErr := scanTemplateRow(row)
	if scanErr != nil {
		// Template was created successfully, return minimal response
		writeJSON(w, http.StatusCreated, map[string]string{"id": templateID, "message": adminMsg.TemplateCreated})
		return
	}
	detail.TemplateResponse = t
	detail.Schedules = []ScheduleResponse{}
	detail.PrizeDistributions = make([]PrizeDistributionResponse, 0, len(req.PrizeDistributions))
	if createdTiers != nil {
		detail.Tiers = createdTiers
	} else {
		detail.Tiers = []TierResponse{}
	}
	detail.TierCount = len(createdTiers)

	// Re-fetch prize distributions
	prizeRows, _ := a.pool.Primary().QueryContext(ctx, `
		SELECT id, rank, percentage, min_participants
		FROM template_prize_distributions WHERE template_id = $1 ORDER BY rank ASC`, templateID)
	if prizeRows != nil {
		defer prizeRows.Close()
		for prizeRows.Next() {
			var pd PrizeDistributionResponse
			if err := prizeRows.Scan(&pd.ID, &pd.Rank, &pd.Percentage, &pd.MinParticipants); err == nil {
				detail.PrizeDistributions = append(detail.PrizeDistributions, pd)
			}
		}
	}

	writeJSON(w, http.StatusCreated, detail)
}

// handleUpdateTemplate updates a tournament template.
// PUT /api/admin/templates/{id}
func (a *App) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequired})
		return
	}

	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate provided fields
	v := validation.New()

	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		v.AddError("name", "invalid", "name cannot be empty")
	}
	if req.DurationMinutes != nil && *req.DurationMinutes <= 0 {
		v.AddError("duration_minutes", "invalid", "duration_minutes must be greater than 0")
	}
	if req.MarketType != nil && !validMarketTypes[*req.MarketType] {
		v.AddError("market_type", "invalid", "market_type must be one of: crypto, forex")
	}
	if req.TemplateDurationType != nil && !validTemplateDurationTypes[*req.TemplateDurationType] {
		v.AddError("template_duration_type", "invalid", "template_duration_type must be one of: quick_30m, free_1h, four_hour, daily, weekly, special")
	}
	if req.EntryFee != nil && *req.EntryFee < 0 {
		v.AddError("entry_fee", "invalid", "entry_fee must be >= 0")
	}
	if req.IsFree != nil && *req.IsFree && req.EntryFee != nil && *req.EntryFee != 0 {
		v.AddError("entry_fee", "invalid", "entry_fee must be 0 for free templates")
	}
	if req.QtyTotal != nil && *req.QtyTotal <= 0 {
		v.AddError("qty_total", "invalid", "qty_total must be greater than 0")
	}
	if req.AssetClass != nil && !validAssetClasses[*req.AssetClass] {
		v.AddError("asset_class", "invalid", "asset_class must be one of: forex, crypto, stocks, mixed")
	}
	if req.CommissionRate != nil && (*req.CommissionRate < 0 || *req.CommissionRate > 50) {
		v.AddError("commission_rate", "invalid", "commission_rate must be between 0 and 50")
	}
	if req.MinParticipants != nil && *req.MinParticipants < 1 {
		v.AddError("min_participants", "invalid", "min_participants must be at least 1")
	}
	if req.MaxParticipants != nil && *req.MaxParticipants <= 0 {
		v.AddError("max_participants", "invalid", "max_participants must be positive if set")
	}

	validatePrizeDistributions(v, req.PrizeDistributions)

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check if template exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`, templateID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Build dynamic UPDATE
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		updates = append(updates, "name = $"+itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Description != nil {
		updates = append(updates, "description = $"+itoa(argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.DurationMinutes != nil {
		updates = append(updates, "duration_minutes = $"+itoa(argIdx))
		args = append(args, *req.DurationMinutes)
		argIdx++
	}
	if req.MarketType != nil {
		updates = append(updates, "market_type = $"+itoa(argIdx)+"::market_type")
		args = append(args, *req.MarketType)
		argIdx++
	}
	if req.TemplateDurationType != nil {
		updates = append(updates, "template_duration_type = $"+itoa(argIdx)+"::template_duration_type")
		args = append(args, *req.TemplateDurationType)
		argIdx++
	}
	if req.EntryFee != nil {
		updates = append(updates, "entry_fee = $"+itoa(argIdx))
		args = append(args, *req.EntryFee)
		argIdx++
		updates = append(updates, "entry_fee_cents = $"+itoa(argIdx))
		args = append(args, int(*req.EntryFee))
		argIdx++
	}
	if req.QtyTotal != nil {
		updates = append(updates, "qty_total = $"+itoa(argIdx))
		args = append(args, *req.QtyTotal)
		argIdx++
	}
	if len(req.Symbols) > 0 {
		symbolsJSON, _ := json.Marshal(req.Symbols)
		updates = append(updates, "symbols_json = $"+itoa(argIdx)+"::jsonb")
		args = append(args, string(symbolsJSON))
		argIdx++
	}
	if req.HasPrize != nil {
		updates = append(updates, "has_prize = $"+itoa(argIdx))
		args = append(args, *req.HasPrize)
		argIdx++
	}
	if req.IsFree != nil {
		updates = append(updates, "is_free = $"+itoa(argIdx))
		args = append(args, *req.IsFree)
		argIdx++
	}
	if req.AssetClass != nil {
		updates = append(updates, "asset_class = $"+itoa(argIdx)+"::asset_class")
		args = append(args, *req.AssetClass)
		argIdx++
	}
	if req.CommissionRate != nil {
		updates = append(updates, "commission_rate = $"+itoa(argIdx))
		args = append(args, *req.CommissionRate)
		argIdx++
	}
	if req.MinParticipants != nil {
		updates = append(updates, "min_participants = $"+itoa(argIdx))
		args = append(args, *req.MinParticipants)
		argIdx++
	}
	if req.MaxParticipants != nil {
		updates = append(updates, "max_participants = $"+itoa(argIdx))
		args = append(args, *req.MaxParticipants)
		argIdx++
	}

	hasPrizeDistUpdates := len(req.PrizeDistributions) > 0

	if len(updates) == 0 && !hasPrizeDistUpdates {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	// Execute template update
	if len(updates) > 0 {
		query := "UPDATE tournament_templates SET " + joinStrings(updates, ", ") +
			" WHERE id = $" + itoa(argIdx)
		args = append(args, templateID)

		_, err = tx.ExecContext(ctx, query, args...)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "template_key") {
				writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.TemplateKeyExists})
				return
			}
			a.log().Error("Failed to update template", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Update prize distributions if provided (replace strategy)
	if hasPrizeDistUpdates {
		_, err = tx.ExecContext(ctx, `DELETE FROM template_prize_distributions WHERE template_id = $1`, templateID)
		if err != nil {
			a.log().Error("Failed to delete existing prize distributions", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}

		for _, pd := range req.PrizeDistributions {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
				VALUES ($1, $2, $3, $4)`,
				templateID, pd.Rank, pd.Percentage, pd.MinParticipants,
			)
			if err != nil {
				a.log().Error("Failed to insert prize distribution", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit template update", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Audit log
	a.logAuditEvent(ctx, actorUserID, "template.updated", "tournament_template", templateID, req)

	// Fetch updated template
	row := a.pool.Primary().QueryRowContext(ctx,
		"SELECT "+templateSelectColumns+" FROM tournament_templates t WHERE t.id = $1", templateID,
	)
	t, scanErr := scanTemplateRow(row)
	if scanErr != nil {
		writeJSON(w, http.StatusOK, map[string]string{"id": templateID, "message": adminMsg.TemplateUpdated})
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// handleDeleteTemplate soft-deletes a tournament template by setting is_active = false.
// DELETE /api/admin/templates/{id}
func (a *App) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var rowsAffected int64
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		result, execErr := a.pool.Primary().ExecContext(ctx,
			`UPDATE tournament_templates SET is_active = false WHERE id = $1 AND is_active = true`,
			templateID,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to soft-delete template", zap.String("template_id", templateID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if rowsAffected == 0 {
		// Check if template exists but is already inactive
		var exists bool
		_ = a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`, templateID,
		).Scan(&exists)
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.TemplateAlreadyInactive})
		return
	}

	// Audit log
	a.logAuditEvent(ctx, actorUserID, "template.deleted", "tournament_template", templateID,
		map[string]string{"action": "soft_delete"})

	a.log().Info("Tournament template soft-deleted",
		zap.String("template_id", templateID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.TemplateDeactivated})
}
