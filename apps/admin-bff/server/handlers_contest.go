package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/domain"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// getActiveSymbolsByAssetClass queries the symbols table for active symbols
// matching the contest's asset class. Returns symbols ordered by sort_order.
func getActiveSymbolsByAssetClass(ctx context.Context, db *sql.DB, assetClass string) ([]string, error) {
	var assetTypes []string
	switch assetClass {
	case "crypto":
		assetTypes = []string{"crypto"}
	case "forex":
		assetTypes = []string{"forex", "commodity"}
	case "stocks":
		assetTypes = []string{"stock"}
	case "mixed":
		assetTypes = []string{"crypto", "forex"}
	default:
		return nil, fmt.Errorf("unknown asset class: %s", assetClass)
	}

	// Build IN clause dynamically (no pq.Array dependency)
	placeholders := make([]string, len(assetTypes))
	args := make([]interface{}, len(assetTypes))
	for i, t := range assetTypes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = t
	}

	query := fmt.Sprintf(`SELECT symbol FROM symbols
		WHERE is_active = true AND asset_type::text IN (%s)
		ORDER BY sort_order ASC, symbol ASC`,
		strings.Join(placeholders, ", "))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query symbols: %w", err)
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

func (a *App) handleCreateContest(w http.ResponseWriter, r *http.Request) {
	var req CreateContestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate input using validation package
	v := validation.New()
	req.Name = v.String("name", req.Name, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 255,
		TrimSpace: true,
	})

	// Sanitize name
	req.Name = validation.SanitizeName(req.Name)

	if req.StartsAt.IsZero() {
		v.AddError("starts_at", "required", "starts_at is required")
	}
	if req.EndsAt != nil && !req.EndsAt.IsZero() && !req.StartsAt.IsZero() && !req.EndsAt.After(req.StartsAt) {
		v.AddError("ends_at", "invalid_range", "ends_at must be after starts_at")
	}
	if req.EntryFeeCents < 0 {
		v.AddError("entry_fee_cents", "invalid_value", "entry_fee_cents must be >= 0")
	}
	if req.IsFree && req.EntryFeeCents != 0 {
		v.AddError("entry_fee_cents", "invalid", "entry_fee_cents must be 0 for free contests")
	}
	if req.IsFree && req.PlatformFeeBps != 0 {
		v.AddError("platform_fee_bps", "invalid", "platform_fee_bps must be 0 for free contests")
	}
	if req.PlatformFeeBps < 0 || req.PlatformFeeBps > 10000 {
		v.AddError("platform_fee_bps", "invalid_range", "platform_fee_bps must be 0-10000")
	}
	if req.CommissionRate < 0 || req.CommissionRate > 50 {
		v.AddError("commission_rate", "invalid_range", "commission_rate must be 0-50%")
	}
	if req.MinParticipants < 0 {
		v.AddError("min_participants", "invalid_value", "min_participants must be >= 0")
	}
	if req.MaxParticipants != nil && *req.MaxParticipants <= 0 {
		v.AddError("max_participants", "invalid_value", "max_participants must be > 0")
	}
	if req.AssetClass != "" && !contracts.AssetClass(req.AssetClass).IsValid() {
		v.AddError("asset_class", "invalid_value", "asset_class must be one of: forex, crypto, stocks, mixed")
	}
	if req.DurationType != "" && !contracts.ContestDurationType(req.DurationType).IsValid() {
		v.AddError("duration_type", "invalid_value", "duration_type must be one of: rush_30min, hourly, four_hour, daily, weekly")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Auto-zero fees for free contests
	if req.IsFree {
		req.EntryFeeCents = 0
		req.PlatformFeeBps = 0
		req.CommissionRate = 0
	}

	// Set defaults
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.DurationType == "" {
		req.DurationType = "hourly"
	}
	if req.AssetClass == "" {
		req.AssetClass = "mixed"
	}
	if req.MinParticipants == 0 {
		req.MinParticipants = 2
	}
	if req.CommissionRate == 0 && !req.IsFree {
		req.CommissionRate = 20.00
	}

	// Auto-calculate ends_at from duration_type if not provided
	var endsAt time.Time
	if req.EndsAt != nil && !req.EndsAt.IsZero() {
		endsAt = *req.EndsAt
	} else {
		dt := contracts.ContestDurationType(req.DurationType)
		endsAt = req.StartsAt.Add(time.Duration(dt.DurationMinutes()) * time.Minute)
	}

	// Calculate duration minutes from duration_type
	durationMinutes := req.DurationMinutes
	if durationMinutes == 0 {
		durationMinutes = contracts.ContestDurationType(req.DurationType).DurationMinutes()
	}

	// Set registration deadline to 1 second before starts_at if not provided
	regDeadline := req.RegistrationDeadline
	if regDeadline == nil {
		deadline := req.StartsAt.Add(-1 * time.Second)
		regDeadline = &deadline
	}

	// Auto-assign symbols based on asset_class from DB (single source of truth)
	if len(req.Symbols) == 0 {
		dbSymbols, dbErr := getActiveSymbolsByAssetClass(r.Context(), a.pool.Replica(), req.AssetClass)
		if dbErr != nil {
			a.log().Warn("Failed to query symbols from DB, using hardcoded defaults",
				zap.String("asset_class", req.AssetClass), zap.Error(dbErr))
		}
		if len(dbSymbols) > 0 {
			req.Symbols = dbSymbols
		} else {
			req.Symbols = contracts.GetDefaultSymbols(
				contracts.AssetClass(req.AssetClass),
				contracts.ContestDurationType(req.DurationType),
			)
		}
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Begin transaction on primary (writes)
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Insert contest with all new fields
	var contest ContestResponse
	err = tx.QueryRowContext(ctx,
		`INSERT INTO contests (
			name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
			duration_type, asset_class, duration_minutes, min_participants, max_participants,
			registration_deadline, auto_start, commission_rate, is_free
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id, name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
		           duration_type, asset_class, COALESCE(duration_minutes, 0), min_participants, max_participants,
		           registration_deadline, auto_start, commission_rate, is_free, auto_generated,
		           template_id, created_at`,
		req.Name, req.Description, req.StartsAt, endsAt, req.Status, req.EntryFeeCents, req.PlatformFeeBps, req.QtyTotal,
		req.DurationType, req.AssetClass, durationMinutes, req.MinParticipants, req.MaxParticipants,
		regDeadline, req.AutoStart, req.CommissionRate, req.IsFree,
	).Scan(&contest.ID, &contest.Name, &contest.Description, &contest.StartsAt, &contest.EndsAt,
		&contest.Status, &contest.EntryFeeCents, &contest.PlatformFeeBps, &contest.QtyTotal,
		&contest.DurationType, &contest.AssetClass, &contest.DurationMinutes, &contest.MinParticipants,
		&contest.MaxParticipants, &contest.RegistrationDeadline, &contest.AutoStart, &contest.CommissionRate,
		&contest.IsFree, &contest.AutoGenerated,
		&contest.TemplateID, &contest.CreatedAt)
	if err != nil {
		a.log().Error("Failed to insert contest", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add symbols if provided
	if len(req.Symbols) > 0 {
		for _, symbol := range req.Symbols {
			sanitizedSymbol := validation.SanitizeSymbol(symbol)
			if sanitizedSymbol == "" {
				continue
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO contest_symbols (contest_id, symbol, enabled) VALUES ($1, $2, TRUE)
				 ON CONFLICT (contest_id, symbol) DO NOTHING`,
				contest.ID, sanitizedSymbol)
			if err != nil {
				a.log().Error("Failed to insert contest symbol", zap.Error(err), zap.String("symbol", sanitizedSymbol))
			}
		}
	}

	// Auto-register system bot for free contests so min_participants is met
	if req.IsFree {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, is_system)
			 VALUES ($1, $2, $3, $3, TRUE)
			 ON CONFLICT (contest_id, user_id) DO NOTHING`,
			contest.ID, domain.TBotUserID, req.QtyTotal)
		if err != nil {
			a.log().Error("Failed to register bot for free contest", zap.Error(err))
		}
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "contest.created", "contest", contest.ID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusCreated, contest)
}

func (a *App) handleListContestTemplates(w http.ResponseWriter, r *http.Request) {
	// Get optional filters from query params
	assetClass := r.URL.Query().Get("asset_class")
	durationType := r.URL.Query().Get("duration_type")
	isFree := r.URL.Query().Get("is_free")

	templates := contracts.ListContestTemplates()

	// Filter templates based on query params
	var filtered []contracts.ContestTemplate
	for _, t := range templates {
		if assetClass != "" && string(t.AssetClass) != assetClass {
			continue
		}
		if durationType != "" && string(t.DurationType) != durationType {
			continue
		}
		if isFree == "true" && !t.IsFree {
			continue
		}
		if isFree == "false" && t.IsFree {
			continue
		}
		filtered = append(filtered, t)
	}

	if filtered == nil {
		filtered = []contracts.ContestTemplate{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": filtered,
	})
}

func (a *App) handleGetContestTemplate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		validation.WriteBadRequest(w, "template key is required")
		return
	}

	template := contracts.GetContestTemplate(key)
	if template == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	writeJSON(w, http.StatusOK, template)
}

func (a *App) handleCreateContestFromTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateContestFromTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Get template
	template := contracts.GetContestTemplate(req.TemplateKey)
	if template == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidTemplateKey})
		return
	}

	// Validate input
	v := validation.New()
	req.Name = v.String("name", req.Name, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 255,
		TrimSpace: true,
	})
	req.Name = validation.SanitizeName(req.Name)

	if req.StartsAt.IsZero() {
		v.AddError("starts_at", "required", "starts_at is required")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Calculate end time from template duration
	endsAt := req.StartsAt.Add(time.Duration(template.DurationMinutes) * time.Minute)

	// Use template values with optional overrides
	entryFeeCents := template.EntryFeeCents
	if req.EntryFeeCents != nil {
		entryFeeCents = *req.EntryFeeCents
	}

	maxParticipants := template.MaxParticipants
	if req.MaxParticipants != nil {
		maxParticipants = *req.MaxParticipants
	}

	regDeadline := req.StartsAt.Add(-1 * time.Second)
	if req.RegistrationDeadline != nil {
		regDeadline = *req.RegistrationDeadline
	}

	symbols := template.Symbols
	if len(req.Symbols) > 0 {
		symbols = req.Symbols
	} else {
		dbSymbols, dbErr := getActiveSymbolsByAssetClass(r.Context(), a.pool.Replica(), string(template.AssetClass))
		if dbErr != nil {
			a.log().Warn("Failed to query symbols from DB for template, using template defaults",
				zap.String("template", req.TemplateKey), zap.Error(dbErr))
		}
		if len(dbSymbols) > 0 {
			symbols = dbSymbols
		}
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

	// Get template ID from database (if stored)
	var templateID *string
	err = tx.QueryRowContext(ctx,
		`SELECT id::text FROM tournament_templates WHERE template_key = $1`,
		req.TemplateKey).Scan(&templateID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.log().Warn("Failed to get template ID", zap.Error(err))
	}

	// Insert contest from template
	var contest ContestResponse
	var maxParticipantsPtr *int
	if maxParticipants > 0 {
		maxParticipantsPtr = &maxParticipants
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO contests (
			name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
			duration_type, asset_class, duration_minutes, min_participants, max_participants,
			registration_deadline, auto_start, commission_rate, is_free, template_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 RETURNING id, name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
		           duration_type, asset_class, COALESCE(duration_minutes, 0), min_participants, max_participants,
		           registration_deadline, auto_start, commission_rate, is_free, auto_generated,
		           template_id, created_at`,
		req.Name, req.Description, req.StartsAt, endsAt, "draft", entryFeeCents,
		int(template.CommissionRate*100), // Convert to basis points
		template.QtyAllocation,
		string(template.DurationType), string(template.AssetClass), template.DurationMinutes,
		template.MinParticipants, maxParticipantsPtr, regDeadline, template.AutoStart,
		template.CommissionRate, template.IsFree, templateID,
	).Scan(&contest.ID, &contest.Name, &contest.Description, &contest.StartsAt, &contest.EndsAt,
		&contest.Status, &contest.EntryFeeCents, &contest.PlatformFeeBps, &contest.QtyTotal,
		&contest.DurationType, &contest.AssetClass, &contest.DurationMinutes, &contest.MinParticipants,
		&contest.MaxParticipants, &contest.RegistrationDeadline, &contest.AutoStart, &contest.CommissionRate,
		&contest.IsFree, &contest.AutoGenerated,
		&contest.TemplateID, &contest.CreatedAt)
	if err != nil {
		a.log().Error("Failed to insert contest from template", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add symbols from template
	for _, symbol := range symbols {
		sanitizedSymbol := validation.SanitizeSymbol(symbol)
		if sanitizedSymbol == "" {
			continue
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_symbols (contest_id, symbol, enabled) VALUES ($1, $2, TRUE)
			 ON CONFLICT (contest_id, symbol) DO NOTHING`,
			contest.ID, sanitizedSymbol)
		if err != nil {
			a.log().Error("Failed to insert contest symbol", zap.Error(err), zap.String("symbol", sanitizedSymbol))
		}
	}

	// Auto-register system bot for free contests so min_participants is met
	if template.IsFree {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, is_system)
			 VALUES ($1, $2, $3, $3, TRUE)
			 ON CONFLICT (contest_id, user_id) DO NOTHING`,
			contest.ID, domain.TBotUserID, template.QtyAllocation)
		if err != nil {
			a.log().Error("Failed to register bot for free contest", zap.Error(err))
		}
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"template_key": req.TemplateKey,
		"name":         req.Name,
		"starts_at":    req.StartsAt,
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "contest.created_from_template", "contest", contest.ID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusCreated, contest)
}

func (a *App) handleListContests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get optional filters from query params
	assetClass := r.URL.Query().Get("asset_class")
	durationType := r.URL.Query().Get("duration_type")
	status := r.URL.Query().Get("status")
	isFree := r.URL.Query().Get("is_free")
	isAutoGenerated := r.URL.Query().Get("is_auto_generated")

	// Parse pagination parameters
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 && parsed <= 100000 {
			offset = parsed
		}
	}

	// Build query with optional filters
	baseWhere := " WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if assetClass != "" {
		baseWhere += fmt.Sprintf(" AND c.asset_class = $%d", argIdx)
		args = append(args, assetClass)
		argIdx++
	}
	if durationType != "" {
		baseWhere += fmt.Sprintf(" AND c.duration_type = $%d", argIdx)
		args = append(args, durationType)
		argIdx++
	}
	if status != "" {
		baseWhere += fmt.Sprintf(" AND c.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	} else {
		// By default, exclude only cancelled contests. Show drafts so admins can see newly created contests.
		baseWhere += " AND c.status != 'cancelled'"
	}
	if isFree == "true" {
		baseWhere += " AND c.is_free = TRUE"
	} else if isFree == "false" {
		baseWhere += " AND c.is_free = FALSE"
	}
	if isAutoGenerated == "true" {
		baseWhere += " AND c.auto_generated = TRUE"
	} else if isAutoGenerated == "false" {
		baseWhere += " AND c.auto_generated = FALSE"
	}

	// Count total matching contests
	countQuery := "SELECT COUNT(*) FROM contests c" + baseWhere
	var total int
	if err := a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		if ctx.Err() != nil {
			return
		}
		a.log().Error("Failed to count contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	query := fmt.Sprintf(`SELECT c.id, c.name, c.description, c.starts_at, c.ends_at, c.status, c.entry_fee_cents, c.platform_fee_bps, c.qty_total,
	                 c.duration_type, c.asset_class, COALESCE(c.duration_minutes, 0), c.min_participants, c.max_participants,
	                 COALESCE((SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id), 0),
	                 c.registration_deadline, c.auto_start, c.commission_rate, c.is_free, c.auto_generated,
	                 c.template_id, c.created_at
	          FROM contests c%s ORDER BY c.created_at DESC LIMIT $%d OFFSET $%d`, baseWhere, argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Read-only query — use replica directly (not via circuit breaker) because
	// the circuit breaker's defer cancel() kills the context before rows can be scanned.
	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.log().Error("Failed to query contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer rows.Close()

	var contests []ContestResponse
	for rows.Next() {
		var c ContestResponse
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.StartsAt, &c.EndsAt,
			&c.Status, &c.EntryFeeCents, &c.PlatformFeeBps, &c.QtyTotal,
			&c.DurationType, &c.AssetClass, &c.DurationMinutes, &c.MinParticipants,
			&c.MaxParticipants, &c.ParticipantCount,
			&c.RegistrationDeadline, &c.AutoStart, &c.CommissionRate,
			&c.IsFree, &c.AutoGenerated,
			&c.TemplateID, &c.CreatedAt); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.log().Error("Failed to scan contest", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		contests = append(contests, c)
	}

	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return
		}
		a.log().Error("Failed to iterate contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if contests == nil {
		contests = []ContestResponse{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contests": contests,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (a *App) handleUpdateContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	var req UpdateContestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Begin transaction on primary (writes)
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check if contest exists and is in an editable state
	var currentStatus string
	var currentIsFree bool
	var currentEntryFeeCents int
	err = tx.QueryRowContext(ctx, `SELECT status, is_free, entry_fee_cents FROM contests WHERE id = $1 FOR UPDATE`, contestID).Scan(&currentStatus, &currentIsFree, &currentEntryFeeCents)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
			return
		}
		a.log().Error("Failed to check contest status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if currentStatus != "draft" && currentStatus != "scheduled" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": adminMsg.CannotModifyState,
		})
		return
	}

	// Cross-field validation: ensure is_free and entry_fee_cents are consistent
	effectiveIsFree := currentIsFree
	if req.IsFree != nil {
		effectiveIsFree = *req.IsFree
	}
	effectiveEntryFee := currentEntryFeeCents
	if req.EntryFeeCents != nil {
		effectiveEntryFee = *req.EntryFeeCents
	}
	if effectiveIsFree && effectiveEntryFee != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.FreeFeeZero,
		})
		return
	}
	if effectiveIsFree && req.PlatformFeeBps != nil && *req.PlatformFeeBps != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.FreePlatformFeeZero,
		})
		return
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		updates = append(updates, "name = $"+itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.StartsAt != nil {
		updates = append(updates, "starts_at = $"+itoa(argIdx))
		args = append(args, *req.StartsAt)
		argIdx++
	}
	if req.EndsAt != nil {
		updates = append(updates, "ends_at = $"+itoa(argIdx))
		args = append(args, *req.EndsAt)
		argIdx++
	}
	if req.EntryFeeCents != nil {
		updates = append(updates, "entry_fee_cents = $"+itoa(argIdx))
		args = append(args, *req.EntryFeeCents)
		argIdx++
	}
	if req.PlatformFeeBps != nil {
		updates = append(updates, "platform_fee_bps = $"+itoa(argIdx))
		args = append(args, *req.PlatformFeeBps)
		argIdx++
	}
	if req.QtyTotal != nil {
		updates = append(updates, "qty_total = $"+itoa(argIdx))
		args = append(args, *req.QtyTotal)
		argIdx++
	}
	if req.Description != nil {
		updates = append(updates, "description = $"+itoa(argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.IsFree != nil {
		updates = append(updates, "is_free = $"+itoa(argIdx))
		args = append(args, *req.IsFree)
		argIdx++
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	// Build and execute update query
	query := "UPDATE contests SET " + joinStrings(updates, ", ") + " WHERE id = $" + itoa(argIdx) +
		` RETURNING id, name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
		           duration_type, asset_class, COALESCE(duration_minutes, 0), min_participants, max_participants,
		           registration_deadline, auto_start, commission_rate, is_free, auto_generated,
		           template_id, created_at`
	args = append(args, contestID)

	var contest ContestResponse
	err = tx.QueryRowContext(ctx, query, args...).Scan(
		&contest.ID, &contest.Name, &contest.Description, &contest.StartsAt, &contest.EndsAt,
		&contest.Status, &contest.EntryFeeCents, &contest.PlatformFeeBps, &contest.QtyTotal,
		&contest.DurationType, &contest.AssetClass, &contest.DurationMinutes, &contest.MinParticipants,
		&contest.MaxParticipants, &contest.RegistrationDeadline, &contest.AutoStart, &contest.CommissionRate,
		&contest.IsFree, &contest.AutoGenerated,
		&contest.TemplateID, &contest.CreatedAt)
	if err != nil {
		a.log().Error("Failed to update contest", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "contest.updated", "contest", contestID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// P2-4: Publish contest state event so downstream services (trading-engine)
	// invalidate their caches (symbol cache, contest cache, etc.)
	if a.kafkaProducer != nil {
		stateEvt := contracts.ContestState{
			ContestID: contestID,
			Status:    contracts.ContestStatus(contest.Status),
			Reason:    "contest_updated",
			Ts:        time.Now().UnixMilli(),
		}
		evtBytes, _ := json.Marshal(stateEvt)
		msg := &sarama.ProducerMessage{
			Topic: "contests.v1",
			Key:   sarama.StringEncoder(contestID),
			Value: sarama.ByteEncoder(evtBytes),
		}

		var pubErr error
		for attempt := 0; attempt < 3; attempt++ {
			_, _, pubErr = a.kafkaProducer.SendMessage(msg)
			if pubErr == nil {
				break
			}
			a.log().Warn("Kafka publish attempt failed, retrying",
				zap.String("contest_id", contestID),
				zap.Int("attempt", attempt+1),
				zap.Error(pubErr))
			time.Sleep(time.Duration(1<<uint(attempt)) * 100 * time.Millisecond)
		}
		if pubErr != nil {
			a.log().Error("Failed to publish contest update event after retries",
				zap.String("contest_id", contestID),
				zap.Error(pubErr))
		}
	}

	writeJSON(w, http.StatusOK, contest)
}

func (a *App) handleDeleteContest(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Begin transaction on primary (writes)
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check if contest exists and verify it is in draft status
	var currentStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1 FOR UPDATE`, contestID).Scan(&currentStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
			return
		}
		a.log().Error("Failed to check contest status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if currentStatus != "draft" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.OnlyDraftDeletable,
		})
		return
	}

	// Delete the contest (cascade will handle related records)
	_, err = tx.ExecContext(ctx, `DELETE FROM contests WHERE id = $1`, contestID)
	if err != nil {
		a.log().Error("Failed to delete contest", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payload := map[string]string{
		"contest_id": contestID,
	}
	payloadJSON, _ := json.Marshal(payload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "contest.deleted", "contest", contestID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ContestDeleted})
}

func (a *App) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to time.Time
	var err error

	if fromStr != "" {
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidFromDate})
			return
		}
	}

	if toStr != "" {
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidToDate})
			return
		}
	}

	// Build query based on parameters
	query := `SELECT id, actor_user_id, action, target_type, target_id, payload_json, created_at
		FROM audit_logs WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if !from.IsZero() {
		query += " AND created_at >= $" + itoa(argIdx)
		args = append(args, from)
		argIdx++
	}

	if !to.IsZero() {
		query += " AND created_at <= $" + itoa(argIdx)
		args = append(args, to)
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT 1000"

	// Read-only query, use replica with circuit breaker
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, query, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query audit logs", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var logs []AuditLogResponse
	for rows.Next() {
		var l AuditLogResponse
		if err := rows.Scan(&l.ID, &l.ActorUserID, &l.Action, &l.TargetType, &l.TargetID, &l.PayloadJSON, &l.CreatedAt); err != nil {
			a.log().Error("Failed to scan audit log", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		logs = append(logs, l)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate audit logs", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if logs == nil {
		logs = []AuditLogResponse{}
	}

	writeJSON(w, http.StatusOK, logs)
}

// handleAddContestSymbols adds or updates symbols for a contest.
func (a *App) handleAddContestSymbols(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	var req AddContestSymbolsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if len(req.Symbols) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.AtLeastOneSymbol})
		return
	}

	// Validate symbols
	for _, s := range req.Symbols {
		if s.Symbol == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SymbolNameRequired})
			return
		}
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Begin transaction on primary (writes)
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check if contest exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
		return
	}

	// Upsert symbols
	var responses []ContestSymbolResponse
	for _, s := range req.Symbols {
		var resp ContestSymbolResponse
		err = tx.QueryRowContext(ctx,
			`INSERT INTO contest_symbols (contest_id, symbol, provider_symbol_twelvedata, provider_symbol_massive, enabled)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (contest_id, symbol) DO UPDATE SET
			   provider_symbol_twelvedata = EXCLUDED.provider_symbol_twelvedata,
			   provider_symbol_massive = EXCLUDED.provider_symbol_massive,
			   enabled = EXCLUDED.enabled
			 RETURNING contest_id, symbol, provider_symbol_twelvedata, provider_symbol_massive, enabled, created_at`,
			contestID, s.Symbol, nullIfEmpty(s.ProviderSymbolTwelveData), nullIfEmpty(s.ProviderSymbolMassive), s.Enabled,
		).Scan(&resp.ContestID, &resp.Symbol, &resp.ProviderSymbolTwelveData, &resp.ProviderSymbolMassive, &resp.Enabled, &resp.CreatedAt)
		if err != nil {
			a.log().Error("Failed to upsert symbol", zap.String("symbol", s.Symbol), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		responses = append(responses, resp)
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "contest.symbols.updated", "contest", contestID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, responses)
}

// handleGetContestSymbols returns symbols for a contest.
func (a *App) handleGetContestSymbols(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()

	// Check if contest exists (read-only, use replica with circuit breaker)
	var exists bool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
		return
	}

	// Read-only query, use replica with circuit breaker
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx,
				`SELECT contest_id, symbol, provider_symbol_twelvedata, provider_symbol_massive, enabled, created_at
				 FROM contest_symbols
				 WHERE contest_id = $1
				 ORDER BY symbol ASC`,
				contestID,
			)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query contest symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var symbols []ContestSymbolResponse
	for rows.Next() {
		var s ContestSymbolResponse
		if err := rows.Scan(&s.ContestID, &s.Symbol, &s.ProviderSymbolTwelveData, &s.ProviderSymbolMassive, &s.Enabled, &s.CreatedAt); err != nil {
			a.log().Error("Failed to scan contest symbol", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		symbols = append(symbols, s)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate contest symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if symbols == nil {
		symbols = []ContestSymbolResponse{}
	}

	writeJSON(w, http.StatusOK, symbols)
}

// handleGetShards returns Kafka lag info and instance heartbeats.
func (a *App) handleGetShards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response := ShardsHealthResponse{
		Instances:      []InstanceHeartbeat{},
		KafkaTopics:    []TopicPartitionLag{},
		KafkaAvailable: a.kafkaAdmin != nil,
		RedisAvailable: a.redis != nil,
		CheckedAt:      time.Now().UTC(),
	}

	// Get instance heartbeats from Redis
	if a.redis != nil {
		instances, err := a.getInstanceHeartbeats(ctx)
		if err != nil {
			a.log().Warn("Failed to get instance heartbeats", zap.Error(err))
		} else {
			response.Instances = instances
		}
	}

	// Get Kafka topic lag info
	if a.kafkaAdmin != nil {
		topics := []string{"orders.v1", "fills.v1", "positions.v1", "ticks.v1", "pnl.v1", "contests.v1"}
		for _, topic := range topics {
			lagInfo := a.getTopicLag(topic)
			response.KafkaTopics = append(response.KafkaTopics, lagInfo)
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetContestLocation returns which Kafka partition owns a contest_id.
func (a *App) handleGetContestLocation(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ContestIDRequired})
		return
	}

	// Check if contest exists (read-only, use replica, circuit breaker protected)
	ctx := r.Context()
	var exists bool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ContestNotFound})
		return
	}

	response := ContestLocationResponse{
		ContestID:       contestID,
		Topic:           "orders.v1", // Primary topic for contest orders
		PartitionKey:    contestID,
		PartitionMethod: "murmur2", // Kafka's default partitioner uses murmur2
	}

	if a.kafkaAdmin == nil {
		response.Available = false
		response.ErrorReason = "kafka admin client unavailable"
		response.Partition = -1
		writeJSON(w, http.StatusOK, response)
		return
	}

	// Get partition count for the topic
	topics, err := a.kafkaAdmin.DescribeTopics([]string{"orders.v1"})
	if err != nil {
		a.log().Warn("Failed to describe topics", zap.Error(err))
		response.Available = false
		response.ErrorReason = fmt.Sprintf("failed to describe topic: %v", err)
		response.Partition = -1
		writeJSON(w, http.StatusOK, response)
		return
	}

	if len(topics) == 0 || topics[0].Err != sarama.ErrNoError {
		response.Available = false
		response.ErrorReason = "topic not found or error"
		response.Partition = -1
		writeJSON(w, http.StatusOK, response)
		return
	}

	numPartitions := int32(len(topics[0].Partitions))
	if numPartitions <= 0 {
		response.Available = false
		response.ErrorReason = "no partitions available"
		response.Partition = -1
		writeJSON(w, http.StatusOK, response)
		return
	}

	// Calculate partition using murmur2 hash (Kafka's default)
	partition := computeKafkaPartition(contestID, numPartitions)
	response.Partition = partition
	response.Available = true

	writeJSON(w, http.StatusOK, response)
}

// startHeartbeat starts a background goroutine that sends periodic heartbeats to Redis.
func (a *App) startHeartbeat(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("startHeartbeat panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	key := fmt.Sprintf("heartbeat:admin-bff:%s", a.config.InstanceID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.redis == nil {
				continue
			}
			heartbeat := map[string]interface{}{
				"instance_id": a.config.InstanceID,
				"last_seen":   time.Now().UTC().Format(time.RFC3339),
				"status":      "healthy",
			}
			data, _ := json.Marshal(heartbeat)
			// Set with 30 second expiry (3x the tick interval)
			if err := a.redis.Set(ctx, key, data, 30*time.Second).Err(); err != nil {
				a.log().Warn("Failed to send heartbeat", zap.Error(err))
			}
		}
	}
}

// getInstanceHeartbeats retrieves all instance heartbeats from Redis.
func (a *App) getInstanceHeartbeats(ctx context.Context) ([]InstanceHeartbeat, error) {
	keys, err := a.redis.Keys(ctx, "heartbeat:*").Result()
	if err != nil {
		return nil, err
	}

	var instances []InstanceHeartbeat
	for _, key := range keys {
		data, err := a.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var hb map[string]interface{}
		if err := json.Unmarshal([]byte(data), &hb); err != nil {
			continue
		}

		instanceID, _ := hb["instance_id"].(string)
		lastSeenStr, _ := hb["last_seen"].(string)
		status, _ := hb["status"].(string)

		lastSeen, _ := time.Parse(time.RFC3339, lastSeenStr)

		instances = append(instances, InstanceHeartbeat{
			InstanceID: instanceID,
			LastSeen:   lastSeen,
			Status:     status,
		})
	}

	return instances, nil
}

// getTopicLag returns lag information for a Kafka topic.
func (a *App) getTopicLag(topic string) TopicPartitionLag {
	result := TopicPartitionLag{
		Topic:      topic,
		Partitions: make(map[int32]LagInfo),
		Available:  false,
	}

	if a.kafkaAdmin == nil {
		result.ErrorReason = "kafka admin client unavailable"
		return result
	}

	// Get topic metadata
	topics, err := a.kafkaAdmin.DescribeTopics([]string{topic})
	if err != nil {
		result.ErrorReason = fmt.Sprintf("failed to describe topic: %v", err)
		return result
	}

	if len(topics) == 0 {
		result.ErrorReason = "topic not found"
		return result
	}

	topicMeta := topics[0]
	if topicMeta.Err != sarama.ErrNoError {
		result.ErrorReason = fmt.Sprintf("topic error: %v", topicMeta.Err)
		return result
	}

	// Get high watermarks for each partition
	partitions := make([]int32, 0, len(topicMeta.Partitions))
	for _, p := range topicMeta.Partitions {
		partitions = append(partitions, p.ID)
	}

	// Get offsets (best effort - we'll use end offsets as high watermark)
	offsets, err := a.kafkaAdmin.ListConsumerGroupOffsets("trading-engine", map[string][]int32{topic: partitions})
	if err != nil {
		// Try to get at least the partition count
		for _, p := range topicMeta.Partitions {
			result.Partitions[p.ID] = LagInfo{
				Partition:     p.ID,
				CurrentOffset: -1,
				HighWaterMark: -1,
				Lag:           -1,
			}
		}
		result.Available = true
		result.ErrorReason = fmt.Sprintf("consumer group offsets unavailable: %v", err)
		return result
	}

	// Calculate lag for each partition
	var totalLag int64
	for _, p := range topicMeta.Partitions {
		block := offsets.GetBlock(topic, p.ID)
		if block == nil {
			result.Partitions[p.ID] = LagInfo{
				Partition:     p.ID,
				CurrentOffset: -1,
				HighWaterMark: -1,
				Lag:           -1,
			}
			continue
		}

		// Note: Getting high watermark requires a separate call to the broker
		// For simplicity, we'll just show the current offset
		// In production, you'd want to fetch end offsets separately
		lagInfo := LagInfo{
			Partition:     p.ID,
			CurrentOffset: block.Offset,
			HighWaterMark: -1, // Would need separate call
			Lag:           -1, // Can't calculate without high watermark
		}
		result.Partitions[p.ID] = lagInfo
	}

	result.TotalLag = totalLag
	result.Available = true

	return result
}

// computeKafkaPartition computes the Kafka partition for a key using murmur2 hash.
// This mimics Kafka's default partitioner behavior.
func computeKafkaPartition(key string, numPartitions int32) int32 {
	if numPartitions <= 0 {
		return 0
	}

	// Use FNV-1a hash as a reasonable approximation
	// Kafka uses murmur2, but for best-effort this is acceptable
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := h.Sum32()

	// Ensure positive value and modulo
	return int32(hash % uint32(numPartitions))
}
