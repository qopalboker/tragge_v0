package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// =============================================================================
// KYC HANDLERS
// =============================================================================

// handleListPendingKYC lists all KYC submissions with status 'pending' or 'under_review'.
func (a *App) handleListPendingKYC(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50 // default
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get total count first (circuit breaker protected)
	var total int
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM user_verification uv
			 JOIN users u ON u.id = uv.user_id
			 WHERE uv.status IN ('pending', 'under_review')`).Scan(&total)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count pending KYC submissions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Query pending submissions with user info and latest document type (circuit breaker protected)
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx,
				`SELECT
					uv.user_id,
					u.email,
					uv.first_name,
					uv.last_name,
					(SELECT document_type::TEXT FROM kyc_documents WHERE user_id = uv.user_id ORDER BY created_at DESC LIMIT 1) as document_type,
					uv.status,
					uv.created_at,
					uv.provider,
					uv.shahkar_verified,
					uv.face_verified,
					uv.card_ocr_verified,
					uv.face_match_score
				 FROM user_verification uv
				 JOIN users u ON u.id = uv.user_id
				 WHERE uv.status IN ('pending', 'under_review')
				 ORDER BY uv.created_at ASC
				 LIMIT $1 OFFSET $2`,
				limit, offset)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query pending KYC submissions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var submissions []KYCPendingSubmission
	for rows.Next() {
		var s KYCPendingSubmission
		var faceMatchScore sql.NullFloat64
		if err := rows.Scan(&s.UserID, &s.Email, &s.FirstName, &s.LastName, &s.DocumentType, &s.Status, &s.SubmittedAt,
			&s.Provider, &s.ShahkarVerified, &s.FaceVerified, &s.CardOCRVerified, &faceMatchScore); err != nil {
			a.log().Error("Failed to scan KYC submission", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		if faceMatchScore.Valid {
			s.FaceMatchScore = &faceMatchScore.Float64
		}
		s.AutoApproved = s.ShahkarVerified && s.FaceVerified && s.CardOCRVerified &&
			faceMatchScore.Valid && faceMatchScore.Float64 > 0.85
		submissions = append(submissions, s)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate KYC submissions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if submissions == nil {
		submissions = []KYCPendingSubmission{}
	}

	writeJSON(w, http.StatusOK, KYCPendingListResponse{
		Submissions: submissions,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	})
}

// handleGetKYCDetails returns full KYC details for a user.
func (a *App) handleGetKYCDetails(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	ctx := r.Context()

	// Get user email (circuit breaker protected)
	var userEmail string
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to get user email", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Get user verification details
	var uv KYCVerificationDetails
	var dateOfBirth, verifiedAt, expiresAt sql.NullTime
	var faceMatchScore, livenessScore sql.NullFloat64
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT user_id, status, first_name, last_name, date_of_birth, nationality,
			        address_line1, address_line2, city, state, postal_code, country,
			        verified_at, verified_by, rejection_reason, expires_at,
			        provider, provider_verification_id, created_at, updated_at,
			        national_code, phone, shahkar_verified, face_verified,
			        face_match_score, liveness_score, liveness_result,
			        card_ocr_verified, card_serial_number,
			        national_code_manual, father_name, birth_certificate_number, birth_certificate_serial
			 FROM user_verification WHERE user_id = $1`, userID).Scan(
			&uv.UserID, &uv.Status, &uv.FirstName, &uv.LastName, &dateOfBirth, &uv.Nationality,
			&uv.AddressLine1, &uv.AddressLine2, &uv.City, &uv.State, &uv.PostalCode, &uv.Country,
			&verifiedAt, &uv.VerifiedBy, &uv.RejectionReason, &expiresAt,
			&uv.Provider, &uv.ProviderVerificationID, &uv.CreatedAt, &uv.UpdatedAt,
			&uv.NationalCode, &uv.Phone, &uv.ShahkarVerified, &uv.FaceVerified,
			&faceMatchScore, &livenessScore, &uv.LivenessResult,
			&uv.CardOCRVerified, &uv.CardSerialNumber,
			&uv.NationalCodeManual, &uv.FatherName, &uv.BirthCertificateNumber, &uv.BirthCertificateSerial)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCNotFound})
			return
		}
		a.log().Error("Failed to get user verification", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Convert nullable dates
	if dateOfBirth.Valid {
		dob := dateOfBirth.Time.Format("2006-01-02")
		uv.DateOfBirth = &dob
	}
	if faceMatchScore.Valid {
		uv.FaceMatchScore = &faceMatchScore.Float64
	}
	if livenessScore.Valid {
		uv.LivenessScore = &livenessScore.Float64
	}
	if verifiedAt.Valid {
		uv.VerifiedAt = &verifiedAt.Time
	}
	if expiresAt.Valid {
		uv.ExpiresAt = &expiresAt.Time
	}

	// Compute auto_approved flag
	uv.AutoApproved = uv.ShahkarVerified && uv.FaceVerified && uv.CardOCRVerified &&
		faceMatchScore.Valid && faceMatchScore.Float64 > 0.85

	// Get KYC documents (circuit breaker protected)
	docResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx,
				`SELECT id, user_id, document_type, document_number, issuing_country,
				        issue_date, expiry_date, front_image_url, back_image_url, selfie_url,
				        selfie_with_doc_url, status, reviewed_at, reviewed_by, review_notes, created_at
				 FROM kyc_documents WHERE user_id = $1 ORDER BY created_at DESC`, userID)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query KYC documents", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	docRows := docResult.(*sql.Rows)
	defer docRows.Close()

	var documents []KYCDocument
	for docRows.Next() {
		var doc KYCDocument
		var issueDate, expiryDate, reviewedAt sql.NullTime
		if err := docRows.Scan(
			&doc.ID, &doc.UserID, &doc.DocumentType, &doc.DocumentNumber, &doc.IssuingCountry,
			&issueDate, &expiryDate, &doc.FrontImageURL, &doc.BackImageURL, &doc.SelfieURL,
			&doc.SelfieWithDocURL, &doc.Status, &reviewedAt, &doc.ReviewedBy, &doc.ReviewNotes, &doc.CreatedAt); err != nil {
			a.log().Error("Failed to scan KYC document", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		if issueDate.Valid {
			id := issueDate.Time.Format("2006-01-02")
			doc.IssueDate = &id
		}
		if expiryDate.Valid {
			ed := expiryDate.Time.Format("2006-01-02")
			doc.ExpiryDate = &ed
		}
		if reviewedAt.Valid {
			doc.ReviewedAt = &reviewedAt.Time
		}
		documents = append(documents, doc)
	}

	if err := docRows.Err(); err != nil {
		a.log().Error("Failed to iterate KYC documents", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Get KYC audit log (circuit breaker protected)
	auditResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx,
				`SELECT id, user_id, action, actor_id, details, created_at
				 FROM kyc_audit_log WHERE user_id = $1 ORDER BY created_at DESC`, userID)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query KYC audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	auditRows := auditResult.(*sql.Rows)
	defer auditRows.Close()

	var auditLog []KYCAuditLogEntry
	for auditRows.Next() {
		var entry KYCAuditLogEntry
		if err := auditRows.Scan(&entry.ID, &entry.UserID, &entry.Action, &entry.ActorID, &entry.Details, &entry.CreatedAt); err != nil {
			a.log().Error("Failed to scan KYC audit log entry", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		auditLog = append(auditLog, entry)
	}

	if err := auditRows.Err(); err != nil {
		a.log().Error("Failed to iterate KYC audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if documents == nil {
		documents = []KYCDocument{}
	}
	if auditLog == nil {
		auditLog = []KYCAuditLogEntry{}
	}

	writeJSON(w, http.StatusOK, KYCDetailsResponse{
		User:      uv,
		Documents: documents,
		AuditLog:  auditLog,
		UserEmail: userEmail,
	})
}

// handleApproveKYC approves a user's KYC verification.
func (a *App) handleApproveKYC(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	var req KYCApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		validation.WriteBadRequest(w, "invalid request body")
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

	// Check if user verification exists and is in a reviewable state
	var currentStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM user_verification WHERE user_id = $1 FOR UPDATE`, userID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCNotFound})
			return
		}
		a.log().Error("Failed to get user verification status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" && currentStatus != "under_review" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCNotReviewable})
		return
	}

	// Update user_verification status to verified
	expiresAt := time.Now().AddDate(1, 0, 0) // 1 year from now
	_, err = tx.ExecContext(ctx,
		`UPDATE user_verification
		 SET status = 'verified', verified_at = NOW(), verified_by = $1, expires_at = $2
		 WHERE user_id = $3`,
		actorUserID, expiresAt, userID)
	if err != nil {
		a.log().Error("Failed to update user verification", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Update all pending/under_review kyc_documents to verified
	_, err = tx.ExecContext(ctx,
		`UPDATE kyc_documents
		 SET status = 'verified', reviewed_at = NOW(), reviewed_by = $1
		 WHERE user_id = $2 AND status IN ('pending', 'under_review')`,
		actorUserID, userID)
	if err != nil {
		a.log().Error("Failed to update KYC documents", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add KYC audit log entry
	details := map[string]interface{}{
		"previous_status": currentStatus,
		"new_status":      "verified",
		"expires_at":      expiresAt.Format(time.RFC3339),
	}
	if req.Notes != nil {
		details["notes"] = *req.Notes
	}
	detailsJSON, _ := json.Marshal(details)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details)
		 VALUES ($1, 'approved', $2, $3)`,
		userID, actorUserID, detailsJSON)
	if err != nil {
		a.log().Error("Failed to write KYC audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write to general audit_logs table
	auditPayload := map[string]interface{}{
		"user_id":         userID,
		"action":          "kyc.approved",
		"previous_status": currentStatus,
		"notes":           req.Notes,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "kyc.approved", "kyc", userID, auditPayloadJSON)
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

	// Ensure wallet exists for verified user (safety net — trigger should have created it)
	infra.SafeGo(a.log(), "kyc-ensure-wallet", func() {
		ensureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := a.walletService.CreateWallet(ensureCtx, userID); err != nil {
			a.log().Warn("Failed to ensure wallet for KYC-approved user",
				zap.Error(err),
				zap.String("user_id", userID))
		} else {
			a.log().Info("Wallet ensured for KYC-approved user",
				zap.String("user_id", userID))
		}
	})

	// Send approval notification email asynchronously
	if a.emailNotifier != nil {
		infra.SafeGo(a.log(), "kyc-approval-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			// Get user email and name
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil {
				a.log().Error("Failed to get user email for KYC notification", zap.Error(err), zap.String("user_id", userID))
				return
			}

			if !userEmail.Valid || userEmail.String == "" {
				a.log().Warn("User has no email, skipping KYC approval notification", zap.String("user_id", userID))
				return
			}

			emailData := notification.KYCApprovedData{
				UserName:     userName.String,
				ExpiresAt:    expiresAt.Format("January 2, 2006"),
				DashboardURL: a.config.FrontendBaseURL + "/dashboard",
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "kyc_update", "email")
			if !emailEnabled {
				return
			}

			if err := a.emailNotifier.SendKYCApproved(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send KYC approval email",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("email", userEmail.String))
			} else {
				a.log().Info("KYC approval email sent", zap.String("user_id", userID), zap.String("email", userEmail.String))
			}
		})
	}

	// Send in-app notification asynchronously (respects user preferences)
	infra.SafeGo(a.log(), "kyc-approved-inapp-notification", func() {
		notifCtx, notifCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer notifCancel()
		enabled, _ := prefs.IsEnabled(notifCtx, a.pool.Replica(), userID, inapp.NotifTypeKYCUpdate, "in_app")
		if !enabled {
			return
		}
		notifMsg := "Your identity has been verified. You can now use all platform features."
		if err := inapp.CreateKYCUpdateNotification(notifCtx, a.pool.Primary(), userID, "approved", notifMsg); err != nil {
			a.log().Warn("Failed to create KYC approved in-app notification",
				zap.Error(err), zap.String("user_id", userID))
		}
	})

	a.log().Info("KYC approved",
		zap.String("user_id", userID),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    adminMsg.KYCApproved,
		"user_id":    userID,
		"status":     "verified",
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// handleRejectKYC rejects a user's KYC verification.
func (a *App) handleRejectKYC(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	var req KYCRejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate reason is provided
	v := validation.New()
	req.Reason = v.String("reason", req.Reason, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 1000,
		TrimSpace: true,
	})
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Validate rejected_fields if provided
	validRejectionFields := map[string]bool{
		"first_name": true, "last_name": true, "father_name": true,
		"national_code": true, "date_of_birth": true, "address": true,
		"front_image": true, "back_image": true, "selfie_with_doc": true,
		"document_number": true, "birth_certificate_number": true,
	}
	if len(req.RejectedFields) > 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCTooManyRejected})
		return
	}
	for _, f := range req.RejectedFields {
		if !validRejectionFields[f] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidRejectedField})
			return
		}
	}
	// Validate field_messages keys are a subset of rejected_fields
	if len(req.FieldMessages) > 0 {
		rejectedSet := make(map[string]bool, len(req.RejectedFields))
		for _, f := range req.RejectedFields {
			rejectedSet[f] = true
		}
		for k := range req.FieldMessages {
			if !rejectedSet[k] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.FieldMessageKeyMismatch})
				return
			}
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

	// Check if user verification exists and is in a reviewable state
	var currentStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM user_verification WHERE user_id = $1 FOR UPDATE`, userID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCNotFound})
			return
		}
		a.log().Error("Failed to get user verification status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" && currentStatus != "under_review" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCNotReviewable})
		return
	}

	// Marshal rejection fields to JSONB
	rejectionFieldsJSON, _ := json.Marshal(req.RejectedFields)
	if req.RejectedFields == nil {
		rejectionFieldsJSON = []byte("[]")
	}
	fieldMessagesJSON, _ := json.Marshal(req.FieldMessages)
	if req.FieldMessages == nil {
		fieldMessagesJSON = []byte("{}")
	}

	// Update user_verification status to rejected with per-field details
	_, err = tx.ExecContext(ctx,
		`UPDATE user_verification
		 SET status = 'rejected', rejection_reason = $1,
		     rejection_fields = $2::jsonb,
		     rejection_field_messages = $3::jsonb
		 WHERE user_id = $4`,
		req.Reason, rejectionFieldsJSON, fieldMessagesJSON, userID)
	if err != nil {
		a.log().Error("Failed to update user verification", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Update all pending/under_review kyc_documents to rejected
	_, err = tx.ExecContext(ctx,
		`UPDATE kyc_documents
		 SET status = 'rejected', reviewed_at = NOW(), reviewed_by = $1, review_notes = $2
		 WHERE user_id = $3 AND status IN ('pending', 'under_review')`,
		actorUserID, req.Reason, userID)
	if err != nil {
		a.log().Error("Failed to update KYC documents", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add KYC audit log entry
	details := map[string]interface{}{
		"previous_status": currentStatus,
		"new_status":      "rejected",
		"reason":          req.Reason,
		"rejected_fields": req.RejectedFields,
		"field_messages":  req.FieldMessages,
	}
	detailsJSON, _ := json.Marshal(details)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details)
		 VALUES ($1, 'rejected', $2, $3)`,
		userID, actorUserID, detailsJSON)
	if err != nil {
		a.log().Error("Failed to write KYC audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write to general audit_logs table
	auditPayload := map[string]interface{}{
		"user_id":         userID,
		"action":          "kyc.rejected",
		"previous_status": currentStatus,
		"reason":          req.Reason,
		"rejected_fields": req.RejectedFields,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "kyc.rejected", "kyc", userID, auditPayloadJSON)
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

	// Send rejection notification email asynchronously
	if a.emailNotifier != nil {
		rejectionReason := req.Reason
		infra.SafeGo(a.log(), "kyc-rejection-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			// Get user email and name
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil {
				a.log().Error("Failed to get user email for KYC notification", zap.Error(err), zap.String("user_id", userID))
				return
			}

			if !userEmail.Valid || userEmail.String == "" {
				a.log().Warn("User has no email, skipping KYC rejection notification", zap.String("user_id", userID))
				return
			}

			emailData := notification.KYCRejectedData{
				UserName:        userName.String,
				Reason:          rejectionReason,
				VerificationURL: a.config.FrontendBaseURL + "/kyc/verify",
				RejectedFields:  req.RejectedFields,
				FieldMessages:   req.FieldMessages,
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "kyc_update", "email")
			if !emailEnabled {
				return
			}

			if err := a.emailNotifier.SendKYCRejected(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send KYC rejection email",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("email", userEmail.String))
			} else {
				a.log().Info("KYC rejection email sent", zap.String("user_id", userID), zap.String("email", userEmail.String))
			}
		})
	}

	// Send in-app notification asynchronously (respects user preferences)
	infra.SafeGo(a.log(), "kyc-rejection-inapp", func() {
		notifCtx, notifCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer notifCancel()
		enabled, _ := prefs.IsEnabled(notifCtx, a.pool.Replica(), userID, inapp.NotifTypeKYCUpdate, "in_app")
		if !enabled {
			return
		}
		notifMsg := "Your verification was rejected."
		if req.Reason != "" {
			notifMsg += " Reason: " + req.Reason
		}
		notifMsg += " Please fix the issues and resubmit."
		if err := inapp.CreateKYCUpdateNotification(notifCtx, a.pool.Primary(), userID, "rejected", notifMsg); err != nil {
			a.log().Warn("Failed to create KYC rejected in-app notification",
				zap.Error(err), zap.String("user_id", userID))
		}
	})

	a.log().Info("KYC rejected",
		zap.String("user_id", userID),
		zap.String("actor_id", actorUserID),
		zap.String("reason", req.Reason))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.KYCRejected,
		"user_id": userID,
		"status":  "rejected",
		"reason":  req.Reason,
	})
}

// handleBulkAutoApproveKYC approves all pending KYC submissions that pass Jibit verification criteria.
// Criteria: shahkar_verified + face_verified (score > 0.85) + card_ocr_verified.
func (a *App) handleBulkAutoApproveKYC(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Find all eligible users for auto-approval (circuit breaker protected)
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx,
				`SELECT user_id FROM user_verification
				 WHERE status IN ('pending', 'under_review')
				   AND shahkar_verified = true
				   AND face_verified = true
				   AND face_match_score > 0.85
				   AND card_ocr_verified = true`)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query auto-approvable KYC submissions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			a.log().Error("Failed to scan user_id", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		userIDs = append(userIDs, uid)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate auto-approvable users", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if len(userIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message":  adminMsg.KYCNoEligible,
			"approved": 0,
		})
		return
	}

	// Approve users in batched transactions for efficiency
	approvedCount := 0
	var failedUserIDs []string
	expiresAt := time.Now().AddDate(1, 0, 0)

	const batchSize = 50
	for i := 0; i < len(userIDs); i += batchSize {
		end := i + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		batch := userIDs[i:end]

		var tx *sql.Tx
		err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
			var beginErr error
			tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
			return beginErr
		})
		if err != nil {
			a.log().Error("Failed to begin transaction for auto-approve batch", zap.Error(err), zap.Int("batch_start", i))
			failedUserIDs = append(failedUserIDs, batch...)
			continue
		}

		batchFailed := false
		batchApproved := 0
		for _, uid := range batch {
			// Lock and verify the row is still in a reviewable state
			var currentStatus string
			err = tx.QueryRowContext(ctx,
				`SELECT status FROM user_verification WHERE user_id = $1 FOR UPDATE`, uid).Scan(&currentStatus)
			if err != nil || (currentStatus != "pending" && currentStatus != "under_review") {
				if err != nil {
					a.log().Error("Failed to lock user verification for auto-approve", zap.Error(err), zap.String("user_id", uid))
				}
				failedUserIDs = append(failedUserIDs, uid)
				continue
			}

			// Update to verified
			_, err = tx.ExecContext(ctx,
				`UPDATE user_verification
				 SET status = 'verified', verified_at = NOW(), verified_by = $1, expires_at = $2
				 WHERE user_id = $3`,
				actorUserID, expiresAt, uid)
			if err != nil {
				a.log().Error("Failed to auto-approve user verification", zap.Error(err), zap.String("user_id", uid))
				failedUserIDs = append(failedUserIDs, uid)
				// Transaction may be in a bad state after an error; abort batch
				batchFailed = true
				break
			}

			// Update documents
			if _, err := tx.ExecContext(ctx,
				`UPDATE kyc_documents
				 SET status = 'verified', reviewed_at = NOW(), reviewed_by = $1
				 WHERE user_id = $2 AND status IN ('pending', 'under_review')`,
				actorUserID, uid); err != nil {
				a.log().Error("Failed to update KYC documents", zap.Error(err), zap.String("user_id", uid))
				failedUserIDs = append(failedUserIDs, uid)
				batchFailed = true
				break
			}

			// Audit log
			details := map[string]interface{}{
				"previous_status": currentStatus,
				"new_status":      "verified",
				"method":          "bulk_auto_approve",
				"expires_at":      expiresAt.Format(time.RFC3339),
			}
			detailsJSON, _ := json.Marshal(details)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO kyc_audit_log (user_id, action, actor_id, details)
				 VALUES ($1, 'approved', $2, $3)`,
				uid, actorUserID, detailsJSON); err != nil {
				a.log().Error("Failed to insert KYC audit log", zap.Error(err), zap.String("user_id", uid))
				failedUserIDs = append(failedUserIDs, uid)
				batchFailed = true
				break
			}

			auditPayload := map[string]interface{}{
				"user_id":         uid,
				"action":          "kyc.bulk_auto_approved",
				"previous_status": currentStatus,
			}
			auditPayloadJSON, _ := json.Marshal(auditPayload)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
				 VALUES ($1, $2, $3, $4, $5)`,
				actorUserID, "kyc.bulk_auto_approved", "kyc", uid, auditPayloadJSON); err != nil {
				a.log().Error("Failed to insert audit log", zap.Error(err), zap.String("user_id", uid))
				failedUserIDs = append(failedUserIDs, uid)
				batchFailed = true
				break
			}

			batchApproved++
		}

		if batchFailed {
			tx.Rollback()
			// Count remaining unprocessed users in this batch as failed
			continue
		}

		if err := tx.Commit(); err != nil {
			a.log().Error("Failed to commit auto-approve batch transaction", zap.Error(err), zap.Int("batch_start", i))
			// Users that were individually tracked as failed stay failed;
			// the ones that appeared to succeed in this batch also failed due to commit error
			failedUserIDs = append(failedUserIDs, batch...)
			// Subtract the ones we already individually added to failedUserIDs
			approvedCount -= batchApproved
			continue
		}

		approvedCount += batchApproved

		// Ensure wallets exist for approved users in this batch
		approvedBatch := batch
		infra.SafeGo(a.log(), "kyc-bulk-approve-wallets", func() {
			ensureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			for _, uid := range approvedBatch {
				if _, err := a.walletService.CreateWallet(ensureCtx, uid); err != nil {
					a.log().Warn("Failed to ensure wallet for bulk KYC-approved user",
						zap.Error(err),
						zap.String("user_id", uid))
				}
			}
		})
	}

	a.log().Info("Bulk auto-approve KYC completed",
		zap.String("actor_id", actorUserID),
		zap.Int("approved", approvedCount),
		zap.Int("failed", len(failedUserIDs)))

	resp := map[string]interface{}{
		"message":  adminMsg.KYCBulkApproved,
		"approved": approvedCount,
		"total":    len(userIDs),
	}
	if len(failedUserIDs) > 0 {
		resp["failed"] = failedUserIDs
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleRequestKYCInfo requests additional information from the user.
func (a *App) handleRequestKYCInfo(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	var req KYCRequestInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate message is provided
	v := validation.New()
	req.Message = v.String("message", req.Message, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 2000,
		TrimSpace: true,
	})
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

	// Check if user verification exists
	var currentStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM user_verification WHERE user_id = $1 FOR UPDATE`, userID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCNotFound})
			return
		}
		a.log().Error("Failed to get user verification status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" && currentStatus != "under_review" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCNotReviewable})
		return
	}

	// Update status to under_review if it was pending
	if currentStatus == "pending" {
		_, err = tx.ExecContext(ctx,
			`UPDATE user_verification SET status = 'under_review' WHERE user_id = $1`,
			userID)
		if err != nil {
			a.log().Error("Failed to update user verification status", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Add KYC audit log entry with the request message
	details := map[string]interface{}{
		"previous_status": currentStatus,
		"new_status":      "under_review",
		"message":         req.Message,
		"action_type":     "info_requested",
	}
	detailsJSON, _ := json.Marshal(details)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details)
		 VALUES ($1, 'info_requested', $2, $3)`,
		userID, actorUserID, detailsJSON)
	if err != nil {
		a.log().Error("Failed to write KYC audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write to general audit_logs table
	auditPayload := map[string]interface{}{
		"user_id":         userID,
		"action":          "kyc.info_requested",
		"previous_status": currentStatus,
		"message":         req.Message,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "kyc.info_requested", "kyc", userID, auditPayloadJSON)
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

	// Send info request notification email asynchronously
	if a.emailNotifier != nil {
		infoMessage := req.Message
		infra.SafeGo(a.log(), "kyc-info-request-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			// Get user email and name
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil {
				a.log().Error("Failed to get user email for KYC notification", zap.Error(err), zap.String("user_id", userID))
				return
			}

			if !userEmail.Valid || userEmail.String == "" {
				a.log().Warn("User has no email, skipping KYC info request notification", zap.String("user_id", userID))
				return
			}

			emailData := notification.KYCInfoRequestData{
				UserName:        userName.String,
				Message:         infoMessage,
				VerificationURL: a.config.FrontendBaseURL + "/kyc/verify",
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "kyc_update", "email")
			if !emailEnabled {
				return
			}

			if err := a.emailNotifier.SendKYCInfoRequest(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send KYC info request email",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("email", userEmail.String))
			} else {
				a.log().Info("KYC info request email sent", zap.String("user_id", userID), zap.String("email", userEmail.String))
			}
		})
	}

	// Send in-app notification asynchronously (respects user preferences)
	infra.SafeGo(a.log(), "kyc-info-request-inapp", func() {
		notifCtx, notifCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer notifCancel()
		enabled, _ := prefs.IsEnabled(notifCtx, a.pool.Replica(), userID, inapp.NotifTypeKYCUpdate, "in_app")
		if !enabled {
			return
		}
		notifMsg := "Our review team has requested additional information."
		if err := inapp.CreateKYCUpdateNotification(notifCtx, a.pool.Primary(), userID, "pending", notifMsg); err != nil {
			a.log().Warn("Failed to create KYC info-request in-app notification",
				zap.Error(err), zap.String("user_id", userID))
		}
	})

	a.log().Info("KYC info requested",
		zap.String("user_id", userID),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.KYCInfoRequested,
		"user_id": userID,
		"status":  "under_review",
	})
}

// handleGetKYCDocumentImage serves KYC document images securely.
func (a *App) handleGetKYCDocumentImage(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "document_id")
	imageType := chi.URLParam(r, "type")

	if documentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCDocIDRequired})
		return
	}

	// Validate image type against whitelist (prevents SQL injection in column name interpolation)
	validTypes := map[string]string{
		"front":           "front_image_url",
		"back":            "back_image_url",
		"selfie":          "selfie_url",
		"selfie_with_doc": "selfie_with_doc_url",
	}
	column, ok := validTypes[imageType]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.KYCInvalidImageType})
		return
	}

	ctx := r.Context()

	// Get the image path from the database (circuit breaker protected)
	var imagePath sql.NullString
	// SAFETY: column is validated against validTypes whitelist above
	query := fmt.Sprintf(`SELECT %s FROM kyc_documents WHERE id = $1`, column)
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, query, documentID).Scan(&imagePath)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCDocNotFound})
			return
		}
		a.log().Error("Failed to get document image path", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if !imagePath.Valid || imagePath.String == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCImageNotFound})
		return
	}

	storedValue := imagePath.String

	// Guard: detect and reject legacy local filesystem paths.
	// These should not exist in production, but log and return a clear error if found.
	if strings.HasPrefix(storedValue, "/") || strings.Contains(storedValue, "data/uploads/") {
		a.log().Error("KYC document references legacy local filesystem path",
			zap.String("document_id", documentID),
			zap.String("stored_path", storedValue))
		writeJSON(w, http.StatusGone, map[string]string{
			"error": adminMsg.KYCDocLegacy,
		})
		return
	}

	// Serve from S3
	if a.kycStorage == nil {
		a.log().Error("KYC S3 storage not configured, cannot serve document",
			zap.String("document_id", documentID),
			zap.String("key", storedValue))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.KYCStorageNotConfigured})
		return
	}

	reader, contentType, size, err := a.kycStorage.Download(ctx, a.config.S3KYCBucket, storedValue)
	if err != nil {
		a.log().Error("Failed to download KYC document from S3",
			zap.Error(err),
			zap.String("document_id", documentID),
			zap.String("key", storedValue))
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.KYCImageNotFound})
		return
	}
	defer reader.Close()

	// Set security headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `inline; filename="kyc_document.jpg"`)

	// Stream the file
	if _, err := io.Copy(w, reader); err != nil {
		a.log().Error("Failed to stream KYC document from S3", zap.Error(err))
	}
}
