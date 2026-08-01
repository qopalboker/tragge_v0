package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func (a *App) handleListContests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse filter parameters
	durationType := r.URL.Query().Get("duration_type")
	assetClass := r.URL.Query().Get("asset_class")
	isFree := r.URL.Query().Get("is_free")
	minEntryStr := r.URL.Query().Get("min_entry")
	maxEntryStr := r.URL.Query().Get("max_entry")

	hasFilters := durationType != "" || assetClass != "" || isFree != "" || minEntryStr != "" || maxEntryStr != ""

	// Check cache first (only if no filters applied)
	if !hasFilters {
		a.contestsCache.mu.RLock()
		if time.Now().Before(a.contestsCache.expiresAt) && a.contestsCache.data != nil {
			cached := a.contestsCache.data
			a.contestsCache.mu.RUnlock()
			writeJSON(w, http.StatusOK, cached)
			return
		}
		a.contestsCache.mu.RUnlock()
	}

	queryStartTime := time.Now()

	// Build dynamic query with filters.
	// NOTE: All column names below are static string literals — only user-supplied values
	// are parameterized via $N placeholders. This is safe from SQL injection.
	query := `
		SELECT c.id, c.name, COALESCE(c.description, ''), c.starts_at, c.ends_at, c.status,
		       c.entry_fee_cents, c.qty_total, c.duration_type, c.asset_class, COALESCE(c.duration_minutes, 0),
		       c.min_participants, c.max_participants, c.registration_deadline, c.commission_rate,
		       c.is_free, c.rules_json,
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count
		FROM contests c
		WHERE c.status IN ('registration_open', 'scheduled', 'running')`

	var args []interface{}
	argIdx := 1

	// Duration type filter
	if durationType != "" {
		// Validate duration type
		validTypes := map[string]bool{
			"rush_30min": true,
			"hourly":     true,
			"four_hour":  true,
			"daily":      true,
			"weekly":     true,
		}
		if validTypes[durationType] {
			query += " AND c.duration_type = $" + strconv.Itoa(argIdx)
			args = append(args, durationType)
			argIdx++
		}
	}

	// Asset class filter
	if assetClass != "" {
		// Validate asset class
		validClasses := map[string]bool{
			"forex":  true,
			"crypto": true,
			"stocks": true,
			"mixed":  true,
		}
		if validClasses[assetClass] {
			query += " AND c.asset_class = $" + strconv.Itoa(argIdx)
			args = append(args, assetClass)
			argIdx++
		}
	}

	// Free only filter
	if isFree == "true" {
		query += " AND c.is_free = TRUE"
	} else if isFree == "false" {
		query += " AND c.is_free = FALSE"
	}

	// Min entry fee filter
	if minEntryStr != "" {
		if minEntry, err := strconv.Atoi(minEntryStr); err == nil && minEntry >= 0 {
			query += " AND c.entry_fee_cents >= $" + strconv.Itoa(argIdx)
			args = append(args, minEntry)
			argIdx++
		}
	}

	// Max entry fee filter
	if maxEntryStr != "" {
		if maxEntry, err := strconv.Atoi(maxEntryStr); err == nil && maxEntry >= 0 {
			query += " AND c.entry_fee_cents <= $" + strconv.Itoa(argIdx)
			args = append(args, maxEntry)
			argIdx++
		}
	}

	query += " ORDER BY c.starts_at ASC"

	// Query active/upcoming contests (read-only, use replica)
	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to query contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	contestMap := make(map[string]*ContestResponse)
	var contestIDs []string

	for rows.Next() {
		var c ContestResponse
		var rules sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.StartsAt, &c.EndsAt,
			&c.Status, &c.EntryFee, &c.QtyTotal, &c.DurationType, &c.AssetClass,
			&c.DurationMinutes, &c.MinParticipants, &c.MaxParticipants, &c.RegistrationDeadline,
			&c.CommissionRate, &c.IsFree, &rules, &c.ParticipantCount); err != nil {
			a.log().Error("Failed to scan contest", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		if rules.Valid && rules.String != "" {
			c.Rules = json.RawMessage(rules.String)
		}
		c.Symbols = []ContestSymbol{}
		contestMap[c.ID] = &c
		contestIDs = append(contestIDs, c.ID)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Query symbols for all contests (using IN clause with proper placeholders)
	if len(contestIDs) > 0 {
		// Build placeholders and args for IN clause
		args := make([]interface{}, len(contestIDs))
		query := `SELECT contest_id, symbol, enabled FROM contest_symbols WHERE contest_id IN (`
		for i, id := range contestIDs {
			args[i] = id
			if i > 0 {
				query += ","
			}
			query += "$" + strconv.Itoa(i+1)
		}
		query += `) ORDER BY symbol`

		symbolRows, err := a.pool.Replica().QueryContext(ctx, query, args...)
		if err != nil {
			a.log().Error("Failed to query contest symbols", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		defer symbolRows.Close()

		for symbolRows.Next() {
			var contestID, symbol string
			var enabled bool
			if err := symbolRows.Scan(&contestID, &symbol, &enabled); err != nil {
				a.log().Error("Failed to scan symbol", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
				return
			}
			if c, ok := contestMap[contestID]; ok {
				c.Symbols = append(c.Symbols, ContestSymbol{Symbol: symbol, Enabled: enabled})
			}
		}
		if err := symbolRows.Err(); err != nil {
			a.log().Error("Failed to iterate symbols", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// Build result slice maintaining order
	contests := make([]ContestResponse, 0, len(contestIDs))
	for _, id := range contestIDs {
		contests = append(contests, *contestMap[id])
	}

	// Update cache (only for unfiltered requests, and only if our data is fresher)
	if !hasFilters {
		a.contestsCache.mu.Lock()
		if queryStartTime.After(a.contestsCache.cachedAt) {
			a.contestsCache.data = contests
			a.contestsCache.cachedAt = queryStartTime
			a.contestsCache.expiresAt = time.Now().Add(contestsCacheTTL)
		}
		a.contestsCache.mu.Unlock()
	}

	writeJSON(w, http.StatusOK, contests)
}

// handleGetContestDetails returns detailed information about a specific contest.
// This endpoint is public but provides extra user-specific information when authenticated.
func (a *App) handleGetContestDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Query contest details
	var resp ContestDetailsResponse
	var startsAt, endsAt time.Time
	var commissionRate float64
	var platformFeeBps int
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT c.id, c.name, COALESCE(c.description, ''), c.status, c.asset_class, c.duration_type,
		       c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free, c.qty_total,
		       c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0),
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id)
		FROM contests c
		WHERE c.id = $1
	`, contestID).Scan(
		&resp.ID, &resp.Name, &resp.Description, &resp.Status, &resp.MarketType, &resp.DurationType,
		&startsAt, &endsAt, &resp.EntryFeeCents, &resp.IsFree, &resp.AvailableQty,
		&resp.MaxParticipants, &commissionRate, &platformFeeBps, &resp.CurrentParticipants,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest details", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Expose asset_class alongside legacy market_type
	resp.AssetClass = resp.MarketType

	// Format timestamps as ISO8601
	resp.StartTime = startsAt.Format(time.RFC3339)
	resp.EndTime = endsAt.Format(time.RFC3339)

	// P2-P3-2: Fee transparency - expose commission rate and gross/net prize pool
	resp.CommissionRate = commissionRate
	if resp.CurrentParticipants > 0 && resp.EntryFeeCents > 0 {
		totalEntryFees := resp.EntryFeeCents * resp.CurrentParticipants
		resp.GrossPrizeCents = totalEntryFees
		feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
		resp.PrizePoolCents = int((int64(totalEntryFees) * int64(10000-feeBps)) / 10000)
	}

	// Query symbols for this contest
	symbolRows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT symbol FROM contest_symbols WHERE contest_id = $1 AND enabled = true ORDER BY symbol`,
		contestID,
	)
	if err != nil {
		a.log().Error("Failed to query contest symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer symbolRows.Close()

	resp.Symbols = []string{}
	for symbolRows.Next() {
		var symbol string
		if err := symbolRows.Scan(&symbol); err != nil {
			a.log().Error("Failed to scan symbol", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		resp.Symbols = append(resp.Symbols, symbol)
	}
	if err := symbolRows.Err(); err != nil {
		a.log().Error("Failed to iterate symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Check if user is authenticated and has joined this contest
	userID := auth.GetUserID(ctx)
	if userID != "" {
		var joinedAt sql.NullTime
		err = a.pool.Replica().QueryRowContext(ctx,
			`SELECT joined_at FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
			contestID, userID,
		).Scan(&joinedAt)
		if err == nil && joinedAt.Valid {
			resp.UserJoined = true
		}
	}

	// P2-P3-3: Include server time for countdown synchronization
	resp.ServerTime = time.Now().UTC().Format(time.RFC3339Nano)

	writeJSON(w, http.StatusOK, resp)
}

// handleGetContestParticipants returns the list of participants in a contest.
func (a *App) handleGetContestParticipants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Verify contest exists (read-only, use replica)
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
		return
	}

	// Query participants with their usernames and contest data
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT cp.user_id, u.username, cp.joined_at,
		       cp.qty_total, cp.qty_available, cp.total_score,
		       cp.final_rank, cp.final_prize_cents
		FROM contest_participants cp
		JOIN users u ON cp.user_id = u.id
		WHERE cp.contest_id = $1
		ORDER BY cp.joined_at ASC
	`, contestID)
	if err != nil {
		a.log().Error("Failed to query contest participants", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	participants := make([]ParticipantEntry, 0)
	for rows.Next() {
		var p ParticipantEntry
		if err := rows.Scan(&p.UserID, &p.Username, &p.JoinedAt,
			&p.QtyTotal, &p.QtyAvailable, &p.TotalScore,
			&p.FinalRank, &p.FinalPrizeCents); err != nil {
			a.log().Error("Failed to scan participant row", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		participants = append(participants, p)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating participant rows", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, ContestParticipantsResponse{
		Participants: participants,
		Total:        len(participants),
	})
}

// handleGetContestLeaderboard returns the leaderboard for a contest.
func (a *App) handleGetContestLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Get contest info including prize pool calculation (read-only, use replica)
	var entryFeeCents int
	var platformFeeBps int
	var commissionRate float64
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT entry_fee_cents, COALESCE(platform_fee_bps, 0), COALESCE(commission_rate, 0), TRUE
		FROM contests WHERE id = $1
	`, contestID).Scan(&entryFeeCents, &platformFeeBps, &commissionRate, &exists)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Query participants with their scores, ranks, prizes, and trade counts
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT
			cp.user_id,
			COALESCE(u.username, ''),
			cp.total_score,
			cp.qty_total,
			COALESCE(cp.final_rank, 0),
			COALESCE(cp.final_prize_cents, 0),
			COALESCE((SELECT COUNT(*) FROM fills f JOIN orders o ON f.order_id = o.id WHERE o.contest_id = cp.contest_id AND o.user_id = cp.user_id), 0)
		FROM contest_participants cp
		JOIN users u ON cp.user_id = u.id
		WHERE cp.contest_id = $1
		ORDER BY cp.total_score DESC, cp.joined_at ASC
	`, contestID)
	if err != nil {
		a.log().Error("Failed to query contest leaderboard", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	leaderboard := make([]ContestLeaderboardEntry, 0)
	position := 0
	for rows.Next() {
		position++
		var userID string
		var username string
		var totalScore float64
		var qtyTotal int64
		var finalRank int
		var finalPrizeCents int
		var tradeCount int

		if err := rows.Scan(&userID, &username, &totalScore, &qtyTotal, &finalRank, &finalPrizeCents, &tradeCount); err != nil {
			a.log().Error("Failed to scan leaderboard row", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		// Calculate PnL percent: (total_score / qty_total) * 100
		var pnlPercent float64
		if qtyTotal > 0 {
			pnlPercent = (totalScore / float64(qtyTotal)) * 100
		}

		// Use final_rank if available, otherwise use calculated position
		displayPosition := position
		if finalRank > 0 {
			displayPosition = finalRank
		}

		leaderboard = append(leaderboard, ContestLeaderboardEntry{
			Position:    displayPosition,
			UserID:      userID,
			Username:    username,
			PnlPercent:  pnlPercent,
			RewardCents: finalPrizeCents,
			TradeCount:  tradeCount,
		})
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating leaderboard rows", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Calculate prize pool: (entry_fee * participants) - platform fee
	totalParticipants := len(leaderboard)
	feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
	prizePoolGross := int64(totalParticipants) * int64(entryFeeCents)
	prizePoolCents := int((prizePoolGross * int64(10000-feeBps)) / 10000)

	writeJSON(w, http.StatusOK, ContestLeaderboardResponse{
		Leaderboard:       leaderboard,
		TotalParticipants: totalParticipants,
		PrizePoolCents:    prizePoolCents,
	})
}

// handleJoinContest allows a user to join a contest (idempotent).
func (a *App) handleJoinContest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Check if contest exists and is open for registration (use Primary before write)
	var status string
	var qtyTotal int64
	var entryFeeCents int
	var contestName string
	var platformFeeBps int
	var commissionRate float64
	var maxParticipants sql.NullInt64
	var currentParticipants int
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT status, qty_total, entry_fee_cents, name, COALESCE(platform_fee_bps, 0), COALESCE(commission_rate, 0), max_participants, current_participants FROM contests WHERE id = $1`,
		contestID,
	).Scan(&status, &qtyTotal, &entryFeeCents, &contestName, &platformFeeBps, &commissionRate, &maxParticipants, &currentParticipants)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if status != "registration_open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestNotOpen})
		return
	}

	// Check if contest has reached max participants
	if maxParticipants.Valid && int64(currentParticipants) >= maxParticipants.Int64 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg.ContestFull})
		return
	}

	// Block system accounts from joining paid contests
	if entryFeeCents > 0 {
		var isSystemAccount bool
		err = a.pool.Primary().QueryRowContext(ctx,
			`SELECT COALESCE(is_system_account, FALSE) FROM users WHERE id = $1`,
			userID,
		).Scan(&isSystemAccount)
		if err != nil {
			a.log().Error("Failed to check system account status", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		if isSystemAccount {
			a.log().Warn("System account attempted to join paid contest",
				zap.String("user_id", userID),
				zap.String("contest_id", contestID))
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.SystemCannotJoinPaid})
			return
		}
	}

	// Check if user is already a participant
	var existingJoinedAt time.Time
	var existingQtyAvailable int64
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT joined_at, qty_available FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	).Scan(&existingJoinedAt, &existingQtyAvailable)
	if err == nil {
		// User already joined - return existing data (idempotent)
		writeJSON(w, http.StatusOK, JoinContestResponse{
			ContestID:     contestID,
			UserID:        userID,
			JoinedAt:      existingJoinedAt,
			QtyTotal:      qtyTotal,
			QtyAvailable:  existingQtyAvailable,
			AlreadyJoined: true,
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		a.log().Error("Failed to check existing participation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Begin transaction
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()

	// Lock the contest row to prevent race conditions on prize pool updates.
	// Re-read status inside the transaction with FOR UPDATE to guarantee consistency.
	var txStatus string
	var txCurrentParticipants int
	var txMaxParticipants sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT status, current_participants, max_participants
		FROM contests WHERE id = $1 FOR UPDATE
	`, contestID).Scan(&txStatus, &txCurrentParticipants, &txMaxParticipants)
	if err != nil {
		a.log().Error("Failed to lock contest row", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if txStatus != "registration_open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestNotOpen})
		return
	}
	if txMaxParticipants.Valid && int64(txCurrentParticipants) >= txMaxParticipants.Int64 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg.ContestFull})
		return
	}

	// If there's an entry fee, check balance and deduct from wallet
	if entryFeeCents > 0 {
		_, err = a.wallet.DeductContestEntryFeeWithName(ctx, tx, userID, contestID, contestName, int64(entryFeeCents))
		if err != nil {
			if insufficientErr, ok := err.(*wallet.InsufficientBalanceError); ok {
				writeJSON(w, http.StatusPaymentRequired, map[string]interface{}{
					"error":     msg.InsufficientBalance,
					"required":  insufficientErr.Required,
					"available": insufficientErr.Available,
				})
				return
			}
			if _, ok := err.(*wallet.WalletFrozenError); ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.WalletFrozen})
				return
			}
			if _, ok := err.(*wallet.WalletNotFoundError); ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.WalletNotFound})
				return
			}
			a.log().Error("Failed to deduct entry fee", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// Insert participant
	var joinedAt time.Time
	var qtyAvailable int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available)
		VALUES ($1, $2, $3, $3)
		RETURNING joined_at, qty_available
	`, contestID, userID, qtyTotal).Scan(&joinedAt, &qtyAvailable)
	if err != nil {
		if strings.Contains(err.Error(), "chk_current_participants_lte_max") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": msg.ContestFull})
			return
		}
		a.log().Error("Failed to insert participant", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Calculate commission and prize contribution, then update the contest record atomically
	var commissionCents int64
	var prizeContributionCents int64
	if entryFeeCents > 0 {
		effectiveCommissionRate := commissionRate
		if effectiveCommissionRate <= 0 {
			effectiveCommissionRate = 20.00 // default 20%
		}
		commissionCents = int64(math.Round(float64(entryFeeCents) * effectiveCommissionRate / 100.0))
		prizeContributionCents = int64(entryFeeCents) - commissionCents

		_, err = tx.ExecContext(ctx, `
			UPDATE contests
			SET prize_pool_net_cents = COALESCE(prize_pool_net_cents, 0) + $1,
			    commission_amount = commission_amount + $2
			WHERE id = $3
		`, prizeContributionCents, commissionCents, contestID)
		if err != nil {
			a.log().Error("Failed to update contest prize pool and commission",
				zap.Error(err),
				zap.String("contest_id", contestID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// Handle affiliate commission for PAID contests
	if entryFeeCents > 0 {
		a.processAffiliateCommission(ctx, tx, userID, contestID, int64(entryFeeCents))
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Create in-app notification for contest join (non-blocking, respects user preferences)
	infra.SafeGo(a.log(), "contest-joined-notification", func() {
		notifCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		enabled, _ := prefs.IsEnabled(notifCtx, a.pool.Replica(), userID, inapp.NotifTypeContestJoined, "in_app")
		if !enabled {
			return
		}
		if err := inapp.CreateContestJoinedNotification(notifCtx, a.pool.Primary(), userID, contestID, contestName); err != nil {
			a.log().Warn("Failed to create contest joined notification",
				zap.Error(err),
				zap.String("user_id", userID),
				zap.String("contest_id", contestID))
		}
	})

	// P2-P3-2: Calculate estimated net prize pool for fee transparency
	effectiveFeeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)

	// Broadcast real-time prize update to all viewers (non-blocking)
	infra.SafeGo(a.log(), "contest-update-joined", func() {
		a.publishContestUpdate(contestID, "participant_joined", entryFeeCents, effectiveFeeBps)
	})

	// Publish tournament feed update for browsing clients (Task 8.3, non-blocking)
	infra.SafeGo(a.log(), "tournament-feed-update-joined", func() {
		feedCtx, feedCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer feedCancel()
		var feedParticipants int
		var feedNetPool int64
		if err := a.pool.Primary().QueryRowContext(feedCtx,
			`SELECT current_participants, COALESCE(prize_pool_net_cents, 0) FROM contests WHERE id = $1`, contestID,
		).Scan(&feedParticipants, &feedNetPool); err == nil {
			a.publishTournamentFeedUpdate(contestID, "participant_joined", feedParticipants, feedNetPool)
		}
	})

	// Read the updated prize pool from the database for accurate response
	var netPool int64
	var participantCount int
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT COALESCE(prize_pool_net_cents, 0), current_participants FROM contests WHERE id = $1`, contestID,
	).Scan(&netPool, &participantCount)
	if err != nil {
		// Fallback to calculation if read fails
		_ = a.pool.Primary().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`, contestID,
		).Scan(&participantCount)
		grossPool := int64(participantCount) * int64(entryFeeCents)
		netPool = (grossPool * int64(10000-effectiveFeeBps)) / 10000
	}

	writeJSON(w, http.StatusOK, JoinContestResponse{
		ContestID:      contestID,
		UserID:         userID,
		JoinedAt:       joinedAt,
		QtyTotal:       qtyTotal,
		QtyAvailable:   qtyAvailable,
		EntryFeeCents:  entryFeeCents,
		PlatformFeeBps: effectiveFeeBps,
		NetPrizeCents:  netPool,
	})
}

// handleLeaveContest allows a user to leave a contest before it starts running.
// It refunds the entry fee, reverses prize pool contribution, and cancels any
// pending affiliate commission created when the user joined.
func (a *App) handleLeaveContest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Read contest details (use Primary since we're about to write)
	var status string
	var entryFeeCents int
	var contestName string
	var commissionRate float64
	var platformFeeBps int
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT status, entry_fee_cents, name, COALESCE(commission_rate, 0), COALESCE(platform_fee_bps, 0)
		 FROM contests WHERE id = $1`, contestID,
	).Scan(&status, &entryFeeCents, &contestName, &commissionRate, &platformFeeBps)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest for leave", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Only allow leaving during registration_open phase
	if status != "registration_open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.CannotLeaveRunning,
		})
		return
	}

	// Verify the user is actually a participant
	var joinedAt time.Time
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT joined_at FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	).Scan(&joinedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.NotParticipant})
			return
		}
		a.log().Error("Failed to check participation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Check for open positions — block leave if user has active trades
	var openPositionCount int
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM positions WHERE contest_id = $1 AND user_id = $2 AND qty_open > 0`,
		contestID, userID,
	).Scan(&openPositionCount)
	if err != nil {
		a.log().Error("Failed to check open positions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if openPositionCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": msg.CannotLeaveOpenPositions,
		})
		return
	}

	// Begin transaction
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.log().Error("Failed to begin leave transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()

	// Lock the contest row to prevent race conditions
	var txStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1 FOR UPDATE`, contestID,
	).Scan(&txStatus)
	if err != nil {
		a.log().Error("Failed to lock contest row for leave", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if txStatus != "registration_open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.CannotLeaveRunning,
		})
		return
	}

	// Remove participant
	result, err := tx.ExecContext(ctx,
		`DELETE FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	)
	if err != nil {
		a.log().Error("Failed to delete participant", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.NotParticipant})
		return
	}

	// Refund entry fee if this was a paid contest
	if entryFeeCents > 0 {
		_, err = a.wallet.RefundContestEntryFeeWithReason(ctx, tx, userID, contestID, contestName, int64(entryFeeCents), wallet.ReasonCodeContestRefundLeave)
		if err != nil {
			a.log().Error("Failed to refund entry fee on leave",
				zap.Error(err),
				zap.String("user_id", userID),
				zap.String("contest_id", contestID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		// Reverse prize pool contribution and commission
		effectiveCommissionRate := commissionRate
		if effectiveCommissionRate <= 0 {
			effectiveCommissionRate = 20.00 // default 20%
		}
		commissionCents := int64(math.Round(float64(entryFeeCents) * effectiveCommissionRate / 100.0))
		prizeContributionCents := int64(entryFeeCents) - commissionCents

		_, err = tx.ExecContext(ctx, `
			UPDATE contests
			SET prize_pool_net_cents = GREATEST(COALESCE(prize_pool_net_cents, 0) - $1, 0),
			    commission_amount = GREATEST(commission_amount - $2, 0)
			WHERE id = $3
		`, prizeContributionCents, commissionCents, contestID)
		if err != nil {
			a.log().Error("Failed to reverse contest prize pool on leave",
				zap.Error(err),
				zap.String("contest_id", contestID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		// Reverse affiliate commission if applicable
		a.reverseAffiliateCommission(ctx, tx, userID, contestID)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit leave transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Broadcast real-time update (non-blocking)
	effectiveFeeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
	infra.SafeGo(a.log(), "contest-update-left", func() {
		a.publishContestUpdate(contestID, "participant_left", entryFeeCents, effectiveFeeBps)
	})

	infra.SafeGo(a.log(), "tournament-feed-update-left", func() {
		feedCtx, feedCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer feedCancel()
		var feedParticipants int
		var feedNetPool int64
		if err := a.pool.Primary().QueryRowContext(feedCtx,
			`SELECT current_participants, COALESCE(prize_pool_net_cents, 0) FROM contests WHERE id = $1`, contestID,
		).Scan(&feedParticipants, &feedNetPool); err == nil {
			a.publishTournamentFeedUpdate(contestID, "participant_left", feedParticipants, feedNetPool)
		}
	})

	a.log().Info("User left contest",
		zap.String("user_id", userID),
		zap.String("contest_id", contestID),
		zap.Int("entry_fee_cents", entryFeeCents))

	writeJSON(w, http.StatusOK, map[string]string{
		"message":    msg.LeftContestSuccess,
		"contest_id": contestID,
	})
}

// reverseAffiliateCommission cancels a pending affiliate commission or debits the
// referrer's wallet if the commission was already credited. Called when a user leaves
// a paid contest. Errors are logged but do not fail the leave operation.
func (a *App) reverseAffiliateCommission(ctx context.Context, tx *db.Transaction, userID, contestID string) {
	// Find the affiliate commission for this contest entry
	var commissionID string
	var referrerID string
	var commissionCents int64
	var commissionStatus string
	err := tx.QueryRowContext(ctx, `
		SELECT id, referrer_id, commission_cents, status
		FROM affiliate_commissions
		WHERE referred_id = $1 AND source_type = 'contest_entry' AND source_id = $2::uuid
	`, userID, contestID).Scan(&commissionID, &referrerID, &commissionCents, &commissionStatus)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			a.log().Warn("Failed to look up affiliate commission for reversal",
				zap.String("user_id", userID),
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
		return // No commission to reverse
	}

	if commissionStatus == "cancelled" {
		return // Already cancelled
	}

	// Cancel the commission record
	_, err = tx.ExecContext(ctx, `
		UPDATE affiliate_commissions
		SET status = 'cancelled'
		WHERE id = $1
	`, commissionID)
	if err != nil {
		a.log().Warn("Failed to cancel affiliate commission",
			zap.String("commission_id", commissionID),
			zap.Error(err))
		return
	}

	// If the commission was already credited to the referrer's wallet, debit it back
	if commissionStatus == "credited" && commissionCents > 0 {
		refType := wallet.LedgerRefTypeCommission
		desc := fmt.Sprintf("Reversal: Affiliate commission for contest %s (user left)", contestID)
		_, err = a.wallet.Debit(ctx, tx, referrerID, commissionCents,
			wallet.LedgerTypeAffiliateCommission, &refType, &commissionID, &desc)
		if err != nil {
			a.log().Warn("Failed to debit referrer wallet for commission reversal",
				zap.String("referrer_id", referrerID),
				zap.String("commission_id", commissionID),
				zap.Int64("commission_cents", commissionCents),
				zap.Error(err))
			// Don't fail — the commission record is already cancelled
		} else {
			a.log().Info("Reversed credited affiliate commission",
				zap.String("referrer_id", referrerID),
				zap.String("referred_id", userID),
				zap.String("contest_id", contestID),
				zap.Int64("commission_cents", commissionCents))
		}
	} else {
		a.log().Info("Cancelled pending affiliate commission",
			zap.String("referrer_id", referrerID),
			zap.String("referred_id", userID),
			zap.String("contest_id", contestID),
			zap.Int64("commission_cents", commissionCents))
	}
}

// handleLeaderboard returns the top 100 entries from the Redis ZSET lb:{contest_id}.
func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := r.URL.Query().Get("contest_id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Verify contest exists (read-only, use replica)
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
		return
	}

	var entries []LeaderboardEntry

	// Try Redis first
	if a.redis != nil {
		redisKey := "lb:" + contestID
		// ZREVRANGE with scores (highest score first)
		results, err := a.redis.ZRevRangeWithScores(ctx, redisKey, 0, 99).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			a.log().Warn("Redis leaderboard fetch failed", zap.Error(err))
			// Fall through to database fallback
		} else if len(results) > 0 {
			entries = make([]LeaderboardEntry, 0, len(results))
			for i, z := range results {
				entries = append(entries, LeaderboardEntry{
					Rank:       i + 1,
					UserID:     z.Member.(string),
					TotalScore: z.Score,
				})
			}
			writeJSON(w, http.StatusOK, LeaderboardResponse{
				ContestID: contestID,
				Entries:   entries,
			})
			return
		}
	}

	// Fallback to database (read-only, use replica)
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT user_id, total_score
		FROM contest_participants
		WHERE contest_id = $1
		ORDER BY total_score DESC
		LIMIT 100
	`, contestID)
	if err != nil {
		a.log().Error("Failed to query leaderboard", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	entries = []LeaderboardEntry{}
	rank := 1
	for rows.Next() {
		var userID string
		var score float64
		if err := rows.Scan(&userID, &score); err != nil {
			a.log().Error("Failed to scan leaderboard entry", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		entries = append(entries, LeaderboardEntry{
			Rank:       rank,
			UserID:     userID,
			TotalScore: score,
		})
		rank++
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate leaderboard", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, LeaderboardResponse{
		ContestID: contestID,
		Entries:   entries,
	})
}

// handleContestHistory returns the user's contest participation history.
func (a *App) handleContestHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse pagination params
	page := 1
	perPage := 10
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	offset := (page - 1) * perPage

	// Count total entries
	var total int
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM contest_participants WHERE user_id = $1
	`, userID).Scan(&total)
	if err != nil {
		a.log().Error("Failed to count contest history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Read-only query, use replica
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT c.id, c.name, c.status, c.starts_at, c.ends_at,
		       c.duration_type, c.asset_class,
		       cp.joined_at, cp.total_score, cp.final_rank, cp.final_prize_cents,
		       (SELECT COUNT(*) FROM contest_participants WHERE contest_id = c.id) AS total_participants,
		       (SELECT COUNT(*) FROM orders WHERE contest_id = c.id AND user_id = $1) AS trade_count
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1
		ORDER BY cp.joined_at DESC
		LIMIT $2 OFFSET $3
	`, userID, perPage, offset)
	if err != nil {
		a.log().Error("Failed to query contest history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	contests := []ContestHistoryEntry{}
	for rows.Next() {
		var entry ContestHistoryEntry
		var finalRank, finalPrize sql.NullInt32
		var startsAt, endsAt time.Time
		var durationType, assetClass sql.NullString
		if err := rows.Scan(&entry.ContestID, &entry.ContestName, &entry.Status,
			&startsAt, &endsAt, &durationType, &assetClass,
			&entry.JoinedAt, &entry.TotalScore, &finalRank, &finalPrize,
			&entry.TotalParticipants, &entry.TradeCount); err != nil {
			a.log().Error("Failed to scan contest history", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		entry.StartsAt = startsAt.Format(time.RFC3339)
		entry.EndsAt = endsAt.Format(time.RFC3339)
		if finalRank.Valid {
			rank := int(finalRank.Int32)
			entry.FinalRank = &rank
		}
		if finalPrize.Valid {
			prize := int(finalPrize.Int32)
			entry.FinalPrizeCents = &prize
		}
		if durationType.Valid {
			entry.DurationType = &durationType.String
		}
		if assetClass.Valid {
			entry.MarketType = &assetClass.String
		}
		// Calculate pnl_percent from total_score (score is portfolio value change)
		if entry.TotalScore != 0 {
			pnl := entry.TotalScore
			entry.PnlPercent = &pnl
		}
		contests = append(contests, entry)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate contest history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, ContestHistoryResponse{
		Contests: contests,
		Total:    total,
		Page:     page,
		PerPage:  perPage,
	})
}

// handleMyTournaments returns the user's tournaments filtered by status category.
func (a *App) handleMyTournaments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse pagination params
	page := 1
	perPage := 10
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	offset := (page - 1) * perPage

	// Parse status filter
	statusFilter := r.URL.Query().Get("status")
	validStatuses := map[string]bool{
		"active":    true,
		"upcoming":  true,
		"completed": true,
		"cancelled": true,
	}
	if statusFilter != "" && !validStatuses[statusFilter] {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidStatus)
		return
	}

	// Map status filter to DB statuses
	var statusCondition string
	switch statusFilter {
	case "active":
		statusCondition = "c.status = 'running'"
	case "upcoming":
		statusCondition = "c.status IN ('scheduled', 'registration_open')"
	case "completed":
		statusCondition = "c.status IN ('completed', 'settling')"
	case "cancelled":
		statusCondition = "c.status = 'cancelled'"
	default:
		// No filter - return all
		statusCondition = "1=1"
	}

	// Get counts for all categories in a single query
	var counts MyTournamentCounts
	countQuery := `
		SELECT
			COALESCE(SUM(CASE WHEN c.status = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status IN ('scheduled', 'registration_open') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status IN ('completed', 'settling') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status = 'cancelled' THEN 1 ELSE 0 END), 0)
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1
	`
	err := a.pool.Replica().QueryRowContext(ctx, countQuery, userID).Scan(
		&counts.Active, &counts.Upcoming, &counts.Completed, &counts.Cancelled,
	)
	if err != nil {
		a.log().Error("Failed to count tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Count total for current filter (for pagination)
	var total int
	totalQuery := `
		SELECT COUNT(*)
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1 AND ` + statusCondition
	err = a.pool.Replica().QueryRowContext(ctx, totalQuery, userID).Scan(&total)
	if err != nil {
		a.log().Error("Failed to count filtered tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Query filtered contests
	dataQuery := `
		SELECT c.id, c.name, c.status, c.starts_at, c.ends_at,
		       c.entry_fee_cents, c.asset_class, c.duration_type, c.is_free, c.qty_total,
		       cp.total_score, cp.final_rank, cp.final_prize_cents,
		       (SELECT COUNT(*) FROM contest_participants WHERE contest_id = c.id) AS total_participants
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1 AND ` + statusCondition + `
		ORDER BY c.starts_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := a.pool.Replica().QueryContext(ctx, dataQuery, userID, perPage, offset)
	if err != nil {
		a.log().Error("Failed to query tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	contests := []MyTournamentEntry{}
	for rows.Next() {
		var entry MyTournamentEntry
		var finalRank, finalPrize sql.NullInt32
		var startsAt, endsAt time.Time
		var assetClass, durationType sql.NullString
		if err := rows.Scan(&entry.ContestID, &entry.ContestName, &entry.Status,
			&startsAt, &endsAt, &entry.EntryFeeCents, &assetClass, &durationType,
			&entry.IsFree, &entry.QtyTotal,
			&entry.TotalScore, &finalRank, &finalPrize,
			&entry.TotalParticipants); err != nil {
			a.log().Error("Failed to scan tournament", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		entry.StartsAt = startsAt.Format(time.RFC3339)
		entry.EndsAt = endsAt.Format(time.RFC3339)
		if finalRank.Valid {
			rank := int(finalRank.Int32)
			entry.FinalRank = &rank
		}
		if finalPrize.Valid {
			prize := int(finalPrize.Int32)
			entry.FinalPrizeCents = &prize
		}
		if assetClass.Valid {
			entry.AssetClass = &assetClass.String
		}
		if durationType.Valid {
			entry.DurationType = &durationType.String
		}
		if entry.TotalScore != 0 {
			pnl := entry.TotalScore
			entry.PnlPercent = &pnl
		}
		contests = append(contests, entry)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, MyTournamentsResponse{
		Contests: contests,
		Total:    total,
		Page:     page,
		PerPage:  perPage,
		Counts:   counts,
	})
}

// handleGetWallet returns the user's wallet balance and status.
func (a *App) handleGetContestMyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Verify contest exists
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
		return
	}

	// Get participant data
	var totalScore float64
	var qtyTotal int64
	var finalRank sql.NullInt64
	var finalPrizeCents sql.NullInt64
	err = a.pool.Replica().QueryRowContext(ctx, `
		SELECT cp.total_score, cp.qty_total, cp.final_rank, cp.final_prize_cents
		FROM contest_participants cp
		WHERE cp.contest_id = $1 AND cp.user_id = $2
	`, contestID, userID).Scan(&totalScore, &qtyTotal, &finalRank, &finalPrizeCents)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.UserNotParticipated})
			return
		}
		a.log().Error("Failed to query contest participant", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get total participants
	var totalParticipants int
	err = a.pool.Replica().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1
	`, contestID).Scan(&totalParticipants)
	if err != nil {
		a.log().Error("Failed to count participants", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Calculate rank from leaderboard position if final_rank is not set
	rank := 0
	if finalRank.Valid {
		rank = int(finalRank.Int64)
	} else {
		err = a.pool.Replica().QueryRowContext(ctx, `
			SELECT COUNT(*) + 1
			FROM contest_participants
			WHERE contest_id = $1 AND total_score > $2
		`, contestID, totalScore).Scan(&rank)
		if err != nil {
			a.log().Error("Failed to calculate rank", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// PnL percent
	var pnlPercent float64
	if qtyTotal > 0 {
		pnlPercent = (totalScore / float64(qtyTotal)) * 100
	}

	rewardCents := 0
	if finalPrizeCents.Valid {
		rewardCents = int(finalPrizeCents.Int64)
	}

	// Get trade stats from positions
	var tradeCount, winningTrades, losingTrades int
	var bestTradePnl, worstTradePnl float64
	err = a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN realized_score > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN realized_score < 0 THEN 1 ELSE 0 END), 0),
			COALESCE(MAX(realized_score), 0),
			COALESCE(MIN(realized_score), 0)
		FROM positions
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID).Scan(&tradeCount, &winningTrades, &losingTrades, &bestTradePnl, &worstTradePnl)
	if err != nil {
		a.log().Error("Failed to query trade stats", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, ContestMyResultResponse{
		ContestID:         contestID,
		UserID:            userID,
		FinalRank:         rank,
		TotalParticipants: totalParticipants,
		TotalScore:        totalScore,
		PnlPercent:        math.Round(pnlPercent*100) / 100,
		RewardCents:       rewardCents,
		TradeCount:        tradeCount,
		WinningTrades:     winningTrades,
		LosingTrades:      losingTrades,
		BestTradePnl:      bestTradePnl,
		WorstTradePnl:     worstTradePnl,
	})
}

// handleGetContestMyTrades returns the authenticated user's trade history for a contest.
func (a *App) handleGetContestMyTrades(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	a.getContestTrades(w, r, ctx, contestID, userID)
}

// handleGetContestUserTrades returns a specific user's trade history for a contest.
// Public for completed contests; for non-completed contests, only the authenticated user
// can view their own trades (to prevent copy trading / front-running in live contests).
func (a *App) handleGetContestUserTrades(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}
	if targetUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.UserIDRequired})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(targetUserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidUserIDFormat})
		return
	}

	// Check contest status — other users' trades are only public for completed contests
	var contestStatus string
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1`, contestID,
	).Scan(&contestStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if contestStatus != "completed" {
		authUserID := auth.GetUserID(ctx)
		if authUserID == "" || authUserID != targetUserID {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": msg.TradeHistoryNotReady,
			})
			return
		}
	}

	a.getContestTrades(w, r, ctx, contestID, targetUserID)
}

// getContestTrades is a shared helper that returns trade history for a user in a contest.
func (a *App) getContestTrades(w http.ResponseWriter, r *http.Request, ctx context.Context, contestID, userID string) {
	// Verify contest exists
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
		return
	}

	// Query positions for this user in this contest
	rows, err := a.pool.Replica().QueryContext(ctx, `
		WITH fill_stats AS (
			SELECT
				f.symbol,
				SUM(f.qty) AS total_fill_qty,
				(ARRAY_AGG(f.fill_price ORDER BY f.created_at DESC))[1] AS last_fill_price
			FROM fills f
			WHERE f.contest_id = $1 AND f.user_id = $2
			GROUP BY f.symbol
		)
		SELECT
			p.position_id,
			p.symbol,
			p.side,
			p.qty_open + COALESCE(fs.total_fill_qty, 0) AS total_qty,
			p.entry_price,
			CASE WHEN p.closed_at IS NOT NULL THEN fs.last_fill_price ELSE NULL END AS exit_price,
			p.realized_score,
			p.opened_at,
			p.closed_at
		FROM positions p
		LEFT JOIN fill_stats fs ON fs.symbol = p.symbol
		WHERE p.contest_id = $1 AND p.user_id = $2
		ORDER BY p.opened_at DESC
	`, contestID, userID)
	if err != nil {
		a.log().Error("Failed to query contest trades", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	trades := make([]ContestTrade, 0)
	var totalPnl float64
	var winCount, lossCount int
	var totalWinPnl, totalLossPnl float64

	for rows.Next() {
		var positionID string
		var symbol string
		var side string
		var qty int64
		var entryPrice float64
		var exitPrice sql.NullFloat64
		var realizedScore float64
		var openedAt time.Time
		var closedAt sql.NullTime

		if err := rows.Scan(&positionID, &symbol, &side, &qty, &entryPrice, &exitPrice, &realizedScore, &openedAt, &closedAt); err != nil {
			a.log().Error("Failed to scan trade row", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		trade := ContestTrade{
			TradeID:    positionID,
			Symbol:     symbol,
			Side:       side,
			Qty:        qty,
			EntryPrice: entryPrice,
			OpenedAt:   openedAt.Format(time.RFC3339),
		}

		if closedAt.Valid {
			trade.Status = "closed"
			closedAtStr := closedAt.Time.Format(time.RFC3339)
			trade.ClosedAt = &closedAtStr
			trade.Pnl = &realizedScore

			// Calculate PnL percent based on entry value
			if entryPrice > 0 && qty > 0 {
				entryValue := entryPrice * float64(qty)
				pnlPct := (realizedScore / entryValue) * 100
				pnlPctRounded := math.Round(pnlPct*100) / 100
				trade.PnlPercent = &pnlPctRounded
			}

			totalPnl += realizedScore
			if realizedScore > 0 {
				winCount++
				totalWinPnl += realizedScore
			} else if realizedScore < 0 {
				lossCount++
				totalLossPnl += realizedScore
			}
		} else {
			trade.Status = "open"
		}

		if exitPrice.Valid {
			ep := exitPrice.Float64
			trade.ExitPrice = &ep
		}

		trades = append(trades, trade)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating trade rows", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	totalTrades := len(trades)
	var avgWin, avgLoss float64
	if winCount > 0 {
		avgWin = math.Round(totalWinPnl/float64(winCount)*100) / 100
	}
	if lossCount > 0 {
		avgLoss = math.Round(totalLossPnl/float64(lossCount)*100) / 100
	}

	writeJSON(w, http.StatusOK, ContestTradesResponse{
		Trades: trades,
		Total:  totalTrades,
		Summary: ContestTradeSummary{
			TotalTrades:   totalTrades,
			WinningTrades: winCount,
			LosingTrades:  lossCount,
			TotalPnl:      math.Round(totalPnl*100) / 100,
			AvgWin:        avgWin,
			AvgLoss:       avgLoss,
		},
	})
}

// handleApplyReferral applies a referral code for the authenticated user.
// POST /api/user/referral/apply
// Used primarily by OAuth signups where the referral code couldn't be applied during registration.
func (a *App) handleApplyReferral(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req ReferralApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ReferralCodeRequired})
		return
	}

	code := strings.ToUpper(req.Code)

	// Check if user already has a referral (was already referred by someone)
	var existingReferral int
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM referrals WHERE referred_id = $1`,
		userID,
	).Scan(&existingReferral)
	if err != nil {
		a.log().Error("Failed to check existing referral", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if existingReferral > 0 {
		writeJSON(w, http.StatusOK, ReferralApplyResponse{
			Applied: false,
			Message: "referral already applied",
		})
		return
	}

	// Look up the referral code
	var referrerID string
	var isActive bool
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT user_id, is_active FROM referral_codes WHERE code = $1`,
		code,
	).Scan(&referrerID, &isActive)

	if err != nil || !isActive {
		writeJSON(w, http.StatusOK, ReferralApplyResponse{
			Applied: false,
			Message: "invalid or inactive referral code",
		})
		return
	}

	// Prevent self-referral
	if referrerID == userID {
		writeJSON(w, http.StatusOK, ReferralApplyResponse{
			Applied: false,
			Message: "cannot use your own referral code",
		})
		return
	}

	// Create the referral entry
	_, err = a.pool.Primary().ExecContext(ctx, `
		INSERT INTO referrals (referrer_id, referred_id, code, status)
		VALUES ($1, $2, $3, 'pending')
		ON CONFLICT (referred_id) DO NOTHING
	`, referrerID, userID, code)
	if err != nil {
		a.log().Error("Failed to create referral entry",
			zap.String("referral_code", code),
			zap.String("referred_id", userID),
			zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("Referral applied via /referral/apply",
		zap.String("referrer_id", referrerID),
		zap.String("referred_id", userID),
		zap.String("code", code))

	writeJSON(w, http.StatusOK, ReferralApplyResponse{
		Applied: true,
		Message: "referral code applied successfully",
	})
}

// handleGetContestStats returns aggregated contest statistics for the authenticated user.
// GET /api/user/me/contest-stats
func (a *App) handleGetContestStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var stats UserContestStatsResponse
	var bestRank sql.NullInt64
	var avgRank sql.NullFloat64
	var favoriteMarket sql.NullString

	// Get aggregate stats from contest_participants
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_contests,
			COALESCE(SUM(CASE WHEN cp.final_rank = 1 THEN 1 ELSE 0 END), 0) AS total_wins,
			COALESCE(SUM(cp.final_prize_cents), 0) AS total_prizes_cents,
			MIN(cp.final_rank) AS best_rank,
			AVG(cp.final_rank) AS average_rank,
			COALESCE(SUM(cp.total_score), 0) AS total_pnl
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1
		  AND c.status = 'completed'
	`, userID).Scan(
		&stats.TotalContests,
		&stats.TotalWins,
		&stats.TotalPrizesCents,
		&bestRank,
		&avgRank,
		&stats.TotalPnl,
	)
	if err != nil {
		a.log().Error("Failed to query contest stats", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if bestRank.Valid {
		stats.BestRank = int(bestRank.Int64)
	}
	if avgRank.Valid {
		stats.AverageRank = math.Round(avgRank.Float64*100) / 100
	}

	// Calculate win rate
	if stats.TotalContests > 0 {
		stats.WinRate = math.Round(float64(stats.TotalWins)/float64(stats.TotalContests)*10000) / 100
	}

	// Get favorite market (most traded asset class)
	err = a.pool.Replica().QueryRowContext(ctx, `
		SELECT c.asset_class
		FROM contest_participants cp
		JOIN contests c ON c.id = cp.contest_id
		WHERE cp.user_id = $1
		  AND c.status = 'completed'
		GROUP BY c.asset_class
		ORDER BY COUNT(*) DESC
		LIMIT 1
	`, userID).Scan(&favoriteMarket)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.log().Error("Failed to query favorite market", zap.Error(err), zap.String("user_id", userID))
		// Non-critical, continue without favorite market
	}
	if favoriteMarket.Valid {
		stats.FavoriteMarket = &favoriteMarket.String
	}

	writeJSON(w, http.StatusOK, stats)
}

// seedAdminUsers creates default admin and test user accounts if they don't exist.
// Runs at startup and logs results without blocking the service.
