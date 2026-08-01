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
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// =============================================================================
// Schedule Types
// =============================================================================

// ScheduleResponse represents a tournament schedule in API responses.
type ScheduleResponse struct {
	ID              string    `json:"id"`
	TemplateID      string    `json:"template_id"`
	TemplateName    string    `json:"template_name,omitempty"`
	CronExpression  string    `json:"cron_expression"`
	StartTimeUTC    *string   `json:"start_time_utc,omitempty"`
	ActiveDays      []int     `json:"active_days"`
	WeekendBehavior string    `json:"weekend_behavior"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateScheduleRequest represents a request to create a tournament schedule.
type CreateScheduleRequest struct {
	TemplateID      string  `json:"template_id"`
	CronExpression  string  `json:"cron_expression"`
	StartTimeUTC    *string `json:"start_time_utc,omitempty"`
	ActiveDays      []int   `json:"active_days,omitempty"`
	WeekendBehavior string  `json:"weekend_behavior"`
}

// UpdateScheduleRequest represents a request to update a tournament schedule.
type UpdateScheduleRequest struct {
	CronExpression  *string `json:"cron_expression,omitempty"`
	StartTimeUTC    *string `json:"start_time_utc,omitempty"`
	ActiveDays      []int   `json:"active_days,omitempty"`
	WeekendBehavior *string `json:"weekend_behavior,omitempty"`
	// SetActiveDays distinguishes between "not provided" and "set to empty"
	// Use active_days: [] to clear active_days, omit to leave unchanged
	setActiveDays bool
}

// UnmarshalJSON custom unmarshaler to detect if active_days was explicitly set.
func (u *UpdateScheduleRequest) UnmarshalJSON(data []byte) error {
	// Use a map to detect which keys are present
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := raw["cron_expression"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			u.CronExpression = &s
		}
	}
	if v, ok := raw["start_time_utc"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			u.StartTimeUTC = &s
		}
	}
	if v, ok := raw["active_days"]; ok {
		u.setActiveDays = true
		if err := json.Unmarshal(v, &u.ActiveDays); err != nil {
			u.ActiveDays = nil
		}
	}
	if v, ok := raw["weekend_behavior"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			u.WeekendBehavior = &s
		}
	}

	return nil
}

// =============================================================================
// Validation helpers
// =============================================================================

var validWeekendBehaviors = map[string]bool{
	"crypto_only": true,
	"skip":        true,
	"normal":      true,
}

func validateCronExpression(v *validation.Validator, expr string) {
	if strings.TrimSpace(expr) == "" {
		v.AddError("cron_expression", "required", "cron_expression is required")
		return
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(expr); err != nil {
		v.AddError("cron_expression", "invalid", fmt.Sprintf("invalid cron expression: %v", err))
	}
}

func validateActiveDays(v *validation.Validator, days []int) {
	for i, d := range days {
		if d < 0 || d > 6 {
			v.AddError(fmt.Sprintf("active_days[%d]", i), "invalid", "active_days values must be between 0 (Saturday) and 6 (Friday)")
			return
		}
	}
}

func validateStartTimeUTC(v *validation.Validator, t *string) {
	if t == nil || *t == "" {
		return
	}
	// Accept HH:MM or HH:MM:SS
	_, err := time.Parse("15:04:05", *t)
	if err != nil {
		_, err = time.Parse("15:04", *t)
		if err != nil {
			v.AddError("start_time_utc", "invalid", "start_time_utc must be in HH:MM or HH:MM:SS format")
		}
	}
}

// =============================================================================
// Schedule scan helpers
// =============================================================================

// intArray is a helper for scanning PostgreSQL INT[] arrays.
type intArray []int

func (a *intArray) Scan(src interface{}) error {
	if src == nil {
		*a = nil
		return nil
	}

	switch v := src.(type) {
	case []byte:
		return a.scanString(string(v))
	case string:
		return a.scanString(v)
	default:
		return fmt.Errorf("intArray: cannot convert %T to []int", src)
	}
}

func (a *intArray) scanString(s string) error {
	if s == "{}" || s == "" {
		*a = []int{}
		return nil
	}

	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		*a = []int{}
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "NULL" || p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("intArray: cannot parse %q as int", p)
		}
		result = append(result, n)
	}
	*a = result
	return nil
}

// intArrayToPostgres converts a Go int slice to a PostgreSQL array literal string.
func intArrayToPostgres(ints []int) string {
	if len(ints) == 0 {
		return "{}"
	}
	parts := make([]string, len(ints))
	for i, v := range ints {
		parts[i] = strconv.Itoa(v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func scanScheduleRow(scanner interface {
	Scan(dest ...interface{}) error
}) (ScheduleResponse, error) {
	var s ScheduleResponse
	var startTimeUTC sql.NullString
	var activeDays intArray
	err := scanner.Scan(
		&s.ID, &s.TemplateID, &s.CronExpression, &startTimeUTC,
		&activeDays, &s.WeekendBehavior, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return s, err
	}
	if startTimeUTC.Valid {
		s.StartTimeUTC = &startTimeUTC.String
	}
	if activeDays == nil {
		s.ActiveDays = []int{}
	} else {
		s.ActiveDays = []int(activeDays)
	}
	return s, nil
}

func scanScheduleRowWithTemplateName(scanner interface {
	Scan(dest ...interface{}) error
}) (ScheduleResponse, error) {
	var s ScheduleResponse
	var startTimeUTC sql.NullString
	var activeDays intArray
	var templateName sql.NullString
	err := scanner.Scan(
		&s.ID, &s.TemplateID, &templateName, &s.CronExpression, &startTimeUTC,
		&activeDays, &s.WeekendBehavior, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return s, err
	}
	if startTimeUTC.Valid {
		s.StartTimeUTC = &startTimeUTC.String
	}
	if templateName.Valid {
		s.TemplateName = templateName.String
	}
	if activeDays == nil {
		s.ActiveDays = []int{}
	} else {
		s.ActiveDays = []int(activeDays)
	}
	return s, nil
}

// =============================================================================
// Schedule Handlers
// =============================================================================

// handleListSchedules lists tournament schedules with filtering and pagination.
// GET /api/admin/schedules
func (a *App) handleListSchedules(w http.ResponseWriter, r *http.Request) {
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

	// Filter: template_id
	if tid := r.URL.Query().Get("template_id"); tid != "" {
		whereClause += fmt.Sprintf(" AND s.template_id = $%d", argIdx)
		args = append(args, tid)
		argIdx++
	}

	// Filter: is_active
	if ia := r.URL.Query().Get("is_active"); ia != "" {
		isActive, err := strconv.ParseBool(ia)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.IsActiveInvalid})
			return
		}
		whereClause += fmt.Sprintf(" AND s.is_active = $%d", argIdx)
		args = append(args, isActive)
		argIdx++
	}

	baseQuery := `
		FROM tournament_schedules s
		LEFT JOIN tournament_templates t ON s.template_id = t.id
	`

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
		a.log().Error("Failed to count schedules", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Fetch schedules
	selectQuery := `
		SELECT s.id, s.template_id, t.name, s.cron_expression, s.start_time_utc,
			s.active_days, s.weekend_behavior, s.is_active, s.created_at, s.updated_at
	` + baseQuery + whereClause +
		fmt.Sprintf(" ORDER BY s.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
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
		a.log().Error("Failed to query schedules", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := rowsResult.(*sql.Rows)
	defer rows.Close()

	schedules := []ScheduleResponse{}
	for rows.Next() {
		s, err := scanScheduleRowWithTemplateName(rows)
		if err != nil {
			a.log().Error("Failed to scan schedule", zap.Error(err))
			continue
		}
		schedules = append(schedules, s)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate schedules", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schedules": schedules,
		"total":     total,
		"page":      page,
		"per_page":  perPage,
	})
}

// handleCreateSchedule creates a new tournament schedule.
// POST /api/admin/schedules
func (a *App) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate input
	v := validation.New()

	if req.TemplateID == "" {
		v.AddError("template_id", "required", "template_id is required")
	}

	validateCronExpression(v, req.CronExpression)
	validateActiveDays(v, req.ActiveDays)
	validateStartTimeUTC(v, req.StartTimeUTC)

	if !validWeekendBehaviors[req.WeekendBehavior] {
		v.AddError("weekend_behavior", "invalid", "weekend_behavior must be one of: crypto_only, skip, normal")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Verify template exists
	var templateExists bool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
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

	// Insert schedule
	var s ScheduleResponse
	var startTimeUTC sql.NullString
	var activeDays intArray

	activeDaysLiteral := intArrayToPostgres(req.ActiveDays)

	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, `
			INSERT INTO tournament_schedules (
				template_id, cron_expression, start_time_utc, active_days,
				weekend_behavior, is_active
			) VALUES ($1, $2, $3, $4::int[], $5::weekend_behavior, TRUE)
			RETURNING id, template_id, cron_expression, start_time_utc,
				active_days, weekend_behavior, is_active, created_at, updated_at`,
			req.TemplateID, req.CronExpression, req.StartTimeUTC, activeDaysLiteral,
			req.WeekendBehavior,
		).Scan(
			&s.ID, &s.TemplateID, &s.CronExpression, &startTimeUTC,
			&activeDays, &s.WeekendBehavior, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
		)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to insert schedule", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if startTimeUTC.Valid {
		s.StartTimeUTC = &startTimeUTC.String
	}
	if activeDays == nil {
		s.ActiveDays = []int{}
	} else {
		s.ActiveDays = []int(activeDays)
	}

	// Audit log
	a.logAuditEvent(ctx, actorUserID, "schedule.created", "tournament_schedule", s.ID,
		map[string]interface{}{"template_id": req.TemplateID, "cron_expression": req.CronExpression})

	a.log().Info("Tournament schedule created",
		zap.String("schedule_id", s.ID),
		zap.String("template_id", req.TemplateID),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusCreated, s)
}

// handleUpdateSchedule updates a tournament schedule.
// PUT /api/admin/schedules/{id}
func (a *App) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ScheduleIDRequired})
		return
	}

	var req UpdateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate provided fields
	v := validation.New()
	if req.CronExpression != nil {
		validateCronExpression(v, *req.CronExpression)
	}
	if req.setActiveDays {
		validateActiveDays(v, req.ActiveDays)
	}
	if req.StartTimeUTC != nil {
		validateStartTimeUTC(v, req.StartTimeUTC)
	}
	if req.WeekendBehavior != nil && !validWeekendBehaviors[*req.WeekendBehavior] {
		v.AddError("weekend_behavior", "invalid", "weekend_behavior must be one of: crypto_only, skip, normal")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Build dynamic UPDATE
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.CronExpression != nil {
		updates = append(updates, "cron_expression = $"+itoa(argIdx))
		args = append(args, *req.CronExpression)
		argIdx++
	}
	if req.StartTimeUTC != nil {
		updates = append(updates, "start_time_utc = $"+itoa(argIdx))
		args = append(args, *req.StartTimeUTC)
		argIdx++
	}
	if req.setActiveDays {
		updates = append(updates, "active_days = $"+itoa(argIdx)+"::int[]")
		args = append(args, intArrayToPostgres(req.ActiveDays))
		argIdx++
	}
	if req.WeekendBehavior != nil {
		updates = append(updates, "weekend_behavior = $"+itoa(argIdx)+"::weekend_behavior")
		args = append(args, *req.WeekendBehavior)
		argIdx++
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	query := "UPDATE tournament_schedules SET " + joinStrings(updates, ", ") +
		" WHERE id = $" + itoa(argIdx) +
		" RETURNING id, template_id, cron_expression, start_time_utc, active_days, weekend_behavior, is_active, created_at, updated_at"
	args = append(args, scheduleID)

	var s ScheduleResponse
	var startTimeUTC sql.NullString
	var activeDays intArray

	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, query, args...).Scan(
			&s.ID, &s.TemplateID, &s.CronExpression, &startTimeUTC,
			&activeDays, &s.WeekendBehavior, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
		)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ScheduleNotFound})
			return
		}
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to update schedule", zap.String("schedule_id", scheduleID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if startTimeUTC.Valid {
		s.StartTimeUTC = &startTimeUTC.String
	}
	if activeDays == nil {
		s.ActiveDays = []int{}
	} else {
		s.ActiveDays = []int(activeDays)
	}

	// Audit log
	a.logAuditEvent(ctx, actorUserID, "schedule.updated", "tournament_schedule", scheduleID, req)

	writeJSON(w, http.StatusOK, s)
}

// handleDeleteSchedule soft-deletes a schedule by setting is_active = false.
// DELETE /api/admin/schedules/{id}
func (a *App) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ScheduleIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var rowsAffected int64
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		result, execErr := a.pool.Primary().ExecContext(ctx,
			`UPDATE tournament_schedules SET is_active = false WHERE id = $1 AND is_active = true`,
			scheduleID,
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
		a.log().Error("Failed to deactivate schedule", zap.String("schedule_id", scheduleID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if rowsAffected == 0 {
		var exists bool
		_ = a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM tournament_schedules WHERE id = $1)`, scheduleID,
		).Scan(&exists)
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ScheduleNotFound})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ScheduleDeactivated})
		return
	}

	a.logAuditEvent(ctx, actorUserID, "schedule.deleted", "tournament_schedule", scheduleID,
		map[string]string{"action": "deactivated"})

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ScheduleDeactivated})
}

// handlePauseSchedule temporarily pauses an active schedule.
// POST /api/admin/schedules/{id}/pause
func (a *App) handlePauseSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ScheduleIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var rowsAffected int64
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		result, execErr := a.pool.Primary().ExecContext(ctx,
			`UPDATE tournament_schedules SET is_active = false WHERE id = $1 AND is_active = true`,
			scheduleID,
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
		a.log().Error("Failed to pause schedule", zap.String("schedule_id", scheduleID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if rowsAffected == 0 {
		var exists bool
		_ = a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM tournament_schedules WHERE id = $1)`, scheduleID,
		).Scan(&exists)
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ScheduleNotFound})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.ScheduleAlreadyPaused})
		return
	}

	a.logAuditEvent(ctx, actorUserID, "schedule.paused", "tournament_schedule", scheduleID, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.SchedulePaused})
}

// handleResumeSchedule resumes a paused schedule.
// POST /api/admin/schedules/{id}/resume
func (a *App) handleResumeSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ScheduleIDRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var rowsAffected int64
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		result, execErr := a.pool.Primary().ExecContext(ctx,
			`UPDATE tournament_schedules SET is_active = true WHERE id = $1 AND is_active = false`,
			scheduleID,
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
		a.log().Error("Failed to resume schedule", zap.String("schedule_id", scheduleID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if rowsAffected == 0 {
		var exists bool
		_ = a.pool.Replica().QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM tournament_schedules WHERE id = $1)`, scheduleID,
		).Scan(&exists)
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ScheduleNotFound})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.ScheduleAlreadyActive})
		return
	}

	a.logAuditEvent(ctx, actorUserID, "schedule.resumed", "tournament_schedule", scheduleID, nil)

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ScheduleResumed})
}
