package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	ticketpkg "github.com/Parsaeffatravesh/tragge/packages/ticket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---- Response types ----

type adminTicketListResponse struct {
	Tickets []adminTicketSummary `json:"tickets"`
	Total   int                  `json:"total"`
	HasMore bool                 `json:"has_more"`
}

type adminTicketSummary struct {
	ID            string           `json:"id"`
	Subject       string           `json:"subject"`
	Category      string           `json:"category"`
	Status        string           `json:"status"`
	Priority      string           `json:"priority"`
	User          adminTicketUser  `json:"user"`
	AssignedAdmin *adminTicketUser `json:"assigned_admin,omitempty"`
	MessageCount  int              `json:"message_count"`
	LastMessageAt *string          `json:"last_message_at,omitempty"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
}

type adminTicketUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type adminTicketDetailResponse struct {
	Ticket   adminTicketInfo  `json:"ticket"`
	Messages []adminTicketMsg `json:"messages"`
}

type adminTicketInfo struct {
	ID            string           `json:"id"`
	Subject       string           `json:"subject"`
	Category      string           `json:"category"`
	Status        string           `json:"status"`
	Priority      string           `json:"priority"`
	User          adminTicketUser  `json:"user"`
	AssignedAdmin *adminTicketUser `json:"assigned_admin,omitempty"`
	ClosedAt      *string          `json:"closed_at,omitempty"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
}

type adminTicketMsg struct {
	ID          string                  `json:"id"`
	Body        string                  `json:"body"`
	IsAdmin     bool                    `json:"is_admin"`
	SenderName  string                  `json:"sender_name"`
	Attachments []adminTicketAttachment `json:"attachments"`
	CreatedAt   string                  `json:"created_at"`
}

type adminTicketAttachment struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
}

type adminTicketStatsResponse struct {
	Total                  int     `json:"total"`
	Open                   int     `json:"open"`
	UserReplied            int     `json:"user_replied"`
	Answered               int     `json:"answered"`
	Closed                 int     `json:"closed"`
	Resolved               int     `json:"resolved"`
	AvgResponseTimeMinutes float64 `json:"avg_response_time_minutes"`
}

// ---- Handlers ----

// handleAdminListTickets lists all tickets with filters (GET /api/admin/tickets).
func (a *App) handleAdminListTickets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	statusFilter := r.URL.Query().Get("status")
	categoryFilter := r.URL.Query().Get("category")
	priorityFilter := r.URL.Query().Get("priority")
	assignedFilter := r.URL.Query().Get("assigned_to")
	searchQuery := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort")
	if sortBy != "created_at" {
		sortBy = "updated_at"
	}

	// Build query
	where := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if statusFilter != "" && ticketpkg.Statuses[statusFilter] {
		where = append(where, fmt.Sprintf("t.status = $%d::ticket_status", argIdx))
		args = append(args, statusFilter)
		argIdx++
	}
	if categoryFilter != "" && ticketpkg.Categories[categoryFilter] {
		where = append(where, fmt.Sprintf("t.category = $%d::ticket_category", argIdx))
		args = append(args, categoryFilter)
		argIdx++
	}
	if priorityFilter != "" && ticketpkg.Priorities[priorityFilter] {
		where = append(where, fmt.Sprintf("t.priority = $%d::ticket_priority", argIdx))
		args = append(args, priorityFilter)
		argIdx++
	}
	if assignedFilter != "" {
		where = append(where, fmt.Sprintf("t.assigned_admin_id = $%d", argIdx))
		args = append(args, assignedFilter)
		argIdx++
	}
	if searchQuery != "" {
		where = append(where, fmt.Sprintf("t.subject ILIKE $%d", argIdx))
		args = append(args, "%"+searchQuery+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM support_tickets t WHERE %s", whereClause)
	if err := a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		a.log().Error("Failed to count admin tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	query := fmt.Sprintf(`
		SELECT t.id, t.subject, t.category::text, t.status::text, t.priority::text,
		       t.created_at, t.updated_at,
		       u.id, COALESCE(u.email, ''), COALESCE(u.username, ''),
		       aa.id, COALESCE(aa.email, ''), COALESCE(aa.username, ''),
		       (SELECT COUNT(*) FROM ticket_messages WHERE ticket_id = t.id) as message_count,
		       (SELECT created_at FROM ticket_messages WHERE ticket_id = t.id ORDER BY created_at DESC LIMIT 1) as last_message_at
		FROM support_tickets t
		JOIN users u ON u.id = t.user_id
		LEFT JOIN users aa ON aa.id = t.assigned_admin_id
		WHERE %s
		ORDER BY t.%s DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to list admin tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer rows.Close()

	tickets := make([]adminTicketSummary, 0)
	for rows.Next() {
		var t adminTicketSummary
		var createdAt, updatedAt time.Time
		var adminID, adminEmail, adminUsername sql.NullString
		var lastMessageAt sql.NullTime

		if err := rows.Scan(
			&t.ID, &t.Subject, &t.Category, &t.Status, &t.Priority,
			&createdAt, &updatedAt,
			&t.User.ID, &t.User.Email, &t.User.Username,
			&adminID, &adminEmail, &adminUsername,
			&t.MessageCount, &lastMessageAt,
		); err != nil {
			a.log().Error("Failed to scan admin ticket", zap.Error(err))
			continue
		}

		t.CreatedAt = createdAt.Format(time.RFC3339)
		t.UpdatedAt = updatedAt.Format(time.RFC3339)

		if adminID.Valid {
			t.AssignedAdmin = &adminTicketUser{
				ID:       adminID.String,
				Email:    adminEmail.String,
				Username: adminUsername.String,
			}
		}
		if lastMessageAt.Valid {
			ts := lastMessageAt.Time.Format(time.RFC3339)
			t.LastMessageAt = &ts
		}

		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating admin tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, adminTicketListResponse{
		Tickets: tickets,
		Total:   total,
		HasMore: offset+limit < total,
	})
}

// handleAdminGetTicket returns ticket details with all messages (GET /api/admin/tickets/{id}).
func (a *App) handleAdminGetTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ticketID := chi.URLParam(r, "id")

	var info adminTicketInfo
	var createdAt, updatedAt time.Time
	var closedAt sql.NullTime
	var adminID, adminEmail, adminUsername sql.NullString

	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT t.id, t.subject, t.category::text, t.status::text, t.priority::text,
		        t.closed_at, t.created_at, t.updated_at,
		        u.id, COALESCE(u.email, ''), COALESCE(u.username, ''),
		        aa.id, COALESCE(aa.email, ''), COALESCE(aa.username, '')
		 FROM support_tickets t
		 JOIN users u ON u.id = t.user_id
		 LEFT JOIN users aa ON aa.id = t.assigned_admin_id
		 WHERE t.id = $1`,
		ticketID,
	).Scan(&info.ID, &info.Subject, &info.Category, &info.Status, &info.Priority,
		&closedAt, &createdAt, &updatedAt,
		&info.User.ID, &info.User.Email, &info.User.Username,
		&adminID, &adminEmail, &adminUsername)

	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TicketNotFound})
			return
		}
		a.log().Error("Failed to load admin ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	info.CreatedAt = createdAt.Format(time.RFC3339)
	info.UpdatedAt = updatedAt.Format(time.RFC3339)
	if closedAt.Valid {
		ts := closedAt.Time.Format(time.RFC3339)
		info.ClosedAt = &ts
	}
	if adminID.Valid {
		info.AssignedAdmin = &adminTicketUser{
			ID:       adminID.String,
			Email:    adminEmail.String,
			Username: adminUsername.String,
		}
	}

	// Load messages
	messages, err := a.loadAdminTicketMessages(ctx, ticketID)
	if err != nil {
		a.log().Error("Failed to load ticket messages", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, adminTicketDetailResponse{
		Ticket:   info,
		Messages: messages,
	})
}

// handleAdminSendMessage sends an admin reply to a ticket (POST /api/admin/tickets/{id}/messages).
func (a *App) handleAdminSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "id")

	// Check ticket exists and is not closed
	var ticketStatus string
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT status::text FROM support_tickets WHERE id = $1`,
		ticketID,
	).Scan(&ticketStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TicketNotFound})
			return
		}
		a.log().Error("Failed to check ticket status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if ticketStatus == "closed" || ticketStatus == "resolved" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TicketClosed})
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(35 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidForm})
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.MessageRequired})
		return
	}
	if utf8.RuneCountInString(body) > 5000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.MessageTooLong})
		return
	}

	messageID := uuid.New().String()

	// Handle optional file attachment
	var attachmentInfo *struct {
		fileName    string
		fileSize    int64
		contentType string
		storageKey  string
	}

	file, fileHeader, fileErr := r.FormFile("attachment")
	if fileErr == nil && a.kycStorage != nil {
		defer file.Close()

		detectedType, valErr := ticketpkg.ValidateFile(fileHeader)
		if valErr != nil {
			a.log().Warn("admin ticket attachment validation failed", zap.Error(valErr))
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidFileType})
			return
		}

		ext := ticketpkg.MimeToExt(detectedType)
		fileUUID := uuid.New().String()
		storageKey := fmt.Sprintf("tickets/%s/%s/%s.%s", ticketID, messageID, fileUUID, ext)

		if err := ticketpkg.ValidateS3Key(storageKey); err != nil {
			a.log().Error("Invalid ticket S3 key", zap.String("key", storageKey), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}

		bucket := a.config.S3TicketBucket
		_, uploadErr := a.kycStorage.Upload(ctx, bucket, storageKey, file, fileHeader.Size, detectedType)
		if uploadErr != nil {
			a.log().Error("Failed to upload ticket attachment", zap.Error(uploadErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.AttachmentUpload})
			return
		}

		attachmentInfo = &struct {
			fileName    string
			fileSize    int64
			contentType string
			storageKey  string
		}{
			fileName:    fileHeader.Filename,
			fileSize:    fileHeader.Size,
			contentType: detectedType,
			storageKey:  storageKey,
		}
	}

	// Insert message and update ticket in transaction
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
			// Cleanup orphan S3 file on transaction failure
			if attachmentInfo != nil {
				if delErr := a.kycStorage.Delete(context.Background(), a.config.S3TicketBucket, attachmentInfo.storageKey); delErr != nil {
					a.log().Warn("Failed to cleanup orphan ticket attachment",
						zap.String("key", attachmentInfo.storageKey), zap.Error(delErr))
				}
			}
		}
	}()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO ticket_messages (id, ticket_id, sender_id, is_admin, body)
		 VALUES ($1, $2, $3, true, $4)`,
		messageID, ticketID, adminID, body,
	)
	if err != nil {
		a.log().Error("Failed to insert admin ticket message", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if attachmentInfo != nil {
		attachmentID := uuid.New().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO ticket_attachments (id, message_id, file_name, file_size, content_type, storage_key)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			attachmentID, messageID, attachmentInfo.fileName, attachmentInfo.fileSize,
			attachmentInfo.contentType, attachmentInfo.storageKey,
		)
		if err != nil {
			a.log().Error("Failed to insert admin attachment", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Update ticket status to answered and auto-assign if not assigned
	_, err = tx.ExecContext(ctx,
		`UPDATE support_tickets
		 SET status = 'answered',
		     assigned_admin_id = COALESCE(assigned_admin_id, $2)
		 WHERE id = $1`,
		ticketID, adminID,
	)
	if err != nil {
		a.log().Error("Failed to update ticket after admin reply", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit admin message transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	committed = true

	// Audit log
	a.logAuditEvent(ctx, adminID, "ticket.reply", "support_ticket", ticketID, map[string]string{
		"message_id": messageID,
	})

	// Send in-app notification to ticket owner (async — don't block response)
	infra.SafeGo(a.log(), "ticket-reply-notification", func() {
		notifCtx, notifCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer notifCancel()

		// Get ticket owner user_id and subject
		var ownerUserID, ticketSubject string
		err := a.pool.Replica().QueryRowContext(notifCtx,
			`SELECT user_id, subject FROM support_tickets WHERE id = $1`,
			ticketID,
		).Scan(&ownerUserID, &ticketSubject)
		if err != nil {
			a.log().Warn("Failed to load ticket for notification",
				zap.Error(err), zap.String("ticket_id", ticketID))
			return
		}

		// Check user preferences
		enabled, _ := prefs.IsEnabled(notifCtx, a.pool.Replica(), ownerUserID, inapp.NotifTypeTicketReply, "in_app")
		if !enabled {
			return
		}

		if err := inapp.CreateTicketReplyNotification(notifCtx, a.pool.Primary(), ownerUserID, ticketID, ticketSubject); err != nil {
			a.log().Warn("Failed to create ticket reply notification",
				zap.Error(err), zap.String("user_id", ownerUserID), zap.String("ticket_id", ticketID))
		}
	})

	writeJSON(w, http.StatusCreated, map[string]string{"id": messageID, "status": "sent"})
}

// handleAdminUpdateStatus updates a ticket's status (PUT /api/admin/tickets/{id}/status).
func (a *App) handleAdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if !ticketpkg.Statuses[req.Status] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidStatus})
		return
	}

	var query string
	args := []interface{}{req.Status, ticketID}

	if req.Status == "closed" || req.Status == "resolved" {
		query = `UPDATE support_tickets SET status = $1::ticket_status, closed_at = NOW() WHERE id = $2`
	} else {
		// Reopening — clear closed_at
		query = `UPDATE support_tickets SET status = $1::ticket_status, closed_at = NULL WHERE id = $2`
	}

	result, err := a.pool.Primary().ExecContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to update ticket status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TicketNotFound})
		return
	}

	a.logAuditEvent(ctx, adminID, "ticket.status_change", "support_ticket", ticketID, map[string]string{
		"new_status": req.Status,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

// handleAdminAssignTicket assigns a ticket to an admin (PUT /api/admin/tickets/{id}/assign).
func (a *App) handleAdminAssignTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "id")

	var req struct {
		AdminID string `json:"admin_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if req.AdminID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.AdminIDRequired})
		return
	}

	// Verify admin exists and has admin role
	var hasAdminRole bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1 AND r.name IN ('admin', 'super_admin')
		)`,
		req.AdminID,
	).Scan(&hasAdminRole)
	if err != nil || !hasAdminRole {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.AdminNotFound})
		return
	}

	result, err := a.pool.Primary().ExecContext(ctx,
		`UPDATE support_tickets SET assigned_admin_id = $1 WHERE id = $2`,
		req.AdminID, ticketID,
	)
	if err != nil {
		a.log().Error("Failed to assign ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TicketNotFound})
		return
	}

	a.logAuditEvent(ctx, actorID, "ticket.assign", "support_ticket", ticketID, map[string]string{
		"assigned_to": req.AdminID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// handleAdminUpdatePriority updates a ticket's priority (PUT /api/admin/tickets/{id}/priority).
func (a *App) handleAdminUpdatePriority(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "id")

	var req struct {
		Priority string `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if !ticketpkg.Priorities[req.Priority] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidPriority})
		return
	}

	result, err := a.pool.Primary().ExecContext(ctx,
		`UPDATE support_tickets SET priority = $1::ticket_priority WHERE id = $2`,
		req.Priority, ticketID,
	)
	if err != nil {
		a.log().Error("Failed to update ticket priority", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TicketNotFound})
		return
	}

	a.logAuditEvent(ctx, adminID, "ticket.priority_change", "support_ticket", ticketID, map[string]string{
		"new_priority": req.Priority,
	})

	writeJSON(w, http.StatusOK, map[string]string{"priority": req.Priority})
}

// handleAdminTicketStats returns aggregated ticket statistics (GET /api/admin/tickets/stats).
func (a *App) handleAdminTicketStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var stats adminTicketStatsResponse
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'open'),
			COUNT(*) FILTER (WHERE status = 'user_replied'),
			COUNT(*) FILTER (WHERE status = 'answered'),
			COUNT(*) FILTER (WHERE status = 'closed'),
			COUNT(*) FILTER (WHERE status = 'resolved')
		FROM support_tickets
	`).Scan(&stats.Total, &stats.Open, &stats.UserReplied, &stats.Answered, &stats.Closed, &stats.Resolved)

	if err != nil {
		a.log().Error("Failed to get ticket stats", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Average response time: time between first user message and first admin response
	var avgMinutes sql.NullFloat64
	_ = a.pool.Replica().QueryRowContext(ctx, `
		SELECT AVG(EXTRACT(EPOCH FROM (admin_msg.created_at - user_msg.created_at)) / 60)
		FROM support_tickets st
		JOIN LATERAL (
			SELECT created_at FROM ticket_messages WHERE ticket_id = st.id AND is_admin = false ORDER BY created_at ASC LIMIT 1
		) user_msg ON true
		JOIN LATERAL (
			SELECT created_at FROM ticket_messages WHERE ticket_id = st.id AND is_admin = true ORDER BY created_at ASC LIMIT 1
		) admin_msg ON true
	`).Scan(&avgMinutes)

	if avgMinutes.Valid {
		stats.AvgResponseTimeMinutes = avgMinutes.Float64
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleAdminGetTicketAttachment downloads a ticket attachment (GET /api/admin/tickets/attachment/{attachmentId}).
func (a *App) handleAdminGetTicketAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	attachmentID := chi.URLParam(r, "attachmentId")

	if a.kycStorage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.FileStorageUnavailable})
		return
	}

	var storageKey, contentType, fileName string
	var fileSize int64
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT ta.storage_key, ta.content_type, ta.file_name, ta.file_size
		 FROM ticket_attachments ta
		 JOIN ticket_messages tm ON tm.id = ta.message_id
		 JOIN support_tickets st ON st.id = tm.ticket_id
		 WHERE ta.id = $1`,
		attachmentID,
	).Scan(&storageKey, &contentType, &fileName, &fileSize)

	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.AttachmentNotFound})
			return
		}
		a.log().Error("Failed to load attachment info", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	bucket := a.config.S3TicketBucket
	reader, _, _, downloadErr := a.kycStorage.Download(ctx, bucket, storageKey)
	if downloadErr != nil {
		a.log().Error("Failed to download attachment from S3", zap.String("key", storageKey), zap.Error(downloadErr))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.AttachmentDownload})
		return
	}
	defer reader.Close()

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))

	safeName := ticketpkg.SanitizeFileName(fileName)
	if strings.HasPrefix(contentType, "image/") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeName))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	}

	io.Copy(w, reader)
}

// ---- Helpers ----

// loadAdminTicketMessages loads messages with attachments for admin view.
func (a *App) loadAdminTicketMessages(ctx context.Context, ticketID string) ([]adminTicketMsg, error) {
	rows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT tm.id, tm.body, tm.is_admin, tm.created_at,
		        COALESCE(u.username, u.email, 'Unknown') as sender_name
		 FROM ticket_messages tm
		 JOIN users u ON u.id = tm.sender_id
		 WHERE tm.ticket_id = $1
		 ORDER BY tm.created_at ASC`,
		ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []adminTicketMsg
	var messageIDs []string
	messageMap := make(map[string]*adminTicketMsg)

	for rows.Next() {
		var m adminTicketMsg
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.Body, &m.IsAdmin, &createdAt, &m.SenderName); err != nil {
			return nil, err
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		m.Attachments = make([]adminTicketAttachment, 0)

		messages = append(messages, m)
		messageIDs = append(messageIDs, m.ID)
		messageMap[m.ID] = &messages[len(messages)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating messages: %w", err)
	}

	if len(messageIDs) > 0 {
		placeholders := make([]string, len(messageIDs))
		args := make([]interface{}, len(messageIDs))
		for i, id := range messageIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		attRows, err := a.pool.Replica().QueryContext(ctx,
			fmt.Sprintf(`SELECT id, message_id, file_name, file_size, content_type
			 FROM ticket_attachments
			 WHERE message_id IN (%s)`, strings.Join(placeholders, ",")),
			args...,
		)
		if err != nil {
			return nil, err
		}
		defer attRows.Close()

		for attRows.Next() {
			var att adminTicketAttachment
			var msgID string
			if err := attRows.Scan(&att.ID, &msgID, &att.FileName, &att.FileSize, &att.ContentType); err != nil {
				return nil, err
			}
			if msg, ok := messageMap[msgID]; ok {
				msg.Attachments = append(msg.Attachments, att)
			}
		}
		if err := attRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating attachments: %w", err)
		}
	}

	return messages, nil
}

// decodeJSON decodes a JSON request body.
func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
