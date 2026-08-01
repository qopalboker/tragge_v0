package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// =============================================================================
// Calendar Entry Types
// =============================================================================

// CalendarEntryRequest represents a request to create or update a calendar entry.
type CalendarEntryRequest struct {
	TemplateID                  string  `json:"template_id"`
	RecurrencePattern           string  `json:"recurrence_pattern"`
	CronExpression              *string `json:"cron_expression,omitempty"`
	StartDate                   string  `json:"start_date"`
	EndDate                     *string `json:"end_date,omitempty"`
	Timezone                    string  `json:"timezone"`
	RegistrationLeadTimeMinutes int     `json:"registration_lead_time_minutes"`
	Enabled                     *bool   `json:"enabled,omitempty"`
}

// CalendarEntryResponse represents a calendar entry in API responses.
type CalendarEntryResponse struct {
	ID                          string     `json:"id"`
	TemplateID                  string     `json:"template_id"`
	TemplateName                string     `json:"template_name,omitempty"`
	RecurrencePattern           string     `json:"recurrence_pattern"`
	CronExpression              *string    `json:"cron_expression,omitempty"`
	StartDate                   time.Time  `json:"start_date"`
	EndDate                     *time.Time `json:"end_date,omitempty"`
	Timezone                    string     `json:"timezone"`
	RegistrationLeadTimeMinutes int        `json:"registration_lead_time_minutes"`
	Enabled                     bool       `json:"enabled"`
	Status                      string     `json:"status"`
	LastRunAt                   *time.Time `json:"last_run_at,omitempty"`
	NextRunAt                   *time.Time `json:"next_run_at,omitempty"`
	CreatedBy                   string     `json:"created_by"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
}

// CalendarEntryDetailResponse includes calendar entry with its contest history.
type CalendarEntryDetailResponse struct {
	CalendarEntryResponse
	ContestHistory []CalendarContestHistoryItem `json:"contest_history"`
}

// CalendarContestHistoryItem represents a contest created from a calendar entry.
type CalendarContestHistoryItem struct {
	ContestID    string    `json:"contest_id"`
	ContestName  string    `json:"contest_name"`
	Status       string    `json:"status"`
	ScheduledFor time.Time `json:"scheduled_for"`
	CreatedAt    time.Time `json:"created_at"`
}

// CalendarEntryListResponse represents a paginated list of calendar entries.
type CalendarEntryListResponse struct {
	Entries []CalendarEntryResponse `json:"entries"`
	Total   int                     `json:"total"`
	Page    int                     `json:"page"`
	PerPage int                     `json:"per_page"`
}

// CalendarPreviewResponse represents preview of upcoming contest creation times.
type CalendarPreviewResponse struct {
	UpcomingTimes []time.Time `json:"upcoming_times"`
	Timezone      string      `json:"timezone"`
	Pattern       string      `json:"pattern"`
}

// =============================================================================
// Valid recurrence patterns
// =============================================================================

var validRecurrencePatterns = map[string]bool{
	"daily":       true,
	"weekly":      true,
	"biweekly":    true,
	"monthly":     true,
	"custom_cron": true,
}

// =============================================================================
// Calendar Handlers
// =============================================================================

// handleCreateCalendarEntry creates a new calendar entry.
// POST /api/admin/calendar
func (a *App) handleCreateCalendarEntry(w http.ResponseWriter, r *http.Request) {
	var req CalendarEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate input
	v := validation.New()

	if req.TemplateID == "" {
		v.AddError("template_id", "required", "template_id is required")
	}

	if !validRecurrencePatterns[req.RecurrencePattern] {
		v.AddError("recurrence_pattern", "invalid", "recurrence_pattern must be one of: daily, weekly, biweekly, monthly, custom_cron")
	}

	if req.RecurrencePattern == "custom_cron" && (req.CronExpression == nil || *req.CronExpression == "") {
		v.AddError("cron_expression", "required", "cron_expression is required when recurrence_pattern is custom_cron")
	}

	if req.CronExpression != nil && *req.CronExpression != "" {
		// Validate cron expression
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*req.CronExpression); err != nil {
			v.AddError("cron_expression", "invalid", fmt.Sprintf("invalid cron expression: %v", err))
		}
	}

	if req.StartDate == "" {
		v.AddError("start_date", "required", "start_date is required")
	}

	startDate, err := time.Parse(time.RFC3339, req.StartDate)
	if err != nil {
		v.AddError("start_date", "invalid", "start_date must be in RFC3339 format")
	}

	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		ed, err := time.Parse(time.RFC3339, *req.EndDate)
		if err != nil {
			v.AddError("end_date", "invalid", "end_date must be in RFC3339 format")
		} else {
			if !ed.After(startDate) {
				v.AddError("end_date", "invalid", "end_date must be after start_date")
			}
			endDate = &ed
		}
	}

	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	// Validate timezone
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		v.AddError("timezone", "invalid", "invalid IANA timezone")
	}

	if req.RegistrationLeadTimeMinutes < 0 {
		v.AddError("registration_lead_time_minutes", "invalid", "registration_lead_time_minutes must be >= 0")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Verify template exists
	var templateExists bool
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`,
			req.TemplateID,
		).Scan(&templateExists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !templateExists {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Calculate initial next_run_at
	loc, _ := time.LoadLocation(req.Timezone)
	nextRun := calculateNextRun(startDate, req.RecurrencePattern, req.CronExpression, loc)

	// Default enabled to true if not specified
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Begin transaction
	var tx *sql.Tx
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
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

	// Insert calendar entry
	var entry CalendarEntryResponse
	err = tx.QueryRowContext(ctx,
		`INSERT INTO calendar_entries (
			template_id, recurrence_pattern, cron_expression, start_date, end_date,
			timezone, registration_lead_time_minutes, enabled, status, next_run_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10)
		RETURNING id, template_id, recurrence_pattern, cron_expression, start_date, end_date,
			timezone, registration_lead_time_minutes, enabled, status, last_run_at, next_run_at,
			created_by, created_at, updated_at`,
		req.TemplateID, req.RecurrencePattern, req.CronExpression, startDate, endDate,
		req.Timezone, req.RegistrationLeadTimeMinutes, enabled, nextRun, actorUserID,
	).Scan(
		&entry.ID, &entry.TemplateID, &entry.RecurrencePattern, &entry.CronExpression,
		&entry.StartDate, &entry.EndDate, &entry.Timezone, &entry.RegistrationLeadTimeMinutes,
		&entry.Enabled, &entry.Status, &entry.LastRunAt, &entry.NextRunAt,
		&entry.CreatedBy, &entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		a.log().Error("Failed to insert calendar entry", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "calendar_entry.created", "calendar_entry", entry.ID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.log().Info("Calendar entry created",
		zap.String("entry_id", entry.ID),
		zap.String("template_id", entry.TemplateID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusCreated, entry)
}

// handleListCalendarEntries lists calendar entries with pagination and filtering.
// GET /api/admin/calendar
func (a *App) handleListCalendarEntries(w http.ResponseWriter, r *http.Request) {
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

	// Parse status filter
	statusFilter := r.URL.Query().Get("status")
	validStatuses := map[string]bool{"active": true, "paused": true, "ended": true}
	if statusFilter != "" && !validStatuses[statusFilter] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidStatus})
		return
	}

	// Build query
	var args []interface{}
	argIdx := 1

	baseQuery := `
		FROM calendar_entries ce
		LEFT JOIN tournament_templates tt ON ce.template_id = tt.id
	`
	whereClause := " WHERE 1=1"

	if statusFilter != "" {
		whereClause += fmt.Sprintf(" AND ce.status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

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
		a.log().Error("Failed to count calendar entries", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Fetch entries
	selectQuery := `
		SELECT ce.id, ce.template_id, tt.name, ce.recurrence_pattern, ce.cron_expression,
			ce.start_date, ce.end_date, ce.timezone, ce.registration_lead_time_minutes,
			ce.enabled, ce.status, ce.last_run_at, ce.next_run_at, ce.created_by,
			ce.created_at, ce.updated_at
	` + baseQuery + whereClause + fmt.Sprintf(` ORDER BY ce.created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
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
		a.log().Error("Failed to query calendar entries", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := rowsResult.(*sql.Rows)
	defer rows.Close()

	entries := []CalendarEntryResponse{}
	for rows.Next() {
		var entry CalendarEntryResponse
		var templateName sql.NullString
		err := rows.Scan(
			&entry.ID, &entry.TemplateID, &templateName, &entry.RecurrencePattern,
			&entry.CronExpression, &entry.StartDate, &entry.EndDate, &entry.Timezone,
			&entry.RegistrationLeadTimeMinutes, &entry.Enabled, &entry.Status,
			&entry.LastRunAt, &entry.NextRunAt, &entry.CreatedBy, &entry.CreatedAt, &entry.UpdatedAt,
		)
		if err != nil {
			a.log().Error("Failed to scan calendar entry", zap.Error(err))
			continue
		}
		if templateName.Valid {
			entry.TemplateName = templateName.String
		}
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, CalendarEntryListResponse{
		Entries: entries,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

// handleGetCalendarEntry gets a single calendar entry with its contest history.
// GET /api/admin/calendar/:id
func (a *App) handleGetCalendarEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	if entryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CalendarIDRequired})
		return
	}

	ctx := r.Context()

	// Fetch calendar entry
	var entry CalendarEntryDetailResponse
	var templateName sql.NullString
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT ce.id, ce.template_id, tt.name, ce.recurrence_pattern, ce.cron_expression,
				ce.start_date, ce.end_date, ce.timezone, ce.registration_lead_time_minutes,
				ce.enabled, ce.status, ce.last_run_at, ce.next_run_at, ce.created_by,
				ce.created_at, ce.updated_at
			FROM calendar_entries ce
			LEFT JOIN tournament_templates tt ON ce.template_id = tt.id
			WHERE ce.id = $1
		`, entryID).Scan(
			&entry.ID, &entry.TemplateID, &templateName, &entry.RecurrencePattern,
			&entry.CronExpression, &entry.StartDate, &entry.EndDate, &entry.Timezone,
			&entry.RegistrationLeadTimeMinutes, &entry.Enabled, &entry.Status,
			&entry.LastRunAt, &entry.NextRunAt, &entry.CreatedBy, &entry.CreatedAt, &entry.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.CalendarNotFound})
			return
		}
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to get calendar entry", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if templateName.Valid {
		entry.TemplateName = templateName.String
	}

	// Fetch last 10 contests created from this entry
	historyResult, historyErr := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT cch.contest_id, c.name, c.status, cch.scheduled_for, cch.created_at
				FROM calendar_contest_history cch
				JOIN contests c ON cch.contest_id = c.id
				WHERE cch.calendar_entry_id = $1
				ORDER BY cch.created_at DESC
				LIMIT 10
			`, entryID)
		},
	)
	if historyErr != nil {
		a.log().Error("Failed to fetch contest history", zap.Error(historyErr))
		// Continue without history
		entry.ContestHistory = []CalendarContestHistoryItem{}
	} else {
		rows := historyResult.(*sql.Rows)
		defer rows.Close()
		entry.ContestHistory = []CalendarContestHistoryItem{}
		for rows.Next() {
			var item CalendarContestHistoryItem
			if err := rows.Scan(&item.ContestID, &item.ContestName, &item.Status, &item.ScheduledFor, &item.CreatedAt); err != nil {
				a.log().Error("Failed to scan contest history item", zap.Error(err))
				continue
			}
			entry.ContestHistory = append(entry.ContestHistory, item)
		}
	}

	writeJSON(w, http.StatusOK, entry)
}

// handleUpdateCalendarEntry updates a calendar entry.
// PUT /api/admin/calendar/:id
func (a *App) handleUpdateCalendarEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	if entryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CalendarIDRequired})
		return
	}

	var req CalendarEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Check if entry exists
	var exists bool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM calendar_entries WHERE id = $1)`,
			entryID,
		).Scan(&exists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to check entry existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.CalendarNotFound})
		return
	}

	// Build update query dynamically
	var setClauses []string
	var args []interface{}
	argIdx := 1

	if req.TemplateID != "" {
		// Verify template exists
		var templateExists bool
		err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM tournament_templates WHERE id = $1)`,
				req.TemplateID,
			).Scan(&templateExists)
		})
		if err != nil {
			if a.isCircuitError(w, err) {
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateNotFound})
			return
		}
		if !templateExists {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TemplateNotFound})
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("template_id = $%d", argIdx))
		args = append(args, req.TemplateID)
		argIdx++
	}

	if req.RecurrencePattern != "" {
		if !validRecurrencePatterns[req.RecurrencePattern] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidRecurrence})
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("recurrence_pattern = $%d", argIdx))
		args = append(args, req.RecurrencePattern)
		argIdx++
	}

	if req.CronExpression != nil {
		if *req.CronExpression != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(*req.CronExpression); err != nil {
				a.log().Warn("invalid cron expression", zap.Error(err))
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidRecurrence})
				return
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("cron_expression = $%d", argIdx))
		args = append(args, req.CronExpression)
		argIdx++
	}

	if req.StartDate != "" {
		startDate, err := time.Parse(time.RFC3339, req.StartDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidDateFormat})
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("start_date = $%d", argIdx))
		args = append(args, startDate)
		argIdx++
	}

	if req.EndDate != nil {
		if *req.EndDate == "" {
			setClauses = append(setClauses, fmt.Sprintf("end_date = $%d", argIdx))
			args = append(args, nil)
			argIdx++
		} else {
			endDate, err := time.Parse(time.RFC3339, *req.EndDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidDateFormat})
				return
			}
			setClauses = append(setClauses, fmt.Sprintf("end_date = $%d", argIdx))
			args = append(args, endDate)
			argIdx++
		}
	}

	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidTimezone})
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("timezone = $%d", argIdx))
		args = append(args, req.Timezone)
		argIdx++
	}

	if req.RegistrationLeadTimeMinutes > 0 {
		setClauses = append(setClauses, fmt.Sprintf("registration_lead_time_minutes = $%d", argIdx))
		args = append(args, req.RegistrationLeadTimeMinutes)
		argIdx++
	}

	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *req.Enabled)
		argIdx++

		// Update status based on enabled
		if !*req.Enabled {
			setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, "paused")
			argIdx++
		} else {
			setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, "active")
			argIdx++
		}
	}

	if len(setClauses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	// Add entry ID to args
	args = append(args, entryID)

	// Begin transaction
	var tx *sql.Tx
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
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

	// Execute update
	updateQuery := fmt.Sprintf(`
		UPDATE calendar_entries
		SET %s, updated_at = NOW()
		WHERE id = $%d
	`, strings.Join(setClauses, ", "), argIdx)

	result, err := tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		a.log().Error("Failed to update calendar entry", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.CalendarNotFound})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "calendar_entry.updated", "calendar_entry", entryID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Fetch updated entry
	var entry CalendarEntryResponse
	var templateName sql.NullString
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT ce.id, ce.template_id, tt.name, ce.recurrence_pattern, ce.cron_expression,
				ce.start_date, ce.end_date, ce.timezone, ce.registration_lead_time_minutes,
				ce.enabled, ce.status, ce.last_run_at, ce.next_run_at, ce.created_by,
				ce.created_at, ce.updated_at
			FROM calendar_entries ce
			LEFT JOIN tournament_templates tt ON ce.template_id = tt.id
			WHERE ce.id = $1
		`, entryID).Scan(
			&entry.ID, &entry.TemplateID, &templateName, &entry.RecurrencePattern,
			&entry.CronExpression, &entry.StartDate, &entry.EndDate, &entry.Timezone,
			&entry.RegistrationLeadTimeMinutes, &entry.Enabled, &entry.Status,
			&entry.LastRunAt, &entry.NextRunAt, &entry.CreatedBy, &entry.CreatedAt, &entry.UpdatedAt,
		)
	})
	if err != nil {
		a.log().Error("Failed to fetch updated entry", zap.Error(err))
		writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.CalendarUpdated})
		return
	}
	if templateName.Valid {
		entry.TemplateName = templateName.String
	}

	a.log().Info("Calendar entry updated",
		zap.String("entry_id", entryID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, entry)
}

// handleDeleteCalendarEntry soft-deletes a calendar entry (sets enabled=false).
// DELETE /api/admin/calendar/:id
func (a *App) handleDeleteCalendarEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	if entryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CalendarIDRequired})
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

	// Soft delete: set enabled=false and status='ended'
	result, err := tx.ExecContext(ctx, `
		UPDATE calendar_entries
		SET enabled = FALSE, status = 'ended', updated_at = NOW()
		WHERE id = $1
	`, entryID)
	if err != nil {
		a.log().Error("Failed to delete calendar entry", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.CalendarNotFound})
		return
	}

	// Write audit log
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "calendar_entry.deleted", "calendar_entry", entryID, []byte("{}"),
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.log().Info("Calendar entry deleted (soft)",
		zap.String("entry_id", entryID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.CalendarDeleted})
}

// handlePreviewCalendarEntry previews the next 10 contest creation times.
// POST /api/admin/calendar/:id/preview
func (a *App) handlePreviewCalendarEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	if entryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CalendarIDRequired})
		return
	}

	ctx := r.Context()

	// Fetch calendar entry
	var pattern string
	var cronExpr sql.NullString
	var startDate time.Time
	var endDate sql.NullTime
	var timezone string
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT recurrence_pattern, cron_expression, start_date, end_date, timezone
			FROM calendar_entries
			WHERE id = $1
		`, entryID).Scan(&pattern, &cronExpr, &startDate, &endDate, &timezone)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.CalendarNotFound})
			return
		}
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to get calendar entry", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	var cronExprPtr *string
	if cronExpr.Valid {
		cronExprPtr = &cronExpr.String
	}

	// Calculate next 10 occurrences
	upcomingTimes := calculateNextOccurrences(time.Now(), pattern, cronExprPtr, loc, endDate, 10)

	writeJSON(w, http.StatusOK, CalendarPreviewResponse{
		UpcomingTimes: upcomingTimes,
		Timezone:      timezone,
		Pattern:       pattern,
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// calculateNextRun calculates the next run time based on the recurrence pattern.
func calculateNextRun(fromTime time.Time, pattern string, cronExpr *string, loc *time.Location) *time.Time {
	now := time.Now()
	if fromTime.After(now) {
		// Schedule starts in the future
		return &fromTime
	}

	occurrences := calculateNextOccurrences(now, pattern, cronExpr, loc, sql.NullTime{}, 1)
	if len(occurrences) > 0 {
		return &occurrences[0]
	}
	return nil
}

// calculateNextOccurrences calculates the next N occurrences based on the pattern.
func calculateNextOccurrences(fromTime time.Time, pattern string, cronExpr *string, loc *time.Location, endDate sql.NullTime, count int) []time.Time {
	var occurrences []time.Time

	if pattern == "custom_cron" && cronExpr != nil && *cronExpr != "" {
		// Use cron parser
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(*cronExpr)
		if err != nil {
			return occurrences
		}

		current := fromTime
		for len(occurrences) < count {
			next := schedule.Next(current)
			if endDate.Valid && next.After(endDate.Time) {
				break
			}
			occurrences = append(occurrences, next.In(loc))
			current = next
		}
	} else {
		// Calculate based on pattern
		current := fromTime.In(loc)

		// Find next occurrence based on pattern
		for len(occurrences) < count {
			var next time.Time
			switch pattern {
			case "daily":
				// Next day at the same time
				next = current.AddDate(0, 0, 1)
				// Align to start of day for first calculation
				if len(occurrences) == 0 {
					next = time.Date(current.Year(), current.Month(), current.Day()+1, 9, 0, 0, 0, loc) // Default 9 AM
				}
			case "weekly":
				// Next week at the same time
				next = current.AddDate(0, 0, 7)
				if len(occurrences) == 0 {
					// Find next Monday at 9 AM
					daysUntilMonday := (8 - int(current.Weekday())) % 7
					if daysUntilMonday == 0 {
						daysUntilMonday = 7
					}
					next = time.Date(current.Year(), current.Month(), current.Day()+daysUntilMonday, 9, 0, 0, 0, loc)
				}
			case "biweekly":
				// Every two weeks
				next = current.AddDate(0, 0, 14)
				if len(occurrences) == 0 {
					daysUntilMonday := (8 - int(current.Weekday())) % 7
					if daysUntilMonday == 0 {
						daysUntilMonday = 7
					}
					next = time.Date(current.Year(), current.Month(), current.Day()+daysUntilMonday, 9, 0, 0, 0, loc)
				}
			case "monthly":
				// Same day next month
				next = current.AddDate(0, 1, 0)
				if len(occurrences) == 0 {
					// First of next month at 9 AM
					next = time.Date(current.Year(), current.Month()+1, 1, 9, 0, 0, 0, loc)
				}
			default:
				// Default to daily
				next = current.AddDate(0, 0, 1)
			}

			if endDate.Valid && next.After(endDate.Time) {
				break
			}

			occurrences = append(occurrences, next)
			current = next
		}
	}

	return occurrences
}
