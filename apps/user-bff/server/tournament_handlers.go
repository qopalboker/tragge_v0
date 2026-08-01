package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ====================
// IRST & Jalali Helpers
// ====================

// tehranLoc is the Asia/Tehran timezone, which is DST-aware:
// IRST (UTC+03:30) in winter, IRDT (UTC+04:30) in summer (~Mar 21 to ~Sep 21).
var tehranLoc *time.Location

func init() {
	var err error
	tehranLoc, err = time.LoadLocation("Asia/Tehran")
	if err != nil {
		log.Fatalf("Failed to load Asia/Tehran timezone: %v", err)
	}
}

// toIRST converts a time to Iran local time (IRST in winter, IRDT in summer).
func toIRST(t time.Time) time.Time {
	return t.In(tehranLoc)
}

// DualTime represents a timestamp in both UTC and IRST formats.
type DualTime struct {
	UTC  string `json:"utc"`
	IRST string `json:"irst"`
}

// newDualTime creates a DualTime from a time.Time value.
func newDualTime(t time.Time) DualTime {
	utc := t.UTC()
	irst := toIRST(utc)
	jy, jm, jd := gregorianToJalali(irst.Year(), int(irst.Month()), irst.Day())
	return DualTime{
		UTC:  utc.Format(time.RFC3339),
		IRST: fmt.Sprintf("%04d/%02d/%02d %02d:%02d %s", jy, jm, jd, irst.Hour(), irst.Minute(), irst.Format("MST")),
	}
}

// newDualTimeDate creates a DualTime for a date (no time component).
func newDualTimeDate(t time.Time) DualTime {
	utc := t.UTC()
	irst := toIRST(utc)
	jy, jm, jd := gregorianToJalali(irst.Year(), int(irst.Month()), irst.Day())
	return DualTime{
		UTC:  utc.Format("2006-01-02"),
		IRST: fmt.Sprintf("%04d/%02d/%02d", jy, jm, jd),
	}
}

// gregorianToJalali converts a Gregorian date to a Jalali (Solar Hijri) date.
// Algorithm based on the well-known conversion formula.
func gregorianToJalali(gy, gm, gd int) (jy, jm, jd int) {
	var gdm = [12]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}

	gy2 := gy + 1
	if gm > 2 {
		gy2 = gy
	}

	days := 355666 + (365 * gy) + ((gy2 + 3) / 4) - ((gy2 + 99) / 100) + ((gy2 + 399) / 400) + gd + gdm[gm-1]

	jy = -1595 + (33 * (days / 12053))
	days %= 12053

	jy += 4 * (days / 1461)
	days %= 1461

	if days > 365 {
		jy += (days - 1) / 365
		days = (days - 1) % 365
	}

	if days < 186 {
		jm = 1 + (days / 31)
		jd = 1 + (days % 31)
	} else {
		jm = 7 + ((days - 186) / 30)
		jd = 1 + ((days - 186) % 30)
	}

	return jy, jm, jd
}

// jalaliWeekday returns the Persian weekday name for a time in IRST.
func jalaliWeekday(t time.Time) string {
	irst := toIRST(t)
	weekdays := map[time.Weekday]string{
		time.Saturday:  "شنبه",
		time.Sunday:    "یکشنبه",
		time.Monday:    "دوشنبه",
		time.Tuesday:   "سه‌شنبه",
		time.Wednesday: "چهارشنبه",
		time.Thursday:  "پنجشنبه",
		time.Friday:    "جمعه",
	}
	return weekdays[irst.Weekday()]
}

// isWeekendIRST returns true if the given time falls on a weekend day (Friday/Saturday)
// in Iran Standard Time. Iran's weekend is Friday-Saturday.
func isWeekendIRST(t time.Time) bool {
	irst := toIRST(t)
	wd := irst.Weekday()
	return wd == time.Friday || wd == time.Saturday
}

// ====================
// Duration Type Mapping
// ====================

// mapDurationType normalizes task-specified filter values to the codebase values.
// Accepts both formats: quick_30m↔rush_30min, free_1h↔hourly.
func mapDurationType(input string) string {
	switch input {
	case "quick_30m":
		return "rush_30min"
	case "free_1h":
		return "hourly"
	case "rush_30min", "hourly", "four_hour", "daily", "weekly":
		return input
	default:
		return ""
	}
}

// mapStatusFilter maps user-facing status filter values to database status values.
func mapStatusFilter(input string) []string {
	switch input {
	case "registration_open":
		return []string{"registration_open"}
	case "active":
		return []string{"running"}
	case "upcoming":
		return []string{"scheduled", "registration_open"}
	default:
		return nil
	}
}

// validMarketTypes maps user-facing market_type filter values.
var validMarketTypes = map[string]bool{
	"crypto": true,
	"forex":  true,
	"stocks": true,
	"mixed":  true,
}

// ====================
// Tournament Listing Response Types
// ====================

// TournamentListItem represents a tournament in the listing response.
type TournamentListItem struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Status              string   `json:"status"`
	MarketType          string   `json:"market_type"`
	DurationType        string   `json:"duration_type"`
	DurationMinutes     int      `json:"duration_minutes"`
	StartTime           DualTime `json:"start_time"`
	EndTime             DualTime `json:"end_time"`
	EntryFeeCents       int      `json:"entry_fee_cents"`
	IsFree              bool     `json:"is_free"`
	PrizePoolCents      int64    `json:"prize_pool_cents"`
	CurrentParticipants int      `json:"current_participants"`
	MaxParticipants     *int     `json:"max_participants,omitempty"`
	CommissionRate      float64  `json:"commission_rate"`
}

// TournamentListResponse is the response for GET /tournaments.
type TournamentListResponse struct {
	Tournaments []TournamentListItem `json:"tournaments"`
	NextCursor  *string              `json:"next_cursor"`
	TotalCount  int                  `json:"total_count"`
	ServerTime  DualTime             `json:"server_time"`
}

// TournamentTierItem represents a single tier option within a grouped tournament.
type TournamentTierItem struct {
	ContestID           string `json:"contest_id"`
	EntryFeeCents       int    `json:"entry_fee_cents"`
	TierLabel           string `json:"tier_label,omitempty"`
	IsFree              bool   `json:"is_free"`
	PrizePoolCents      int64  `json:"prize_pool_cents"`
	CurrentParticipants int    `json:"current_participants"`
	MaxParticipants     *int   `json:"max_participants,omitempty"`
}

// TournamentGroup represents contests from the same template grouped together.
type TournamentGroup struct {
	TemplateID      string               `json:"template_id"`
	Name            string               `json:"name"`
	Description     string               `json:"description,omitempty"`
	Status          string               `json:"status"`
	MarketType      string               `json:"market_type"`
	DurationType    string               `json:"duration_type"`
	DurationMinutes int                  `json:"duration_minutes"`
	StartTime       DualTime             `json:"start_time"`
	EndTime         DualTime             `json:"end_time"`
	CommissionRate  float64              `json:"commission_rate"`
	Tiers           []TournamentTierItem `json:"tiers"`
}

// TournamentGroupedResponse is the response for GET /tournaments?group_by=template.
type TournamentGroupedResponse struct {
	Groups     []TournamentGroup    `json:"groups"`
	Ungrouped  []TournamentListItem `json:"ungrouped"`
	TotalCount int                  `json:"total_count"`
	ServerTime DualTime             `json:"server_time"`
}

// TournamentDetailResponse is the response for GET /tournaments/:id.
type TournamentDetailResponse struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Description         string             `json:"description,omitempty"`
	Status              string             `json:"status"`
	MarketType          string             `json:"market_type"`
	DurationType        string             `json:"duration_type"`
	DurationMinutes     int                `json:"duration_minutes"`
	StartTime           DualTime           `json:"start_time"`
	EndTime             DualTime           `json:"end_time"`
	EntryFeeCents       int                `json:"entry_fee_cents"`
	IsFree              bool               `json:"is_free"`
	PrizePoolCents      int64              `json:"prize_pool_cents"`
	GrossPrizeCents     int64              `json:"gross_prize_pool_cents"`
	CurrentParticipants int                `json:"current_participants"`
	MaxParticipants     *int               `json:"max_participants,omitempty"`
	TimeRemainingMs     int64              `json:"time_remaining_ms"`
	UserJoined          bool               `json:"user_joined"`
	Symbols             []string           `json:"symbols"`
	CommissionRate      float64            `json:"commission_rate"`
	PrizeDistribution   []PrizeRankPreview `json:"prize_distribution"`
	ServerTime          DualTime           `json:"server_time"`
}

// ====================
// Calendar Response Types
// ====================

// CalendarTournament represents a tournament in the calendar response.
type CalendarTournament struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	IsActive        bool     `json:"is_active"`
	MarketType      string   `json:"market_type"`
	DurationType    string   `json:"duration_type"`
	DurationMinutes int      `json:"duration_minutes"`
	StartTime       DualTime `json:"start_time"`
	EndTime         DualTime `json:"end_time"`
	EntryFeeCents   int      `json:"entry_fee_cents"`
	IsFree          bool     `json:"is_free"`
	PrizePoolCents  int64    `json:"prize_pool_cents"`
	Participants    int      `json:"participants"`
	MaxParticipants *int     `json:"max_participants,omitempty"`
	UserRegistered  bool     `json:"user_registered"`
}

// CalendarDayGroups holds tournaments grouped by duration type for a day.
type CalendarDayGroups struct {
	Rush30min []CalendarTournament `json:"rush_30min,omitempty"`
	Hourly    []CalendarTournament `json:"hourly,omitempty"`
	FourHour  []CalendarTournament `json:"four_hour,omitempty"`
	Daily     []CalendarTournament `json:"daily,omitempty"`
	Weekly    []CalendarTournament `json:"weekly,omitempty"`
}

// CalendarDay represents a single day in the calendar.
type CalendarDay struct {
	DateUTC       string            `json:"date_utc"`
	JalaliDate    string            `json:"jalali_date"`
	JalaliWeekday string            `json:"jalali_weekday"`
	Weekday       string            `json:"weekday"`
	IsWeekend     bool              `json:"is_weekend"`
	CryptoOnly    bool              `json:"crypto_only"`
	Groups        CalendarDayGroups `json:"groups"`
	Total         int               `json:"total"`
}

// TournamentCalendarResponse is the response for GET /tournaments/calendar.
type TournamentCalendarResponse struct {
	From  DualTime      `json:"from"`
	To    DualTime      `json:"to"`
	Days  []CalendarDay `json:"days"`
	Total int           `json:"total"`
}

// ====================
// Redis Cache Helpers
// ====================

const tournamentCacheTTL = 30 * time.Second

// tournamentCacheKey generates a cache key for tournament list queries.
func tournamentCacheKey(params string) string {
	hash := sha256.Sum256([]byte(params))
	return "tournaments:list:" + fmt.Sprintf("%x", hash[:8])
}

// ====================
// Cursor Pagination Helpers
// ====================

// tournamentCursor represents a decoded pagination cursor.
type tournamentCursor struct {
	SortValue string `json:"v"`
	ID        string `json:"id"`
}

// encodeCursor encodes a cursor to a base64 string.
func encodeCursor(sortValue, id string) string {
	data, _ := json.Marshal(tournamentCursor{SortValue: sortValue, ID: id})
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a base64 cursor string.
func decodeCursor(cursor string) (sortValue, id string, err error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", fmt.Errorf("invalid cursor encoding")
	}
	var c tournamentCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return "", "", fmt.Errorf("invalid cursor format")
	}
	if c.ID == "" {
		return "", "", fmt.Errorf("invalid cursor: missing id")
	}
	return c.SortValue, c.ID, nil
}

// ====================
// Tournament Listing Handler (Task 8.1)
// ====================

// handleListTournaments returns tournaments with filters, sorting, cursor pagination, and Redis caching.
func (a *App) handleListTournaments(w http.ResponseWriter, r *http.Request) {
	// If group_by=template is requested, delegate to the grouped handler
	if r.URL.Query().Get("group_by") == "template" {
		a.handleListTournamentsGrouped(w, r)
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	// Parse filter parameters
	marketType := r.URL.Query().Get("market_type")
	durationType := r.URL.Query().Get("duration_type")
	statusFilter := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort_by")
	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	// Validate and normalize filters
	if marketType != "" && !validMarketTypes[marketType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidMarketType})
		return
	}

	normalizedDuration := ""
	if durationType != "" {
		normalizedDuration = mapDurationType(durationType)
		if normalizedDuration == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDurationType})
			return
		}
	}

	var statusValues []string
	if statusFilter != "" {
		statusValues = mapStatusFilter(statusFilter)
		if statusValues == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidStatus})
			return
		}
	}

	// Validate sort_by
	validSorts := map[string]bool{"start_time": true, "prize_pool": true, "participant_count": true}
	if sortBy == "" {
		sortBy = "start_time"
	}
	if !validSorts[sortBy] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidSortBy})
		return
	}

	// Parse limit
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 100 {
		limit = 100
	}

	// Try Redis cache (only for first page, no cursor)
	cacheKey := ""
	if cursor == "" && a.redis != nil {
		cacheParams := fmt.Sprintf("mt=%s&dt=%s&st=%s&sb=%s&l=%d", marketType, normalizedDuration, statusFilter, sortBy, limit)
		cacheKey = tournamentCacheKey(cacheParams)

		cached, err := a.redis.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(cached))
			return
		}
	}

	// Build SQL query
	query := `
		SELECT c.id, c.name, COALESCE(c.description, ''), c.status, c.asset_class, c.duration_type,
		       c.duration_minutes, c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		       c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0),
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count
		FROM contests c
		WHERE 1=1`

	var args []interface{}
	argIdx := 1

	// Default status filter: show registration_open, scheduled, running
	if len(statusValues) > 0 {
		placeholders := make([]string, len(statusValues))
		for i, sv := range statusValues {
			placeholders[i] = "$" + strconv.Itoa(argIdx)
			args = append(args, sv)
			argIdx++
		}
		query += " AND c.status IN (" + strings.Join(placeholders, ",") + ")"
	} else {
		query += " AND c.status IN ('registration_open', 'scheduled', 'running')"
	}

	// Market type filter
	if marketType != "" {
		query += " AND c.asset_class = $" + strconv.Itoa(argIdx)
		args = append(args, marketType)
		argIdx++
	}

	// Duration type filter
	if normalizedDuration != "" {
		query += " AND c.duration_type = $" + strconv.Itoa(argIdx)
		args = append(args, normalizedDuration)
		argIdx++
	}

	// Cursor-based pagination
	if cursor != "" {
		cursorVal, cursorID, err := decodeCursor(cursor)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidCursor})
			return
		}

		switch sortBy {
		case "start_time":
			query += fmt.Sprintf(" AND (c.starts_at, c.id) > ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, cursorVal, cursorID)
			argIdx += 2
		case "prize_pool":
			// For descending sort, use < instead of >
			query += fmt.Sprintf(` AND (
				(SELECT COUNT(*) FROM contest_participants cp2 WHERE cp2.contest_id = c.id) * c.entry_fee_cents < $%d
				OR (
					(SELECT COUNT(*) FROM contest_participants cp3 WHERE cp3.contest_id = c.id) * c.entry_fee_cents = $%d
					AND c.id > $%d
				)
			)`, argIdx, argIdx, argIdx+1)
			args = append(args, cursorVal, cursorID)
			argIdx += 2
		case "participant_count":
			query += fmt.Sprintf(` AND (
				(SELECT COUNT(*) FROM contest_participants cp2 WHERE cp2.contest_id = c.id) < $%d
				OR (
					(SELECT COUNT(*) FROM contest_participants cp3 WHERE cp3.contest_id = c.id) = $%d
					AND c.id > $%d
				)
			)`, argIdx, argIdx, argIdx+1)
			args = append(args, cursorVal, cursorID)
			argIdx += 2
		}
	}

	// Sort order
	switch sortBy {
	case "start_time":
		query += " ORDER BY c.starts_at ASC, c.id ASC"
	case "prize_pool":
		query += " ORDER BY participant_count * c.entry_fee_cents DESC, c.id ASC"
	case "participant_count":
		query += " ORDER BY participant_count DESC, c.id ASC"
	}

	// Fetch limit + 1 to determine if there are more results
	query += fmt.Sprintf(" LIMIT %d", limit+1)

	// Count total (for first page only)
	var totalCount int
	if cursor == "" {
		countQuery := `
			SELECT COUNT(*)
			FROM contests c
			WHERE 1=1`
		var countArgs []interface{}
		countArgIdx := 1

		if len(statusValues) > 0 {
			placeholders := make([]string, len(statusValues))
			for i, sv := range statusValues {
				placeholders[i] = "$" + strconv.Itoa(countArgIdx)
				countArgs = append(countArgs, sv)
				countArgIdx++
			}
			countQuery += " AND c.status IN (" + strings.Join(placeholders, ",") + ")"
		} else {
			countQuery += " AND c.status IN ('registration_open', 'scheduled', 'running')"
		}
		if marketType != "" {
			countQuery += " AND c.asset_class = $" + strconv.Itoa(countArgIdx)
			countArgs = append(countArgs, marketType)
			countArgIdx++
		}
		if normalizedDuration != "" {
			countQuery += " AND c.duration_type = $" + strconv.Itoa(countArgIdx)
			countArgs = append(countArgs, normalizedDuration)
		}

		_ = a.pool.Replica().QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
	}

	// Execute query
	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to query tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	tournaments := make([]TournamentListItem, 0, limit)
	var lastID string
	var lastSortValue string

	count := 0
	for rows.Next() {
		count++
		if count > limit {
			break // We have more results
		}

		var t TournamentListItem
		var startsAt, endsAt time.Time
		var commissionRate float64
		var platformFeeBps int
		var participantCount int

		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Status, &t.MarketType, &t.DurationType,
			&t.DurationMinutes, &startsAt, &endsAt, &t.EntryFeeCents, &t.IsFree,
			&t.MaxParticipants, &commissionRate, &platformFeeBps, &participantCount); err != nil {
			a.log().Error("Failed to scan tournament", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		t.CurrentParticipants = participantCount
		t.CommissionRate = commissionRate
		t.StartTime = newDualTime(startsAt)
		t.EndTime = newDualTime(endsAt)

		// Calculate prize pool
		if !t.IsFree && t.EntryFeeCents > 0 && participantCount > 0 {
			feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
			gross := int64(t.EntryFeeCents) * int64(participantCount)
			t.PrizePoolCents = (gross * int64(10000-feeBps)) / 10000
		}

		// Track cursor values
		lastID = t.ID
		switch sortBy {
		case "start_time":
			lastSortValue = startsAt.Format(time.RFC3339Nano)
		case "prize_pool":
			lastSortValue = strconv.FormatInt(int64(t.EntryFeeCents)*int64(participantCount), 10)
		case "participant_count":
			lastSortValue = strconv.Itoa(participantCount)
		}

		tournaments = append(tournaments, t)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Build response
	resp := TournamentListResponse{
		Tournaments: tournaments,
		TotalCount:  totalCount,
		ServerTime:  newDualTime(now),
	}

	// Set next cursor if there are more results
	if count > limit {
		nc := encodeCursor(lastSortValue, lastID)
		resp.NextCursor = &nc
	}

	// Cache result in Redis (only for first page)
	if cacheKey != "" && a.redis != nil {
		if data, err := json.Marshal(resp); err == nil {
			_ = a.redis.Set(ctx, cacheKey, string(data), tournamentCacheTTL).Err()
		}
	}

	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, http.StatusOK, resp)
}

// ====================
// Grouped Tournament Listing Handler
// ====================

// handleListTournamentsGrouped returns tournaments grouped by template.
// Contests sharing the same template_id and start time are grouped together
// with their tier information.
func (a *App) handleListTournamentsGrouped(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()

	// Parse filter parameters
	marketType := r.URL.Query().Get("market_type")
	durationType := r.URL.Query().Get("duration_type")
	statusFilter := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	// Validate filters
	if marketType != "" && !validMarketTypes[marketType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidMarketType})
		return
	}

	normalizedDuration := ""
	if durationType != "" {
		normalizedDuration = mapDurationType(durationType)
		if normalizedDuration == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDurationType})
			return
		}
	}

	var statusValues []string
	if statusFilter != "" {
		statusValues = mapStatusFilter(statusFilter)
		if statusValues == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidStatus})
			return
		}
	}

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 200 {
		limit = 200
	}

	// Try Redis cache
	cacheKey := ""
	if a.redis != nil {
		cacheParams := fmt.Sprintf("grouped:mt=%s&dt=%s&st=%s&l=%d", marketType, normalizedDuration, statusFilter, limit)
		cacheKey = tournamentCacheKey(cacheParams)

		cached, err := a.redis.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(cached))
			return
		}
	}

	// Build SQL query with tier information
	query := `
		SELECT c.id, c.name, COALESCE(c.description, ''), c.status, c.asset_class, c.duration_type,
		       c.duration_minutes, c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		       c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0),
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count,
		       c.template_id,
		       COALESCE(tet.label, '') as tier_label,
		       COALESCE(tt.name, '') as template_name
		FROM contests c
		LEFT JOIN template_entry_tiers tet ON tet.id = c.tier_id
		LEFT JOIN tournament_templates tt ON tt.id = c.template_id
		WHERE 1=1`

	var args []interface{}
	argIdx := 1

	// Status filter
	if len(statusValues) > 0 {
		placeholders := make([]string, len(statusValues))
		for i, sv := range statusValues {
			placeholders[i] = "$" + strconv.Itoa(argIdx)
			args = append(args, sv)
			argIdx++
		}
		query += " AND c.status IN (" + strings.Join(placeholders, ",") + ")"
	} else {
		query += " AND c.status IN ('registration_open', 'scheduled', 'running')"
	}

	if marketType != "" {
		query += " AND c.asset_class = $" + strconv.Itoa(argIdx)
		args = append(args, marketType)
		argIdx++
	}

	if normalizedDuration != "" {
		query += " AND c.duration_type = $" + strconv.Itoa(argIdx)
		args = append(args, normalizedDuration)
		argIdx++
	}

	query += " ORDER BY c.starts_at ASC, c.template_id ASC NULLS LAST, c.entry_fee_cents ASC"
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to query grouped tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	// Scan rows and group by (template_id, starts_at)
	type groupKey struct {
		TemplateID string
		StartsAt   time.Time
	}
	groupOrder := []groupKey{}
	groups := map[groupKey]*TournamentGroup{}
	ungrouped := []TournamentListItem{}

	for rows.Next() {
		var id, name, description, status, assetClass, durType string
		var durationMinutes, entryFeeCents, platformFeeBps, participantCount int
		var startsAt, endsAt time.Time
		var isFree bool
		var maxParticipants *int
		var commissionRate float64
		var templateID *string
		var tierLabel string
		var templateName string

		if err := rows.Scan(&id, &name, &description, &status, &assetClass, &durType,
			&durationMinutes, &startsAt, &endsAt, &entryFeeCents, &isFree,
			&maxParticipants, &commissionRate, &platformFeeBps, &participantCount,
			&templateID, &tierLabel, &templateName); err != nil {
			a.log().Error("Failed to scan grouped tournament", zap.Error(err))
			continue
		}

		startDT := newDualTime(startsAt)
		endDT := newDualTime(endsAt)

		// Calculate prize pool
		var prizePoolCents int64
		if !isFree && entryFeeCents > 0 && participantCount > 0 {
			feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
			gross := int64(entryFeeCents) * int64(participantCount)
			prizePoolCents = (gross * int64(10000-feeBps)) / 10000
		}

		// If no template_id, put in ungrouped
		if templateID == nil || *templateID == "" {
			ungrouped = append(ungrouped, TournamentListItem{
				ID:                  id,
				Name:                name,
				Description:         description,
				Status:              status,
				MarketType:          assetClass,
				DurationType:        durType,
				DurationMinutes:     durationMinutes,
				StartTime:           startDT,
				EndTime:             endDT,
				EntryFeeCents:       entryFeeCents,
				IsFree:              isFree,
				PrizePoolCents:      prizePoolCents,
				CurrentParticipants: participantCount,
				MaxParticipants:     maxParticipants,
				CommissionRate:      commissionRate,
			})
			continue
		}

		key := groupKey{TemplateID: *templateID, StartsAt: startsAt.Truncate(time.Minute)}
		g, exists := groups[key]
		if !exists {
			groupName := name
			if templateName != "" {
				groupName = templateName
			}
			g = &TournamentGroup{
				TemplateID:      *templateID,
				Name:            groupName,
				Description:     description,
				Status:          status,
				MarketType:      assetClass,
				DurationType:    durType,
				DurationMinutes: durationMinutes,
				StartTime:       startDT,
				EndTime:         endDT,
				CommissionRate:  commissionRate,
				Tiers:           []TournamentTierItem{},
			}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}

		g.Tiers = append(g.Tiers, TournamentTierItem{
			ContestID:           id,
			EntryFeeCents:       entryFeeCents,
			TierLabel:           tierLabel,
			IsFree:              isFree,
			PrizePoolCents:      prizePoolCents,
			CurrentParticipants: participantCount,
			MaxParticipants:     maxParticipants,
		})
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate grouped tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Build ordered groups list; groups with only 1 tier go to ungrouped
	resultGroups := make([]TournamentGroup, 0, len(groupOrder))
	for _, key := range groupOrder {
		g := groups[key]
		if len(g.Tiers) == 1 {
			tier := g.Tiers[0]
			ungrouped = append(ungrouped, TournamentListItem{
				ID:                  tier.ContestID,
				Name:                g.Name,
				Description:         g.Description,
				Status:              g.Status,
				MarketType:          g.MarketType,
				DurationType:        g.DurationType,
				DurationMinutes:     g.DurationMinutes,
				StartTime:           g.StartTime,
				EndTime:             g.EndTime,
				EntryFeeCents:       tier.EntryFeeCents,
				IsFree:              tier.IsFree,
				PrizePoolCents:      tier.PrizePoolCents,
				CurrentParticipants: tier.CurrentParticipants,
				MaxParticipants:     tier.MaxParticipants,
				CommissionRate:      g.CommissionRate,
			})
		} else {
			resultGroups = append(resultGroups, *g)
		}
	}

	resp := TournamentGroupedResponse{
		Groups:     resultGroups,
		Ungrouped:  ungrouped,
		TotalCount: len(resultGroups) + len(ungrouped),
		ServerTime: newDualTime(now),
	}

	// Cache result in Redis
	if cacheKey != "" && a.redis != nil {
		if data, err := json.Marshal(resp); err == nil {
			_ = a.redis.Set(ctx, cacheKey, string(data), tournamentCacheTTL).Err()
		}
	}

	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, http.StatusOK, resp)
}

// ====================
// Tournament Details Handler (Task 8.1)
// ====================

// handleGetTournamentDetails returns detailed tournament information with prize distribution.
func (a *App) handleGetTournamentDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TournamentIDRequired})
		return
	}

	// Query tournament details
	var resp TournamentDetailResponse
	var startsAt, endsAt time.Time
	var commissionRate float64
	var platformFeeBps int
	var minParticipants int

	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT c.id, c.name, COALESCE(c.description, ''), c.status, c.asset_class, c.duration_type,
		       c.duration_minutes, c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		       c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0),
		       COALESCE(c.min_participants, 2),
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id)
		FROM contests c
		WHERE c.id = $1
	`, contestID).Scan(
		&resp.ID, &resp.Name, &resp.Description, &resp.Status, &resp.MarketType, &resp.DurationType,
		&resp.DurationMinutes, &startsAt, &endsAt, &resp.EntryFeeCents, &resp.IsFree,
		&resp.MaxParticipants, &commissionRate, &platformFeeBps, &minParticipants,
		&resp.CurrentParticipants,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.TournamentNotFound})
			return
		}
		a.log().Error("Failed to query tournament details", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Set dual times
	resp.StartTime = newDualTime(startsAt)
	resp.EndTime = newDualTime(endsAt)
	resp.ServerTime = newDualTime(now)
	resp.CommissionRate = commissionRate

	// Calculate time remaining
	remaining := endsAt.Sub(now)
	if remaining > 0 {
		resp.TimeRemainingMs = remaining.Milliseconds()
	}

	// Calculate prize pool
	effectiveFeeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
	if !resp.IsFree && resp.EntryFeeCents > 0 && resp.CurrentParticipants > 0 {
		gross := int64(resp.EntryFeeCents) * int64(resp.CurrentParticipants)
		resp.GrossPrizeCents = gross
		resp.PrizePoolCents = (gross * int64(10000-effectiveFeeBps)) / 10000
	}

	// Calculate prize distribution
	if resp.CurrentParticipants > 0 && resp.EntryFeeCents > 0 {
		_, _, prizes := CalculatePrizeDistribution(
			resp.CurrentParticipants, resp.EntryFeeCents, effectiveFeeBps,
		)
		resp.PrizeDistribution = prizes
	}
	if resp.PrizeDistribution == nil {
		resp.PrizeDistribution = []PrizeRankPreview{}
	}

	// Query symbols
	symbolRows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT symbol FROM contest_symbols WHERE contest_id = $1 AND enabled = true ORDER BY symbol`,
		contestID,
	)
	if err != nil {
		a.log().Error("Failed to query tournament symbols", zap.Error(err))
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

	// Check if user is authenticated and has joined
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

	writeJSON(w, http.StatusOK, resp)
}

// ====================
// Tournament Calendar Handler (Task 8.2)
// ====================

// handleTournamentCalendar returns a day-by-day tournament calendar with Jalali dates.
func (a *App) handleTournamentCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse date parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var fromDate, toDate time.Time
	var err error

	if fromStr != "" {
		fromDate, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDateFormat})
			return
		}
	} else {
		fromDate = time.Now().UTC().Truncate(24 * time.Hour)
	}

	if toStr != "" {
		toDate, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDateFormat})
			return
		}
		toDate = toDate.Add(24*time.Hour - time.Second)
	} else {
		toDate = fromDate.Add(7 * 24 * time.Hour)
	}

	// Validate date range (max 30 days)
	if toDate.Sub(fromDate) > 30*24*time.Hour {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.DateRangeTooLarge})
		return
	}

	// Parse optional filters
	assetClass := r.URL.Query().Get("asset_class")
	durationType := r.URL.Query().Get("duration_type")

	// Get user ID if authenticated
	userID := auth.GetUserID(ctx)

	// Build query
	userRegisteredClause := "FALSE as user_registered"
	var args []interface{}
	dateStartIdx := 1

	if userID != "" {
		userRegisteredClause = "EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id = c.id AND cp.user_id = $1) as user_registered"
		args = append(args, userID)
		dateStartIdx = 2
	}

	args = append(args, fromDate, toDate)

	query := fmt.Sprintf(`
		SELECT
			c.id, c.name, c.asset_class, c.duration_type, c.duration_minutes,
			c.entry_fee_cents, c.starts_at, c.ends_at,
			c.status, c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0), c.is_free,
			(SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count,
			%s
		FROM contests c
		WHERE c.starts_at >= $%d AND c.starts_at <= $%d
		  AND c.status IN ('scheduled', 'registration_open', 'running')`,
		userRegisteredClause, dateStartIdx, dateStartIdx+1)

	argIdx := dateStartIdx + 2

	if assetClass != "" && validMarketTypes[assetClass] {
		query += fmt.Sprintf(" AND c.asset_class = $%d", argIdx)
		args = append(args, assetClass)
		argIdx++
	}

	if durationType != "" {
		normalizedDuration := mapDurationType(durationType)
		if normalizedDuration != "" {
			query += fmt.Sprintf(" AND c.duration_type = $%d", argIdx)
			args = append(args, normalizedDuration)
			argIdx++
		}
	}

	query += " ORDER BY c.starts_at ASC"

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to query calendar tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	// Collect all tournaments
	type rawTournament struct {
		CalendarTournament
		startsAt time.Time
	}
	var allTournaments []rawTournament

	for rows.Next() {
		var ct CalendarTournament
		var entryFeeCents int
		var startsAt, endsAt time.Time
		var maxParticipants sql.NullInt32
		var commissionRate float64
		var platformFeeBps int
		var participantCount int
		var userRegistered bool
		var isFree bool

		if err := rows.Scan(
			&ct.ID, &ct.Name, &ct.MarketType, &ct.DurationType, &ct.DurationMinutes,
			&entryFeeCents, &startsAt, &endsAt,
			&ct.Status, &maxParticipants, &commissionRate, &platformFeeBps, &isFree,
			&participantCount, &userRegistered,
		); err != nil {
			a.log().Error("Failed to scan calendar tournament", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		ct.EntryFeeCents = entryFeeCents
		ct.IsFree = isFree
		ct.StartTime = newDualTime(startsAt)
		ct.EndTime = newDualTime(endsAt)
		ct.Participants = participantCount
		ct.UserRegistered = userRegistered
		ct.IsActive = ct.Status == "running"

		if maxParticipants.Valid {
			maxP := int(maxParticipants.Int32)
			ct.MaxParticipants = &maxP
		}

		// Calculate prize pool
		if !isFree && entryFeeCents > 0 && participantCount > 0 {
			feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
			grossCents := int64(entryFeeCents) * int64(participantCount)
			ct.PrizePoolCents = (grossCents * int64(10000-feeBps)) / 10000
		}

		allTournaments = append(allTournaments, rawTournament{
			CalendarTournament: ct,
			startsAt:           startsAt,
		})
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate calendar tournaments", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Build day-by-day structure
	dayMap := make(map[string]*CalendarDay)
	var dayOrder []string

	// Pre-populate days in the range
	for d := fromDate; d.Before(toDate.Add(time.Second)); d = d.Add(24 * time.Hour) {
		dateKey := d.Format("2006-01-02")
		irst := toIRST(d)
		jy, jm, jd := gregorianToJalali(irst.Year(), int(irst.Month()), irst.Day())

		dayMap[dateKey] = &CalendarDay{
			DateUTC:       dateKey,
			JalaliDate:    fmt.Sprintf("%04d/%02d/%02d", jy, jm, jd),
			JalaliWeekday: jalaliWeekday(d),
			Weekday:       d.Weekday().String(),
			IsWeekend:     isWeekendIRST(d),
			CryptoOnly:    isWeekendIRST(d), // Weekends are crypto-only
			Groups:        CalendarDayGroups{},
		}
		dayOrder = append(dayOrder, dateKey)
	}

	// Assign tournaments to days
	totalTournaments := 0
	for _, rt := range allTournaments {
		dateKey := rt.startsAt.UTC().Format("2006-01-02")
		day, ok := dayMap[dateKey]
		if !ok {
			continue
		}

		switch rt.DurationType {
		case "rush_30min":
			day.Groups.Rush30min = append(day.Groups.Rush30min, rt.CalendarTournament)
		case "hourly":
			day.Groups.Hourly = append(day.Groups.Hourly, rt.CalendarTournament)
		case "four_hour":
			day.Groups.FourHour = append(day.Groups.FourHour, rt.CalendarTournament)
		case "daily":
			day.Groups.Daily = append(day.Groups.Daily, rt.CalendarTournament)
		case "weekly":
			day.Groups.Weekly = append(day.Groups.Weekly, rt.CalendarTournament)
		}

		day.Total++
		totalTournaments++
	}

	// Build ordered days list
	days := make([]CalendarDay, 0, len(dayOrder))
	for _, dateKey := range dayOrder {
		days = append(days, *dayMap[dateKey])
	}

	writeJSON(w, http.StatusOK, TournamentCalendarResponse{
		From:  newDualTimeDate(fromDate),
		To:    newDualTimeDate(toDate),
		Days:  days,
		Total: totalTournaments,
	})
}

// ====================
// Tournament Feed Publisher (used by join handler)
// ====================

// publishTournamentFeedUpdate publishes a tournament feed update via Redis pub/sub.
// This is called after a contest update to also notify the tournament feed subscribers.
func (a *App) publishTournamentFeedUpdate(contestID string, event string, currentParticipants int, prizePoolCents int64) {
	if a.redis == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			a.log().Error("Panic in publishTournamentFeedUpdate",
				zap.Any("panic", r),
				zap.String("contest_id", contestID),
				zap.String("event", event))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	payload := map[string]interface{}{
		"type":                 "tournament.prize_pool_changed",
		"contest_id":           contestID,
		"event":                event,
		"current_participants": currentParticipants,
		"prize_pool_cents":     prizePoolCents,
		"ts":                   time.Now().UnixMilli(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		a.log().Warn("Failed to marshal tournament feed payload", zap.Error(err))
		return
	}

	// Publish to per-tournament channel
	channel := "tournament_feed:" + contestID
	if err := a.redis.Client().Publish(ctx, channel, string(data)).Err(); err != nil {
		a.log().Warn("Failed to publish tournament feed update",
			zap.Error(err), zap.String("contest_id", contestID))
	}

	// Publish to global feed channel
	if err := a.redis.Client().Publish(ctx, "tournament_feed:global", string(data)).Err(); err != nil {
		a.log().Warn("Failed to publish tournament feed global update",
			zap.Error(err), zap.String("contest_id", contestID))
	}
}

// Suppress unused import warnings at compile time
var (
	_ = math.Floor
	_ = redis.Nil
)
