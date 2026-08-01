package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/domain"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// =============================================================================
// Special Tournament Types
// =============================================================================

// CreateSpecialTournamentRequest represents a request to create a special/event tournament.
type CreateSpecialTournamentRequest struct {
	Name               string                   `json:"name"`
	Description        *string                  `json:"description,omitempty"`
	MarketType         string                   `json:"market_type"`
	EntryFee           int64                    `json:"entry_fee"`
	StartTime          time.Time                `json:"start_time"`
	EndTime            time.Time                `json:"end_time"`
	PrizePool          *int64                   `json:"prize_pool,omitempty"`
	IsSponsored        bool                     `json:"is_sponsored"`
	QtyTotal           int64                    `json:"qty_total"`
	Symbols            []string                 `json:"symbols"`
	CommissionRate     float64                  `json:"commission_rate"`
	BannerImageURL     *string                  `json:"banner_image_url,omitempty"`
	PrizeDistributions []PrizeDistributionInput `json:"prize_distributions,omitempty"`
}

// =============================================================================
// Tournament Stats Types
// =============================================================================

// TournamentOverviewStats represents aggregated tournament statistics.
type TournamentOverviewStats struct {
	ActiveTournaments     int64     `json:"active_tournaments"`
	TotalParticipants     int64     `json:"total_participants"`
	TotalPrizePool        int64     `json:"total_prize_pool"`
	CommissionToday       int64     `json:"commission_today"`
	CommissionThisWeek    int64     `json:"commission_this_week"`
	CommissionThisMonth   int64     `json:"commission_this_month"`
	AvgParticipantsByType []TypeAvg `json:"avg_participants_by_type"`
}

// TypeAvg represents average participants for a duration type.
type TypeAvg struct {
	DurationType    string  `json:"duration_type"`
	AvgParticipants float64 `json:"avg_participants"`
}

// TournamentDetailStats represents detailed statistics for a single tournament.
type TournamentDetailStats struct {
	ContestID          string                `json:"contest_id"`
	Name               string                `json:"name"`
	Status             string                `json:"status"`
	ParticipantCount   int64                 `json:"participant_count"`
	EntryFeeRevenue    int64                 `json:"entry_fee_revenue"`
	CommissionAmount   int64                 `json:"commission_amount"`
	NetPrizePool       int64                 `json:"net_prize_pool"`
	StartsAt           time.Time             `json:"starts_at"`
	EndsAt             time.Time             `json:"ends_at"`
	EntryFeeCents      int                   `json:"entry_fee_cents"`
	CommissionRate     float64               `json:"commission_rate"`
	DurationType       string                `json:"duration_type"`
	AssetClass         string                `json:"asset_class"`
	ParticipantHistory []ParticipantSnapshot `json:"participant_history"`
}

// ParticipantSnapshot represents a point-in-time participant count.
type ParticipantSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

// =============================================================================
// Special Tournament Handler
// =============================================================================

// handleCreateSpecialTournament creates a one-off special/event tournament.
// POST /api/admin/tournaments/special
func (a *App) handleCreateSpecialTournament(w http.ResponseWriter, r *http.Request) {
	var req CreateSpecialTournamentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate input
	v := validation.New()

	if strings.TrimSpace(req.Name) == "" {
		v.AddError("name", "required", "name is required")
	}
	if req.StartTime.IsZero() {
		v.AddError("start_time", "required", "start_time is required")
	}
	if req.EndTime.IsZero() {
		v.AddError("end_time", "required", "end_time is required")
	}
	if !req.StartTime.IsZero() && !req.EndTime.IsZero() && !req.EndTime.After(req.StartTime) {
		v.AddError("end_time", "invalid", "end_time must be after start_time")
	}
	if !validMarketTypes[req.MarketType] {
		v.AddError("market_type", "invalid", "market_type must be one of: crypto, forex")
	}
	if req.EntryFee < 0 {
		v.AddError("entry_fee", "invalid", "entry_fee must be >= 0")
	}
	if req.QtyTotal <= 0 {
		v.AddError("qty_total", "invalid", "qty_total must be greater than 0")
	}
	if len(req.Symbols) == 0 {
		v.AddError("symbols", "required", "at least one symbol is required")
	}
	if req.CommissionRate < 0 || req.CommissionRate > 50 {
		v.AddError("commission_rate", "invalid", "commission_rate must be between 0 and 50")
	}
	if req.IsSponsored && req.PrizePool == nil {
		v.AddError("prize_pool", "required", "prize_pool is required for sponsored tournaments")
	}
	if req.PrizePool != nil && *req.PrizePool < 0 {
		v.AddError("prize_pool", "invalid", "prize_pool must be >= 0")
	}

	validatePrizeDistributions(v, req.PrizeDistributions)

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Sanitize name
	req.Name = validation.SanitizeName(req.Name)

	// Calculate duration
	durationMinutes := int(req.EndTime.Sub(req.StartTime).Minutes())

	// Map market_type to asset_class for the contests table
	assetClass := req.MarketType // crypto -> crypto, forex -> forex

	// Build rules_json with special tournament metadata
	rulesJSON, _ := json.Marshal(map[string]interface{}{
		"is_sponsored":     req.IsSponsored,
		"prize_pool":       req.PrizePool,
		"banner_image_url": req.BannerImageURL,
		"special_event":    true,
	})

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

	// Insert contest as a special tournament (no template_id, no schedule_id)
	var contest ContestResponse
	err = tx.QueryRowContext(ctx,
		`INSERT INTO contests (
			name, description, starts_at, ends_at, status,
			entry_fee_cents, platform_fee_bps, qty_total,
			duration_type, asset_class, duration_minutes,
			min_participants, registration_deadline,
			auto_start, commission_rate, is_free, rules_json
		) VALUES ($1, $2, $3, $4, 'draft',
			$5, 0, $6,
			'special'::contest_duration_type, $7::asset_class, $8,
			2, $3,
			FALSE, $9, $10, $11::jsonb)
		RETURNING id, name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
			duration_type, asset_class, COALESCE(duration_minutes, 0), min_participants, max_participants,
			registration_deadline, auto_start, commission_rate, is_free, auto_generated,
			template_id, created_at`,
		req.Name, req.Description, req.StartTime, req.EndTime,
		int(req.EntryFee), req.QtyTotal,
		assetClass, durationMinutes,
		req.CommissionRate, req.EntryFee == 0, string(rulesJSON),
	).Scan(
		&contest.ID, &contest.Name, &contest.Description, &contest.StartsAt, &contest.EndsAt,
		&contest.Status, &contest.EntryFeeCents, &contest.PlatformFeeBps, &contest.QtyTotal,
		&contest.DurationType, &contest.AssetClass, &contest.DurationMinutes, &contest.MinParticipants,
		&contest.MaxParticipants, &contest.RegistrationDeadline, &contest.AutoStart, &contest.CommissionRate,
		&contest.IsFree, &contest.AutoGenerated,
		&contest.TemplateID, &contest.CreatedAt,
	)
	if err != nil {
		a.log().Error("Failed to insert special tournament", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add symbols
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

	// Store prize distribution rules in rules_json (already done above)
	// Also store them as metadata for reference
	if len(req.PrizeDistributions) > 0 {
		prizeJSON, _ := json.Marshal(req.PrizeDistributions)
		_, err = tx.ExecContext(ctx,
			`UPDATE contests SET rules_json = rules_json || jsonb_build_object('prize_distributions', $1::jsonb) WHERE id = $2`,
			string(prizeJSON), contest.ID,
		)
		if err != nil {
			a.log().Error("Failed to store prize distributions in rules_json", zap.Error(err))
			// Non-fatal, continue
		}
	}

	// Auto-register system bot for free contests so min_participants is met
	if req.EntryFee == 0 {
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
		actorUserID, "tournament.special_created", "contest", contest.ID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit special tournament creation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.log().Info("Special tournament created",
		zap.String("contest_id", contest.ID),
		zap.String("name", req.Name),
		zap.Bool("is_sponsored", req.IsSponsored),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusCreated, contest)
}

// =============================================================================
// Tournament Stats Handlers
// =============================================================================

// handleGetTournamentOverviewStats returns aggregated tournament statistics.
// GET /api/admin/stats/tournaments
func (a *App) handleGetTournamentOverviewStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startOfWeek := startOfDay.AddDate(0, 0, -int(now.Weekday()))
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	stats := TournamentOverviewStats{}

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 5)

	recordError := func(err error, context string) {
		if err != nil {
			mu.Lock()
			mu.Unlock()
			a.log().Error("Stats query failed", zap.String("context", context), zap.Error(err))
		}
	}

	// Query 1: Active tournaments count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "stats-active-tournaments", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var count int64
		err := a.pool.Replica().QueryRowContext(queryCtx, `
			SELECT COUNT(*) FROM contests WHERE status = 'running'
		`).Scan(&count)
		recordError(err, "active tournaments")
		mu.Lock()
		stats.ActiveTournaments = count
		mu.Unlock()
	})

	// Query 2: Total participants across active tournaments
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "stats-total-participants", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var count int64
		err := a.pool.Replica().QueryRowContext(queryCtx, `
			SELECT COUNT(*)
			FROM contest_participants cp
			JOIN contests c ON cp.contest_id = c.id
			WHERE c.status = 'running'
		`).Scan(&count)
		recordError(err, "total participants")
		mu.Lock()
		stats.TotalParticipants = count
		mu.Unlock()
	})

	// Query 3: Total prize pool across active tournaments
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "stats-total-prize-pool", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var prizePool int64
		err := a.pool.Replica().QueryRowContext(queryCtx, `
			SELECT COALESCE(SUM(
				(SELECT COUNT(*) FROM contest_participants WHERE contest_id = c.id)
				* c.entry_fee_cents
				* (100 - c.commission_rate) / 100
			), 0)
			FROM contests c
			WHERE c.status = 'running'
		`).Scan(&prizePool)
		recordError(err, "total prize pool")
		mu.Lock()
		stats.TotalPrizePool = prizePool
		mu.Unlock()
	})

	// Query 4: Commission earned today/week/month
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "stats-commission-earned", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var today, week, month int64
		err := a.pool.Replica().QueryRowContext(queryCtx, `
			SELECT
				COALESCE(SUM(commission_amount) FILTER (WHERE ends_at >= $1), 0),
				COALESCE(SUM(commission_amount) FILTER (WHERE ends_at >= $2), 0),
				COALESCE(SUM(commission_amount) FILTER (WHERE ends_at >= $3), 0)
			FROM contests
			WHERE status IN ('completed', 'settling')
		`, startOfDay, startOfWeek, startOfMonth).Scan(&today, &week, &month)
		recordError(err, "commission")
		mu.Lock()
		stats.CommissionToday = today
		stats.CommissionThisWeek = week
		stats.CommissionThisMonth = month
		mu.Unlock()
	})

	// Query 5: Average participants per tournament type
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "stats-avg-participants-by-type", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		rows, err := a.pool.Replica().QueryContext(queryCtx, `
			SELECT
				c.duration_type,
				AVG(participant_count)::float8 as avg_participants
			FROM (
				SELECT c.id, c.duration_type,
					(SELECT COUNT(*) FROM contest_participants WHERE contest_id = c.id) as participant_count
				FROM contests c
				WHERE c.status IN ('running', 'completed', 'settling')
					AND c.created_at >= $1
			) c
			WHERE c.duration_type IS NOT NULL
			GROUP BY c.duration_type
			ORDER BY c.duration_type
		`, startOfMonth)
		if err != nil {
			recordError(err, "avg participants by type")
			return
		}
		defer rows.Close()

		var avgs []TypeAvg
		for rows.Next() {
			var ta TypeAvg
			if scanErr := rows.Scan(&ta.DurationType, &ta.AvgParticipants); scanErr != nil {
				continue
			}
			avgs = append(avgs, ta)
		}
		mu.Lock()
		stats.AvgParticipantsByType = avgs
		mu.Unlock()
	})

	wg.Wait()

	if stats.AvgParticipantsByType == nil {
		stats.AvgParticipantsByType = []TypeAvg{}
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGetTournamentDetailStats returns detailed statistics for a single tournament.
// GET /api/admin/stats/tournaments/{id}
func (a *App) handleGetTournamentDetailStats(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TournamentIDRequired})
		return
	}

	ctx := r.Context()

	// Fetch contest details
	var stats TournamentDetailStats
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT c.id, c.name, c.status, c.starts_at, c.ends_at,
				c.entry_fee_cents, c.commission_rate, c.commission_amount,
				c.duration_type, c.asset_class
			FROM contests c
			WHERE c.id = $1
		`, contestID).Scan(
			&stats.ContestID, &stats.Name, &stats.Status,
			&stats.StartsAt, &stats.EndsAt,
			&stats.EntryFeeCents, &stats.CommissionRate, &stats.CommissionAmount,
			&stats.DurationType, &stats.AssetClass,
		)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TournamentNotFound})
			return
		}
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to get tournament stats", zap.String("contest_id", contestID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Fetch participant count
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1
		`, contestID).Scan(&stats.ParticipantCount)
	})
	if err != nil {
		a.log().Error("Failed to get participant count", zap.Error(err))
	}

	// Calculate derived metrics
	stats.EntryFeeRevenue = stats.ParticipantCount * int64(stats.EntryFeeCents)
	stats.NetPrizePool = stats.EntryFeeRevenue - stats.CommissionAmount

	// If commission_amount is 0 but we have participants, estimate from rate
	if stats.CommissionAmount == 0 && stats.ParticipantCount > 0 {
		estimatedCommission := float64(stats.EntryFeeRevenue) * stats.CommissionRate / 100
		stats.CommissionAmount = int64(estimatedCommission)
		stats.NetPrizePool = stats.EntryFeeRevenue - stats.CommissionAmount
	}

	// Fetch participant join history (grouped by hour)
	historyResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT
					date_trunc('hour', joined_at) as bucket,
					COUNT(*) as count
				FROM contest_participants
				WHERE contest_id = $1
				GROUP BY bucket
				ORDER BY bucket ASC
			`, contestID)
		},
	)
	if err != nil {
		a.log().Error("Failed to get participant history", zap.Error(err))
		stats.ParticipantHistory = []ParticipantSnapshot{}
	} else {
		rows := historyResult.(*sql.Rows)
		defer rows.Close()

		stats.ParticipantHistory = []ParticipantSnapshot{}
		var runningTotal int64
		for rows.Next() {
			var snapshot ParticipantSnapshot
			var bucketCount int64
			if scanErr := rows.Scan(&snapshot.Timestamp, &bucketCount); scanErr != nil {
				continue
			}
			runningTotal += bucketCount
			snapshot.Count = runningTotal
			stats.ParticipantHistory = append(stats.ParticipantHistory, snapshot)
		}
	}

	writeJSON(w, http.StatusOK, stats)
}
