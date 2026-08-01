package server

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Parsaeffatravesh/tragge/packages/audit"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	ticketpkg "github.com/Parsaeffatravesh/tragge/packages/ticket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---- Response types ----

type ticketListResponse struct {
	Tickets []ticketSummary `json:"tickets"`
	Total   int             `json:"total"`
	HasMore bool            `json:"has_more"`
}

type ticketSummary struct {
	ID                 string  `json:"id"`
	Subject            string  `json:"subject"`
	Category           string  `json:"category"`
	Status             string  `json:"status"`
	Priority           string  `json:"priority"`
	LastMessagePreview *string `json:"last_message_preview,omitempty"`
	LastMessageAt      *string `json:"last_message_at,omitempty"`
	LastMessageIsAdmin *bool   `json:"last_message_is_admin,omitempty"`
	Unread             bool    `json:"unread"`
	MessageCount       int     `json:"message_count"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type ticketDetailResponse struct {
	Ticket   ticketInfo      `json:"ticket"`
	Messages []ticketMessage `json:"messages"`
}

type ticketInfo struct {
	ID        string  `json:"id"`
	Subject   string  `json:"subject"`
	Category  string  `json:"category"`
	Status    string  `json:"status"`
	Priority  string  `json:"priority"`
	ClosedAt  *string `json:"closed_at,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type ticketMessage struct {
	ID          string             `json:"id"`
	Body        string             `json:"body"`
	IsAdmin     bool               `json:"is_admin"`
	SenderName  string             `json:"sender_name"`
	Attachments []ticketAttachment `json:"attachments"`
	CreatedAt   string             `json:"created_at"`
}

type ticketAttachment struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
}

// ---- Handlers ----

// handleCreateTicket creates a new support ticket (POST /api/user/me/tickets).
func (a *App) handleCreateTicket_Support(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	if a.kycStorage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.FileStorageUnavailable})
		return
	}

	// Parse multipart form (max 35MB to account for overhead)
	if err := r.ParseMultipartForm(35 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidForm})
		return
	}

	subject := strings.TrimSpace(r.FormValue("subject"))
	category := strings.TrimSpace(r.FormValue("category"))
	body := strings.TrimSpace(r.FormValue("body"))

	// Validate required fields
	if subject == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.SubjectRequired})
		return
	}
	if utf8.RuneCountInString(subject) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.SubjectTooLong})
		return
	}
	if category == "" {
		category = "other"
	}
	if !ticketpkg.Categories[category] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidCategory})
		return
	}
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.MessageRequired})
		return
	}
	if utf8.RuneCountInString(body) > 5000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.MessageTooLong})
		return
	}

	// Rate limit: max 5 tickets per hour per user
	var recentCount int
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM support_tickets WHERE user_id = $1 AND created_at > NOW() - INTERVAL '1 hour'`,
		userID,
	).Scan(&recentCount)
	if err != nil {
		a.log().Error("Failed to check ticket rate limit", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if recentCount >= 5 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.TooManyTickets})
		return
	}

	ticketID := uuid.New().String()
	messageID := uuid.New().String()

	// Handle optional file attachment
	var attachmentInfo *struct {
		fileName    string
		fileSize    int64
		contentType string
		storageKey  string
	}

	file, fileHeader, fileErr := r.FormFile("attachment")
	if fileErr == nil {
		defer file.Close()

		// Validate the file
		detectedType, err := ticketpkg.ValidateFile(fileHeader)
		if err != nil {
			a.log().Warn("ticket attachment validation failed", zap.Error(err))
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidAttachmentFile})
			return
		}

		ext := ticketpkg.MimeToExt(detectedType)
		fileUUID := uuid.New().String()
		storageKey := fmt.Sprintf("tickets/%s/%s/%s.%s", ticketID, messageID, fileUUID, ext)

		if err := ticketpkg.ValidateS3Key(storageKey); err != nil {
			a.log().Error("Invalid ticket S3 key generated", zap.String("key", storageKey), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		// Upload to S3 (using kycStorage which is the private bucket store)
		bucket := a.config.S3TicketBucket
		_, uploadErr := a.kycStorage.Upload(ctx, bucket, storageKey, file, fileHeader.Size, detectedType)
		if uploadErr != nil {
			a.log().Error("Failed to upload ticket attachment", zap.Error(uploadErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.AttachmentUploadFailed})
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

	// Create ticket + first message in a transaction
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
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
		`INSERT INTO support_tickets (id, user_id, subject, category, status, priority)
		 VALUES ($1, $2, $3, $4::ticket_category, 'open', 'medium')`,
		ticketID, userID, subject, category,
	)
	if err != nil {
		a.log().Error("Failed to create ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO ticket_messages (id, ticket_id, sender_id, is_admin, body)
		 VALUES ($1, $2, $3, false, $4)`,
		messageID, ticketID, userID, body,
	)
	if err != nil {
		a.log().Error("Failed to create ticket message", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
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
			a.log().Error("Failed to create ticket attachment", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit ticket transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	committed = true

	a.auditLogger.LogFromRequest(r, userID, audit.EventType("TICKET_CREATE"), map[string]interface{}{
		"entity":    "support_ticket",
		"ticket_id": ticketID,
		"category":  category,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       ticketID,
		"subject":  subject,
		"category": category,
		"status":   "open",
		"priority": "medium",
	})
}

// handleListUserTickets lists the current user's tickets (GET /api/user/me/tickets).
func (a *App) handleListUserTickets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	statusFilter := r.URL.Query().Get("status")

	// Build query
	query := `
		SELECT t.id, t.subject, t.category::text, t.status::text, t.priority::text,
		       t.created_at, t.updated_at,
		       (SELECT COUNT(*) FROM ticket_messages WHERE ticket_id = t.id) as message_count,
		       lm.body as last_message_body,
		       lm.created_at as last_message_at,
		       lm.is_admin as last_message_is_admin,
		       -- unread = last message is from admin AND user hasn't replied after it
		       COALESCE(lm.is_admin AND NOT EXISTS(
		           SELECT 1 FROM ticket_messages tm2
		           WHERE tm2.ticket_id = t.id AND tm2.is_admin = false AND tm2.created_at > lm.created_at
		       ), false) as unread
		FROM support_tickets t
		LEFT JOIN LATERAL (
		    SELECT body, created_at, is_admin FROM ticket_messages
		    WHERE ticket_id = t.id ORDER BY created_at DESC LIMIT 1
		) lm ON true
		WHERE t.user_id = $1
	`
	args := []interface{}{userID}
	argIdx := 2

	if statusFilter != "" && ticketpkg.Statuses[statusFilter] {
		query += fmt.Sprintf(" AND t.status = $%d::ticket_status", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM support_tickets WHERE user_id = $1"
	countArgs := []interface{}{userID}
	if statusFilter != "" && ticketpkg.Statuses[statusFilter] {
		countQuery += " AND status = $2::ticket_status"
		countArgs = append(countArgs, statusFilter)
	}

	var total int
	if err := a.pool.Replica().QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		a.log().Error("Failed to count tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	query += " ORDER BY t.updated_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to list tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	tickets := make([]ticketSummary, 0)
	for rows.Next() {
		var t ticketSummary
		var createdAt, updatedAt time.Time
		var lastBody sql.NullString
		var lastAt sql.NullTime
		var lastIsAdmin sql.NullBool
		var unread bool

		if err := rows.Scan(
			&t.ID, &t.Subject, &t.Category, &t.Status, &t.Priority,
			&createdAt, &updatedAt, &t.MessageCount,
			&lastBody, &lastAt, &lastIsAdmin, &unread,
		); err != nil {
			a.log().Error("Failed to scan ticket", zap.Error(err))
			continue
		}

		t.CreatedAt = createdAt.Format(time.RFC3339)
		t.UpdatedAt = updatedAt.Format(time.RFC3339)
		t.Unread = unread

		if lastBody.Valid {
			preview := lastBody.String
			if utf8.RuneCountInString(preview) > 100 {
				runes := []rune(preview)
				preview = string(runes[:100]) + "..."
			}
			t.LastMessagePreview = &preview
		}
		if lastAt.Valid {
			ts := lastAt.Time.Format(time.RFC3339)
			t.LastMessageAt = &ts
		}
		if lastIsAdmin.Valid {
			t.LastMessageIsAdmin = &lastIsAdmin.Bool
		}

		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, ticketListResponse{
		Tickets: tickets,
		Total:   total,
		HasMore: offset+limit < total,
	})
}

// handleGetTicketDetail returns ticket details with all messages (GET /api/user/me/tickets/{ticketId}).
func (a *App) handleGetTicketDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "ticketId")

	// Load ticket (ownership check)
	var info ticketInfo
	var createdAt, updatedAt time.Time
	var closedAt sql.NullTime
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT id, subject, category::text, status::text, priority::text, closed_at, created_at, updated_at
		 FROM support_tickets WHERE id = $1 AND user_id = $2`,
		ticketID, userID,
	).Scan(&info.ID, &info.Subject, &info.Category, &info.Status, &info.Priority,
		&closedAt, &createdAt, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.TicketNotFound})
			return
		}
		a.log().Error("Failed to load ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	info.CreatedAt = createdAt.Format(time.RFC3339)
	info.UpdatedAt = updatedAt.Format(time.RFC3339)
	if closedAt.Valid {
		ts := closedAt.Time.Format(time.RFC3339)
		info.ClosedAt = &ts
	}

	// Load messages with attachments
	messages, err := a.loadTicketMessages(ctx, ticketID)
	if err != nil {
		a.log().Error("Failed to load ticket messages", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, ticketDetailResponse{
		Ticket:   info,
		Messages: messages,
	})
}

// handleSendTicketMessage sends a message to a ticket (POST /api/user/me/tickets/{ticketId}/messages).
func (a *App) handleSendTicketMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "ticketId")

	// Check ticket ownership and status
	var ticketStatus string
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT status::text FROM support_tickets WHERE id = $1 AND user_id = $2`,
		ticketID, userID,
	).Scan(&ticketStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.TicketNotFound})
			return
		}
		a.log().Error("Failed to check ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if ticketStatus == "closed" || ticketStatus == "resolved" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TicketClosed})
		return
	}

	// Rate limit: max 10 messages per minute per user
	var recentMsgCount int
	err = a.pool.Replica().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ticket_messages tm
		 JOIN support_tickets st ON st.id = tm.ticket_id
		 WHERE st.user_id = $1 AND tm.is_admin = false AND tm.created_at > NOW() - INTERVAL '1 minute'`,
		userID,
	).Scan(&recentMsgCount)
	if err != nil {
		a.log().Error("Failed to check message rate limit", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if recentMsgCount >= 10 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.TooManyMessages})
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(35 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidForm})
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.MessageRequired})
		return
	}
	if utf8.RuneCountInString(body) > 5000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.MessageTooLong})
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
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": valErr.Error()})
			return
		}

		ext := ticketpkg.MimeToExt(detectedType)
		fileUUID := uuid.New().String()
		storageKey := fmt.Sprintf("tickets/%s/%s/%s.%s", ticketID, messageID, fileUUID, ext)

		if err := ticketpkg.ValidateS3Key(storageKey); err != nil {
			a.log().Error("Invalid ticket S3 key generated", zap.String("key", storageKey), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		bucket := a.config.S3TicketBucket
		_, uploadErr := a.kycStorage.Upload(ctx, bucket, storageKey, file, fileHeader.Size, detectedType)
		if uploadErr != nil {
			a.log().Error("Failed to upload ticket attachment", zap.Error(uploadErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.AttachmentUploadFailed})
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

	// Insert message and update ticket status in transaction
	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
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
		 VALUES ($1, $2, $3, false, $4)`,
		messageID, ticketID, userID, body,
	)
	if err != nil {
		a.log().Error("Failed to insert ticket message", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
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
			a.log().Error("Failed to insert attachment", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// Update ticket status to user_replied
	_, err = tx.ExecContext(ctx,
		`UPDATE support_tickets SET status = 'user_replied' WHERE id = $1 AND status != 'open'`,
		ticketID,
	)
	if err != nil {
		a.log().Error("Failed to update ticket status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit message transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	committed = true

	writeJSON(w, http.StatusCreated, map[string]string{"id": messageID, "status": "sent"})
}

// handleCloseTicket closes a ticket (POST /api/user/me/tickets/{ticketId}/close).
func (a *App) handleCloseTicket_Support(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	ticketID := chi.URLParam(r, "ticketId")

	result, err := a.pool.Primary().ExecContext(ctx,
		`UPDATE support_tickets SET status = 'closed', closed_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND status NOT IN ('closed', 'resolved')`,
		ticketID, userID,
	)
	if err != nil {
		a.log().Error("Failed to close ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.TicketAlreadyClosed})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// handleGetTicketAttachment downloads a ticket attachment (GET /api/user/me/tickets/attachment/{attachmentId}).
func (a *App) handleGetTicketAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	attachmentID := chi.URLParam(r, "attachmentId")

	if a.kycStorage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.FileStorageUnavailable})
		return
	}

	// Verify attachment belongs to a ticket owned by this user
	var storageKey, contentType, fileName string
	var fileSize int64
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT ta.storage_key, ta.content_type, ta.file_name, ta.file_size
		 FROM ticket_attachments ta
		 JOIN ticket_messages tm ON tm.id = ta.message_id
		 JOIN support_tickets st ON st.id = tm.ticket_id
		 WHERE ta.id = $1 AND st.user_id = $2`,
		attachmentID, userID,
	).Scan(&storageKey, &contentType, &fileName, &fileSize)

	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.AttachmentNotFound})
			return
		}
		a.log().Error("Failed to load attachment info", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.streamTicketAttachment(w, r, storageKey, contentType, fileName, fileSize)
}

// handleUnreadTicketCount returns the number of tickets with unread admin replies.
// GET /api/user/me/tickets/unread-count
func (a *App) handleUnreadTicketCount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var count int
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM support_tickets t
		WHERE t.user_id = $1
		  AND t.status NOT IN ('closed', 'resolved')
		  AND EXISTS (
		    SELECT 1 FROM ticket_messages lm
		    WHERE lm.ticket_id = t.id AND lm.is_admin = true
		      AND lm.created_at = (SELECT MAX(created_at) FROM ticket_messages WHERE ticket_id = t.id)
		      AND NOT EXISTS (
		        SELECT 1 FROM ticket_messages tm2
		        WHERE tm2.ticket_id = t.id AND tm2.is_admin = false AND tm2.created_at > lm.created_at
		      )
		  )
	`, userID).Scan(&count)

	if err != nil {
		a.log().Error("Failed to count unread tickets", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// ---- Shared helpers ----

// loadTicketMessages loads all messages for a ticket with their attachments.
func (a *App) loadTicketMessages(ctx context.Context, ticketID string) ([]ticketMessage, error) {
	rows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT tm.id, tm.body, tm.is_admin, tm.created_at
		 FROM ticket_messages tm
		 WHERE tm.ticket_id = $1
		 ORDER BY tm.created_at ASC`,
		ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ticketMessage
	var messageIDs []string
	messageMap := make(map[string]*ticketMessage)

	for rows.Next() {
		var m ticketMessage
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.Body, &m.IsAdmin, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		m.Attachments = make([]ticketAttachment, 0)

		if m.IsAdmin {
			m.SenderName = "پشتیبانی"
		} else {
			m.SenderName = "شما"
		}

		messages = append(messages, m)
		messageIDs = append(messageIDs, m.ID)
		messageMap[m.ID] = &messages[len(messages)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating messages: %w", err)
	}

	// Load attachments for all messages
	if len(messageIDs) > 0 {
		// Build IN clause
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
			var att ticketAttachment
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

// streamTicketAttachment streams an attachment file from S3 to the HTTP response.
func (a *App) streamTicketAttachment(w http.ResponseWriter, r *http.Request, storageKey, contentType, fileName string, fileSize int64) {
	ctx := r.Context()
	bucket := a.config.S3TicketBucket

	reader, _, _, err := a.kycStorage.Download(ctx, bucket, storageKey)
	if err != nil {
		a.log().Error("Failed to download attachment from S3", zap.String("key", storageKey), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.AttachmentDownloadFailed})
		return
	}
	defer reader.Close()

	// Security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))

	// Inline for images, attachment for PDFs
	safeName := ticketpkg.SanitizeFileName(fileName)
	if strings.HasPrefix(contentType, "image/") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeName))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	}

	io.Copy(w, reader)
}
