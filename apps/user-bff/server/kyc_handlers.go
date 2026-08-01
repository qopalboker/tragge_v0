package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (a *App) handleKYCStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Query user_verification table (read-only, use replica)
	var status string
	var firstName, lastName sql.NullString
	var verifiedAt sql.NullTime
	var rejectionReason sql.NullString
	var shahkarVerified, faceVerified, cardOCRVerified bool
	var faceMatchScore, livenessScore sql.NullFloat64
	var fatherName, nationalCodeManual, phone, city, addressLine1, postalCode, state sql.NullString
	var dateOfBirth sql.NullTime
	var rejectionFieldsJSON, rejectionFieldMsgsJSON sql.NullString

	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT status, first_name, last_name, verified_at, rejection_reason,
		       shahkar_verified, face_verified, face_match_score, liveness_score, card_ocr_verified,
		       father_name, national_code_manual, phone, city, address_line1, postal_code, state,
		       date_of_birth, rejection_fields::text, rejection_field_messages::text
		FROM user_verification
		WHERE user_id = $1
	`, userID).Scan(&status, &firstName, &lastName, &verifiedAt, &rejectionReason,
		&shahkarVerified, &faceVerified, &faceMatchScore, &livenessScore, &cardOCRVerified,
		&fatherName, &nationalCodeManual, &phone, &city, &addressLine1, &postalCode, &state,
		&dateOfBirth, &rejectionFieldsJSON, &rejectionFieldMsgsJSON)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, KYCStatusResponse{
				Status: "none",
			})
			return
		}
		a.log().Error("Failed to query KYC status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	response := KYCStatusResponse{
		Status:          status,
		ShahkarVerified: shahkarVerified,
		FaceVerified:    faceVerified,
		CardOCRVerified: cardOCRVerified,
	}

	if firstName.Valid {
		response.FirstName = &firstName.String
	}
	if lastName.Valid {
		response.LastName = &lastName.String
	}
	if verifiedAt.Valid {
		response.VerifiedAt = &verifiedAt.Time
	}
	if rejectionReason.Valid {
		response.RejectionReason = &rejectionReason.String
	}
	if faceMatchScore.Valid {
		response.FaceMatchScore = &faceMatchScore.Float64
	}
	if livenessScore.Valid {
		response.LivenessScore = &livenessScore.Float64
	}

	// Only return pre-populated data for rejected status (minimize data exposure)
	if status == "rejected" {
		if fatherName.Valid {
			response.FatherNamePrefill = &fatherName.String
		}
		if nationalCodeManual.Valid {
			response.NationalCodePrefill = &nationalCodeManual.String
		}
		if dateOfBirth.Valid {
			dob := dateOfBirth.Time.Format("2006-01-02")
			response.DateOfBirthPrefill = &dob
		}
		if phone.Valid {
			response.PhonePrefill = &phone.String
		}
		if city.Valid {
			response.CityPrefill = &city.String
		}
		if addressLine1.Valid {
			response.AddressPrefill = &addressLine1.String
		}
		if postalCode.Valid {
			response.PostalCodePrefill = &postalCode.String
		}
		if state.Valid {
			response.ProvincePrefill = &state.String
		}

		// Parse rejection fields
		if rejectionFieldsJSON.Valid && rejectionFieldsJSON.String != "" && rejectionFieldsJSON.String != "[]" {
			var fields []string
			if err := json.Unmarshal([]byte(rejectionFieldsJSON.String), &fields); err == nil && len(fields) > 0 {
				response.RejectionFields = fields
			}
		}
		if rejectionFieldMsgsJSON.Valid && rejectionFieldMsgsJSON.String != "" && rejectionFieldMsgsJSON.String != "{}" {
			var msgs map[string]string
			if err := json.Unmarshal([]byte(rejectionFieldMsgsJSON.String), &msgs); err == nil && len(msgs) > 0 {
				response.RejectionFieldMessages = msgs
			}
		}

		// Also query latest document info for prefill
		var docType, docNumber, birthCertNum, birthCertSerial sql.NullString
		docErr := a.pool.Replica().QueryRowContext(ctx, `
			SELECT document_type::text, document_number,
			       (SELECT birth_certificate_number FROM user_verification WHERE user_id = $1),
			       (SELECT birth_certificate_serial FROM user_verification WHERE user_id = $1)
			FROM kyc_documents WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1
		`, userID).Scan(&docType, &docNumber, &birthCertNum, &birthCertSerial)
		if docErr == nil {
			if docType.Valid {
				response.DocumentTypePrefill = &docType.String
			}
			if docNumber.Valid {
				response.DocumentNumberPrefill = &docNumber.String
			}
			if birthCertNum.Valid {
				response.BirthCertNumberPrefill = &birthCertNum.String
			}
			if birthCertSerial.Valid {
				response.BirthCertSerialPrefill = &birthCertSerial.String
			}
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// nationalCodeRegex validates Iranian national codes (exactly 10 digits).
var nationalCodeRegex = regexp.MustCompile(`^\d{10}$`)

// iranPhoneRegex validates Iranian phone numbers (09XXXXXXXXX).
var iranPhoneRegex = regexp.MustCompile(`^09\d{9}$`)

// validateIranianNationalCode validates an Iranian national code using the checksum algorithm.
func validateIranianNationalCode(code string) bool {
	if !nationalCodeRegex.MatchString(code) {
		return false
	}
	// All digits can't be the same (e.g. 1111111111)
	allSame := true
	for i := 1; i < len(code); i++ {
		if code[i] != code[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}
	// Checksum: sum of (digit[i] * (10-i)) for i=0..8
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(code[i]-'0') * (10 - i)
	}
	remainder := sum % 11
	checkDigit := int(code[9] - '0')
	if remainder < 2 {
		return checkDigit == remainder
	}
	return checkDigit == 11-remainder
}

// handleKYCVerifyPhone handles Step 1 of Jibit KYC: Shahkar phone+national code verification.
// POST /api/user/kyc/verify-phone
func (a *App) handleKYCVerifyPhone(w http.ResponseWriter, r *http.Request) {
	if a.jibitKYC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.KYCServiceUnavailable})
		return
	}

	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req struct {
		NationalCode string `json:"national_code"`
		Phone        string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	// Validate national code (10 digits)
	if !nationalCodeRegex.MatchString(req.NationalCode) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "کد ملی باید ۱۰ رقم باشد"})
		return
	}

	// Validate phone number (Iranian format)
	if !iranPhoneRegex.MatchString(req.Phone) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "شماره موبایل نامعتبر است (مثال: 09123456789)"})
		return
	}

	// Call Jibit Shahkar API
	result, err := a.jibitKYC.VerifyShahkar(ctx, req.Phone, req.NationalCode)
	if err != nil {
		a.log().Error("Jibit Shahkar verification failed", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "سرویس احراز هویت در دسترس نیست. لطفا مجددا تلاش کنید."})
		return
	}

	if !result.Matched {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"matched": false,
			"message": "شماره موبایل با کد ملی مطابقت ندارد",
		})
		return
	}

	// Upsert user_verification record with phone verified status
	txIDs, _ := json.Marshal([]string{result.TransactionID})
	_, err = a.pool.Primary().ExecContext(ctx, `
		INSERT INTO user_verification (user_id, status, national_code, phone, shahkar_verified, provider, jibit_transaction_ids)
		VALUES ($1, 'pending', $2, $3, TRUE, 'jibit', $4)
		ON CONFLICT (user_id) DO UPDATE SET
			national_code = EXCLUDED.national_code,
			phone = EXCLUDED.phone,
			shahkar_verified = TRUE,
			provider = 'jibit',
			jibit_transaction_ids = user_verification.jibit_transaction_ids || $4::jsonb,
			status = CASE
				WHEN user_verification.status IN ('none', 'rejected') THEN 'pending'
				ELSE user_verification.status
			END
	`, userID, req.NationalCode, req.Phone, txIDs)
	if err != nil {
		a.log().Error("Failed to update user_verification after Shahkar", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Log to audit
	details, _ := json.Marshal(map[string]interface{}{
		"step":            "shahkar",
		"matched":         true,
		"mobile_operator": result.MobileOperator,
		"transaction_id":  result.TransactionID,
	})
	if _, err := a.pool.Primary().ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details) VALUES ($1, 'shahkar_verified', $2, $3)`,
		userID, userID, details); err != nil {
		a.log().Warn("Failed to write KYC audit log", zap.String("action", "shahkar_verified"), zap.String("user_id", userID), zap.Error(err))
	}

	a.log().Info("Shahkar verification passed",
		zap.String("user_id", userID),
		zap.String("operator", result.MobileOperator))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched":         true,
		"mobile_operator": result.MobileOperator,
		"next_step":       "verify-face",
		"message":         "شماره موبایل با موفقیت تایید شد",
	})
}

// handleKYCVerifyFace handles Step 2 of Jibit KYC: Biometric face verification with liveness.
// POST /api/user/kyc/verify-face (multipart/form-data with selfie_image)
func (a *App) handleKYCVerifyFace(w http.ResponseWriter, r *http.Request) {
	if a.jibitKYC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.KYCServiceUnavailable})
		return
	}

	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check that Shahkar step is complete
	var nationalCode sql.NullString
	var shahkarVerified bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT national_code, shahkar_verified FROM user_verification WHERE user_id = $1`,
		userID).Scan(&nationalCode, &shahkarVerified)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ابتدا مرحله تایید شماره موبایل را انجام دهید"})
			return
		}
		a.log().Error("Failed to check Shahkar status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !shahkarVerified || !nationalCode.Valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ابتدا مرحله تایید شماره موبایل را انجام دهید"})
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(15 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.FormParseFailed})
		return
	}
	defer r.MultipartForm.RemoveAll()

	selfieFile, selfieHeader, err := r.FormFile("selfie_image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.SelfieRequired})
		return
	}
	defer selfieFile.Close()

	// Validate file (magic byte + dimensions + size)
	if valResult, err := validateKYCFile(selfieHeader); err != nil {
		a.log().Warn("kyc selfie validation failed", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "تصویر سلفی نامعتبر است"})
		return
	} else {
		_ = valResult // validation only — result not used yet
	}

	// Read selfie bytes
	selfieBytes, err := io.ReadAll(selfieFile)
	if err != nil {
		a.log().Error("Failed to read selfie file", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Call Jibit Face Verification API
	result, err := a.jibitKYC.VerifyFace(ctx, nationalCode.String, selfieBytes)
	if err != nil {
		a.log().Error("Jibit face verification failed", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "سرویس احراز هویت در دسترس نیست. لطفا مجددا تلاش کنید."})
		return
	}

	// Update verification record with face results
	_, err = a.pool.Primary().ExecContext(ctx, `
		UPDATE user_verification
		SET face_verified = $1,
		    face_match_score = $2,
		    liveness_score = $3,
		    liveness_result = $4
		WHERE user_id = $5
	`, result.Matched && result.LivenessResult == "LIVE",
		result.MatchScore, result.LivenessScore, result.LivenessResult, userID)
	if err != nil {
		a.log().Error("Failed to update face verification results", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Log to audit
	details, _ := json.Marshal(map[string]interface{}{
		"step":            "face_verification",
		"matched":         result.Matched,
		"match_score":     result.MatchScore,
		"liveness_score":  result.LivenessScore,
		"liveness_result": result.LivenessResult,
	})
	if _, err := a.pool.Primary().ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details) VALUES ($1, 'face_verified', $2, $3)`,
		userID, userID, details); err != nil {
		a.log().Warn("Failed to write KYC audit log", zap.String("action", "face_verified"), zap.String("user_id", userID), zap.Error(err))
	}

	if !result.Matched || result.LivenessResult != "LIVE" {
		msg := "تایید هویت ناموفق بود."
		if result.LivenessResult == "FAKE" {
			msg = "تصویر ارسالی زنده تشخیص داده نشد. لطفا تصویر سلفی زنده ارسال کنید."
		} else if !result.Matched {
			msg = "تصویر چهره با اطلاعات ثبت شده مطابقت ندارد."
		}

		a.log().Info("Face verification failed",
			zap.String("user_id", userID),
			zap.Float64("match_score", result.MatchScore),
			zap.String("liveness", result.LivenessResult))

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"matched":         false,
			"liveness_result": result.LivenessResult,
			"message":         msg,
		})
		return
	}

	a.log().Info("Face verification passed",
		zap.String("user_id", userID),
		zap.Float64("match_score", result.MatchScore),
		zap.Float64("liveness_score", result.LivenessScore))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched":         true,
		"liveness_result": result.LivenessResult,
		"next_step":       "verify-card",
		"message":         "تایید هویت بایومتریک با موفقیت انجام شد",
	})
}

// handleKYCVerifyCard handles Step 3 of Jibit KYC: National card OCR verification.
// POST /api/user/kyc/verify-card (multipart/form-data with front_image)
func (a *App) handleKYCVerifyCard(w http.ResponseWriter, r *http.Request) {
	if a.jibitKYC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.KYCServiceUnavailable})
		return
	}

	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check that Shahkar and face verification are complete
	var nationalCode sql.NullString
	var shahkarVerified, faceVerified bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT national_code, shahkar_verified, face_verified FROM user_verification WHERE user_id = $1`,
		userID).Scan(&nationalCode, &shahkarVerified, &faceVerified)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ابتدا مراحل قبلی احراز هویت را انجام دهید"})
			return
		}
		a.log().Error("Failed to check verification status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !shahkarVerified {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ابتدا مرحله تایید شماره موبایل را انجام دهید"})
		return
	}
	if !faceVerified {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ابتدا مرحله تایید هویت بایومتریک را انجام دهید"})
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(15 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.FormParseFailed})
		return
	}
	defer r.MultipartForm.RemoveAll()

	frontFile, frontHeader, err := r.FormFile("front_image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.FrontImageRequired})
		return
	}
	defer frontFile.Close()

	// Validate file (magic byte + dimensions + size)
	if valResult, err := validateKYCFile(frontHeader); err != nil {
		a.log().Warn("kyc front image validation failed", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "تصویر مدرک نامعتبر است"})
		return
	} else {
		_ = valResult // validation only — result not used yet
	}

	// Read image bytes
	frontBytes, err := io.ReadAll(frontFile)
	if err != nil {
		a.log().Error("Failed to read card image file", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Call Jibit National Card OCR API
	result, err := a.jibitKYC.OCRNationalCard(ctx, frontBytes)
	if err != nil {
		a.log().Error("Jibit card OCR failed", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "سرویس احراز هویت در دسترس نیست. لطفا مجددا تلاش کنید."})
		return
	}

	// Cross-check OCR national code with previously verified national code
	ocrMatched := result.NationalCode == nationalCode.String

	// Log to audit
	details, _ := json.Marshal(map[string]interface{}{
		"step":              "card_ocr",
		"ocr_national_code": result.NationalCode,
		"expected_national": nationalCode.String,
		"matched":           ocrMatched,
		"first_name":        result.FirstName,
		"last_name":         result.LastName,
		"serial_number":     result.SerialNumber,
	})
	if _, err := a.pool.Primary().ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details) VALUES ($1, 'card_ocr_verified', $2, $3)`,
		userID, userID, details); err != nil {
		a.log().Warn("Failed to write KYC audit log", zap.String("action", "card_ocr_verified"), zap.String("user_id", userID), zap.Error(err))
	}

	if !ocrMatched {
		a.log().Info("Card OCR national code mismatch",
			zap.String("user_id", userID),
			zap.String("ocr_code", result.NationalCode),
			zap.String("expected", nationalCode.String))

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"matched": false,
			"message": "کد ملی روی کارت با اطلاعات ثبت شده مطابقت ندارد",
		})
		return
	}

	// All 3 steps passed — auto-approve
	expiresAt := time.Now().AddDate(1, 0, 0) // 1 year from now
	_, err = a.pool.Primary().ExecContext(ctx, `
		UPDATE user_verification
		SET card_ocr_verified = TRUE,
		    card_serial_number = $1,
		    first_name = $2,
		    last_name = $3,
		    status = 'verified',
		    verified_at = NOW(),
		    expires_at = $4
		WHERE user_id = $5
	`, result.SerialNumber, result.FirstName, result.LastName, expiresAt, userID)
	if err != nil {
		a.log().Error("Failed to auto-approve KYC after card OCR", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Log auto-approval to audit
	approvalDetails, _ := json.Marshal(map[string]interface{}{
		"step":       "auto_approved",
		"method":     "jibit_3step",
		"first_name": result.FirstName,
		"last_name":  result.LastName,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
	if _, err := a.pool.Primary().ExecContext(ctx,
		`INSERT INTO kyc_audit_log (user_id, action, actor_id, details) VALUES ($1, 'auto_approved', $2, $3)`,
		userID, userID, approvalDetails); err != nil {
		a.log().Warn("Failed to write KYC audit log", zap.String("action", "auto_approved"), zap.String("user_id", userID), zap.Error(err))
	}

	a.log().Info("KYC auto-approved via Jibit 3-step verification",
		zap.String("user_id", userID),
		zap.String("first_name", result.FirstName),
		zap.String("last_name", result.LastName))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched":    true,
		"verified":   true,
		"first_name": result.FirstName,
		"last_name":  result.LastName,
		"expires_at": expiresAt.Format(time.RFC3339),
		"message":    "احراز هویت با موفقیت انجام شد",
	})
}

// handleKYCSubmit handles KYC document submission with file uploads.
func (a *App) handleKYCSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Ensure KYC S3 storage is configured
	if a.kycStorage == nil {
		a.log().Error("KYC upload attempted but S3 KYC storage not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.FileStorageUnavailable,
		})
		return
	}

	// Check rate limit: max 3 KYC submissions per 24 hours
	var submissionCount int
	err := a.pool.Primary().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM kyc_audit_log
		WHERE user_id = $1
		AND action IN ('submitted', 'resubmitted')
		AND created_at > NOW() - INTERVAL '24 hours'
	`, userID).Scan(&submissionCount)
	if err != nil {
		a.log().Error("Failed to check KYC submission rate", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if submissionCount >= kycMaxSubmissionsPerDay {
		a.log().Info("KYC submission rate limit exceeded",
			zap.String("user_id", userID),
			zap.Int("submissions_24h", submissionCount))
		w.Header().Set("Retry-After", "86400") // 24 hours in seconds
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error":   "rate limit exceeded",
			"message": msg.KYCRateLimit,
		})
		return
	}

	// Check current KYC status first
	var currentStatus string
	err = a.pool.Primary().QueryRowContext(ctx, `
		SELECT status FROM user_verification WHERE user_id = $1
	`, userID).Scan(&currentStatus)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.log().Error("Failed to check KYC status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// If status is pending or under_review, don't allow resubmission
	if currentStatus == "pending" || currentStatus == "under_review" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "verification already in progress",
		})
		return
	}

	isResubmission := currentStatus == "rejected"

	// Parse multipart form with size limit (32MB total to account for base64 encoding overhead)
	if err := r.ParseMultipartForm(32 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.FormParseFailed,
		})
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Validate file fields: reject unexpected field names and limit total file count
	if err := validateKYCFormFileFields(r.MultipartForm); err != nil {
		a.log().Warn("kyc form file fields validation failed", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.InvalidForm,
		})
		return
	}

	// Extract and validate form fields
	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))
	dateOfBirth := strings.TrimSpace(r.FormValue("date_of_birth"))
	nationality := strings.ToUpper(strings.TrimSpace(r.FormValue("nationality")))
	// Accept both "address_line1" and "address_line_1" (frontend sends the latter)
	addressLine1 := strings.TrimSpace(r.FormValue("address_line1"))
	if addressLine1 == "" {
		addressLine1 = strings.TrimSpace(r.FormValue("address_line_1"))
	}
	city := strings.TrimSpace(r.FormValue("city"))
	country := strings.ToUpper(strings.TrimSpace(r.FormValue("country")))
	documentType := strings.ToLower(strings.TrimSpace(r.FormValue("document_type")))
	documentNumber := strings.TrimSpace(r.FormValue("document_number"))

	// Iranian manual KYC fields
	nationalCodeManual := strings.TrimSpace(r.FormValue("national_code_manual"))
	fatherName := strings.TrimSpace(r.FormValue("father_name"))
	birthCertificateNumber := strings.TrimSpace(r.FormValue("birth_certificate_number"))
	birthCertificateSerial := strings.TrimSpace(r.FormValue("birth_certificate_serial"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	province := strings.TrimSpace(r.FormValue("province"))
	postalCode := strings.TrimSpace(r.FormValue("postal_code"))

	// Determine if this is an Iranian manual KYC submission (has national_code_manual)
	isIranianKYC := nationalCodeManual != ""

	// For rejected resubmissions, load which fields were rejected to allow partial updates
	var rejectionFields []string
	isPartialResubmission := false
	if isResubmission {
		var rejectionFieldsJSON sql.NullString
		_ = a.pool.Primary().QueryRowContext(ctx,
			`SELECT rejection_fields::text FROM user_verification WHERE user_id = $1`,
			userID).Scan(&rejectionFieldsJSON)
		if rejectionFieldsJSON.Valid && rejectionFieldsJSON.String != "" && rejectionFieldsJSON.String != "[]" {
			_ = json.Unmarshal([]byte(rejectionFieldsJSON.String), &rejectionFields)
			if len(rejectionFields) > 0 {
				isPartialResubmission = true
			}
		}
	}

	// Validate required fields (skip validation for fields not in rejectionFields during partial resubmission)
	validationErrors := make(map[string]string)

	// Helper: check if a field is required in partial resubmission mode
	fieldRequired := func(fieldName string) bool {
		if !isPartialResubmission {
			return true
		}
		for _, f := range rejectionFields {
			if f == fieldName {
				return true
			}
		}
		return false
	}

	// Validate first_name
	if fieldRequired("first_name") {
		if firstName == "" {
			validationErrors["first_name"] = "first name is required"
		} else if isIranianKYC && !persianNameRegex.MatchString(firstName) {
			validationErrors["first_name"] = "نام باید فارسی و بین ۲ تا ۱۰۰ کاراکتر باشد"
		} else if !isIranianKYC && !nameRegex.MatchString(firstName) {
			validationErrors["first_name"] = "first name must be 2-100 characters with no numbers"
		}
	}

	// Validate last_name
	if fieldRequired("last_name") {
		if lastName == "" {
			validationErrors["last_name"] = "last name is required"
		} else if isIranianKYC && !persianNameRegex.MatchString(lastName) {
			validationErrors["last_name"] = "نام خانوادگی باید فارسی و بین ۲ تا ۱۰۰ کاراکتر باشد"
		} else if !isIranianKYC && !nameRegex.MatchString(lastName) {
			validationErrors["last_name"] = "last name must be 2-100 characters with no numbers"
		}
	}

	// Validate father_name (required for Iranian KYC)
	if isIranianKYC && fieldRequired("father_name") {
		if fatherName == "" {
			validationErrors["father_name"] = "نام پدر الزامی است"
		} else if !persianNameRegex.MatchString(fatherName) {
			validationErrors["father_name"] = "نام پدر باید فارسی و بین ۲ تا ۱۰۰ کاراکتر باشد"
		}
	}

	// Validate national_code_manual (Iranian national code with checksum)
	if isIranianKYC && fieldRequired("national_code") {
		if !validateIranianNationalCode(nationalCodeManual) {
			validationErrors["national_code_manual"] = "کد ملی نامعتبر است"
		}
	}

	// Validate date_of_birth (must be 18+)
	if fieldRequired("date_of_birth") {
		if dateOfBirth == "" {
			validationErrors["date_of_birth"] = "date of birth is required"
		} else {
			dob, err := time.Parse("2006-01-02", dateOfBirth)
			if err != nil {
				validationErrors["date_of_birth"] = "date of birth must be in YYYY-MM-DD format"
			} else {
				age := calculateAge(dob)
				if age < 18 {
					validationErrors["date_of_birth"] = "you must be at least 18 years old"
				}
			}
		}
	}

	// Validate nationality (not required for Iranian KYC — defaults to IR)
	if !isIranianKYC && fieldRequired("nationality") {
		if nationality == "" {
			validationErrors["nationality"] = "nationality is required"
		} else if !validCountryCodes[nationality] {
			validationErrors["nationality"] = "invalid nationality code (use ISO 3166-1 alpha-2)"
		}
	}

	// Validate address
	if fieldRequired("address") {
		if addressLine1 == "" {
			validationErrors["address_line1"] = "address is required"
		}
		if city == "" {
			validationErrors["city"] = "city is required"
		}
	}

	// Validate country (not required for Iranian KYC — defaults to IR)
	if !isIranianKYC {
		if country == "" {
			validationErrors["country"] = "country is required"
		} else if !validCountryCodes[country] {
			validationErrors["country"] = "invalid country code (use ISO 3166-1 alpha-2)"
		}
	}

	// Validate phone (Iranian format, optional for international)
	if isIranianKYC && phone != "" {
		if !iranPhoneRegex.MatchString(phone) {
			validationErrors["phone"] = "شماره موبایل نامعتبر است (مثال: 09123456789)"
		}
	}

	// Validate document_type
	if fieldRequired("document_number") {
		if documentType == "" {
			validationErrors["document_type"] = "document type is required"
		} else if !validDocumentTypes[documentType] {
			validationErrors["document_type"] = "invalid document type"
		}
	}

	// Validate document_number (flexible for Iranian documents)
	if fieldRequired("document_number") {
		if documentNumber == "" && !isIranianKYC {
			validationErrors["document_number"] = "document number is required"
		} else if documentNumber != "" && !isIranianKYC && !documentNumberRegex.MatchString(documentNumber) {
			validationErrors["document_number"] = "document number must be alphanumeric, 5-20 characters"
		}
	}

	// Validate birth_certificate fields
	if isIranianKYC && documentType == "birth_certificate" {
		if birthCertificateNumber != "" {
			if matched, _ := regexp.MatchString(`^\d{1,10}$`, birthCertificateNumber); !matched {
				validationErrors["birth_certificate_number"] = "شماره شناسنامه باید ۱ تا ۱۰ رقم باشد"
			}
		}
		if birthCertificateSerial != "" {
			if matched, _ := regexp.MatchString(`^[A-Za-z0-9\x{0600}-\x{06FF}]{1,30}$`, birthCertificateSerial); !matched {
				validationErrors["birth_certificate_serial"] = "سریال شناسنامه نامعتبر است"
			}
		}
	}

	// Validate file uploads
	var frontDetectedType, selfieDetectedType, backDetectedType, selfieWithDocDetectedType string

	frontFile, frontHeader, err := r.FormFile("front_image")
	if err != nil {
		if fieldRequired("front_image") {
			validationErrors["front_image"] = "تصویر روی مدرک الزامی است"
		}
	} else {
		defer frontFile.Close()
		if result, err := validateKYCFile(frontHeader); err != nil {
			a.log().Warn("kyc front image validation failed", zap.Error(err))
			validationErrors["front_image"] = safeKYCValidationMessage(err)
		} else {
			frontDetectedType = result.DetectedType
			if result.HasEXIF {
				a.log().Info("KYC upload contains EXIF metadata",
					zap.String("user_id", userID),
					zap.String("field", "front_image"))
			}
		}
	}

	// selfie (legacy field for international KYC)
	selfieFile, selfieHeader, err := r.FormFile("selfie")
	if err != nil {
		// selfie is only required for non-Iranian KYC when selfie_with_doc is not provided
		if !isIranianKYC && fieldRequired("selfie_with_doc") {
			validationErrors["selfie"] = "تصویر سلفی الزامی است"
		}
	} else {
		defer selfieFile.Close()
		if result, err := validateKYCFile(selfieHeader); err != nil {
			a.log().Warn("kyc selfie validation failed", zap.Error(err))
			validationErrors["selfie"] = safeKYCValidationMessage(err)
		} else {
			selfieDetectedType = result.DetectedType
			if result.HasEXIF {
				a.log().Info("KYC upload contains EXIF metadata",
					zap.String("user_id", userID),
					zap.String("field", "selfie"))
			}
		}
	}

	// selfie_with_doc (required for Iranian KYC)
	var selfieWithDocFile multipart.File
	var selfieWithDocHeader *multipart.FileHeader
	selfieWithDocFile, selfieWithDocHeader, err = r.FormFile("selfie_with_doc")
	if err != nil {
		if isIranianKYC && fieldRequired("selfie_with_doc") {
			validationErrors["selfie_with_doc"] = "سلفی با مدرک الزامی است"
		}
	} else {
		defer selfieWithDocFile.Close()
		if result, err := validateKYCFile(selfieWithDocHeader); err != nil {
			a.log().Warn("kyc selfie_with_doc validation failed", zap.Error(err))
			validationErrors["selfie_with_doc"] = safeKYCValidationMessage(err)
		} else {
			selfieWithDocDetectedType = result.DetectedType
			if result.HasEXIF {
				a.log().Info("KYC upload contains EXIF metadata",
					zap.String("user_id", userID),
					zap.String("field", "selfie_with_doc"))
			}
		}
	}

	// Back image is optional
	var backFile multipart.File
	var backHeader *multipart.FileHeader
	backFile, backHeader, err = r.FormFile("back_image")
	if err == nil {
		defer backFile.Close()
		if result, err := validateKYCFile(backHeader); err != nil {
			a.log().Warn("kyc back image validation failed", zap.Error(err))
			validationErrors["back_image"] = safeKYCValidationMessage(err)
		} else {
			backDetectedType = result.DetectedType
			if result.HasEXIF {
				a.log().Info("KYC upload contains EXIF metadata",
					zap.String("user_id", userID),
					zap.String("field", "back_image"))
			}
		}
	}

	// Return validation errors if any
	if len(validationErrors) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "validation failed",
			"details": validationErrors,
		})
		return
	}

	// Upload files to S3 (private KYC bucket)
	kycBucket := a.config.S3KYCBucket

	// Track uploaded S3 keys for cleanup on transaction failure
	var uploadedKYCKeys []string

	var frontImageURL string
	if frontFile != nil {
		frontObjectKey := fmt.Sprintf("kyc/%s/front_%s%s", userID, uuid.New().String(), kycExtFromContentType(frontDetectedType))
		if err := validateKYCS3Key(frontObjectKey); err != nil {
			a.log().Error("Invalid S3 key generated for front image", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		_, err = a.kycStorage.Upload(ctx, kycBucket, frontObjectKey, frontFile, frontHeader.Size, frontDetectedType)
		if err != nil {
			a.log().Error("Failed to upload front image to S3", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.SaveFrontFailed})
			return
		}
		frontImageURL = frontObjectKey
		uploadedKYCKeys = append(uploadedKYCKeys, frontObjectKey)
	}

	var selfieURL string
	if selfieFile != nil {
		selfieObjectKey := fmt.Sprintf("kyc/%s/selfie_%s%s", userID, uuid.New().String(), kycExtFromContentType(selfieDetectedType))
		_, err = a.kycStorage.Upload(ctx, kycBucket, selfieObjectKey, selfieFile, selfieHeader.Size, selfieDetectedType)
		if err != nil {
			a.log().Error("Failed to upload selfie to S3", zap.Error(err))
			for _, key := range uploadedKYCKeys {
				_ = a.kycStorage.Delete(ctx, kycBucket, key)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.SaveSelfieFailed})
			return
		}
		selfieURL = selfieObjectKey
		uploadedKYCKeys = append(uploadedKYCKeys, selfieObjectKey)
	}

	var selfieWithDocURL string
	if selfieWithDocFile != nil {
		selfieWithDocObjectKey := fmt.Sprintf("kyc/%s/selfie_with_doc_%s%s", userID, uuid.New().String(), kycExtFromContentType(selfieWithDocDetectedType))
		if err := validateKYCS3Key(selfieWithDocObjectKey); err != nil {
			a.log().Error("Invalid S3 key generated for selfie_with_doc", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		_, err = a.kycStorage.Upload(ctx, kycBucket, selfieWithDocObjectKey, selfieWithDocFile, selfieWithDocHeader.Size, selfieWithDocDetectedType)
		if err != nil {
			a.log().Error("Failed to upload selfie_with_doc to S3", zap.Error(err))
			for _, key := range uploadedKYCKeys {
				_ = a.kycStorage.Delete(ctx, kycBucket, key)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.SaveSelfieDocFailed})
			return
		}
		selfieWithDocURL = selfieWithDocObjectKey
		uploadedKYCKeys = append(uploadedKYCKeys, selfieWithDocObjectKey)
	}

	var backImageURL *string
	if backFile != nil {
		backObjectKey := fmt.Sprintf("kyc/%s/back_%s%s", userID, uuid.New().String(), kycExtFromContentType(backDetectedType))
		_, err = a.kycStorage.Upload(ctx, kycBucket, backObjectKey, backFile, backHeader.Size, backDetectedType)
		if err != nil {
			a.log().Error("Failed to upload back image to S3", zap.Error(err))
			for _, key := range uploadedKYCKeys {
				_ = a.kycStorage.Delete(ctx, kycBucket, key)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.SaveBackFailed})
			return
		}
		backImageURL = &backObjectKey
		uploadedKYCKeys = append(uploadedKYCKeys, backObjectKey)
	}

	// Parse date of birth for database
	dob, _ := time.Parse("2006-01-02", dateOfBirth)
	kycCommitted := false
	defer func() {
		if !kycCommitted {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cleanupCancel()
			for _, key := range uploadedKYCKeys {
				if delErr := a.kycStorage.Delete(cleanupCtx, kycBucket, key); delErr != nil {
					a.log().Warn("Failed to clean up orphaned KYC file",
						zap.Error(delErr), zap.String("key", key))
				}
			}
		}
	}()

	// Begin transaction
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()

	// Set defaults for Iranian KYC
	if isIranianKYC {
		if nationality == "" {
			nationality = "IR"
		}
		if country == "" {
			country = "IR"
		}
	}

	// Convert province to state column
	state := province

	// Prepare nullable fields
	var nationalCodeManualDB, fatherNameDB, birthCertNumberDB, birthCertSerialDB, phoneDB sql.NullString
	if nationalCodeManual != "" {
		nationalCodeManualDB = sql.NullString{String: nationalCodeManual, Valid: true}
	}
	if fatherName != "" {
		fatherNameDB = sql.NullString{String: fatherName, Valid: true}
	}
	if birthCertificateNumber != "" {
		birthCertNumberDB = sql.NullString{String: birthCertificateNumber, Valid: true}
	}
	if birthCertificateSerial != "" {
		birthCertSerialDB = sql.NullString{String: birthCertificateSerial, Valid: true}
	}
	if phone != "" {
		phoneDB = sql.NullString{String: phone, Valid: true}
	}

	// Insert or update user_verification record
	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_verification (
			user_id, status, first_name, last_name, date_of_birth,
			nationality, address_line1, city, country, state, postal_code, phone,
			national_code_manual, father_name, birth_certificate_number, birth_certificate_serial,
			rejection_fields, rejection_field_messages, rejection_reason
		) VALUES ($1, 'pending', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, '[]', '{}', NULL)
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'pending',
			first_name = COALESCE(NULLIF(EXCLUDED.first_name, ''), user_verification.first_name),
			last_name = COALESCE(NULLIF(EXCLUDED.last_name, ''), user_verification.last_name),
			date_of_birth = COALESCE(EXCLUDED.date_of_birth, user_verification.date_of_birth),
			nationality = COALESCE(NULLIF(EXCLUDED.nationality, ''), user_verification.nationality),
			address_line1 = COALESCE(NULLIF(EXCLUDED.address_line1, ''), user_verification.address_line1),
			city = COALESCE(NULLIF(EXCLUDED.city, ''), user_verification.city),
			country = COALESCE(NULLIF(EXCLUDED.country, ''), user_verification.country),
			state = COALESCE(EXCLUDED.state, user_verification.state),
			postal_code = COALESCE(EXCLUDED.postal_code, user_verification.postal_code),
			phone = COALESCE(EXCLUDED.phone, user_verification.phone),
			national_code_manual = COALESCE(EXCLUDED.national_code_manual, user_verification.national_code_manual),
			father_name = COALESCE(EXCLUDED.father_name, user_verification.father_name),
			birth_certificate_number = COALESCE(EXCLUDED.birth_certificate_number, user_verification.birth_certificate_number),
			birth_certificate_serial = COALESCE(EXCLUDED.birth_certificate_serial, user_verification.birth_certificate_serial),
			rejection_fields = '[]',
			rejection_field_messages = '{}',
			rejection_reason = NULL,
			updated_at = NOW()
	`, userID, firstName, lastName, dob, nationality, addressLine1, city, country,
		sql.NullString{String: state, Valid: state != ""},
		sql.NullString{String: postalCode, Valid: postalCode != ""},
		phoneDB, nationalCodeManualDB, fatherNameDB, birthCertNumberDB, birthCertSerialDB)
	if err != nil {
		a.log().Error("Failed to upsert user_verification", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Insert kyc_documents record
	documentID := uuid.New().String()
	var selfieURLDB, selfieWithDocURLDB sql.NullString
	if selfieURL != "" {
		selfieURLDB = sql.NullString{String: selfieURL, Valid: true}
	}
	if selfieWithDocURL != "" {
		selfieWithDocURLDB = sql.NullString{String: selfieWithDocURL, Valid: true}
	}
	var frontImageURLDB sql.NullString
	if frontImageURL != "" {
		frontImageURLDB = sql.NullString{String: frontImageURL, Valid: true}
	}
	// For Iranian KYC, use national_code_manual as document_number if not explicitly set
	if isIranianKYC && documentNumber == "" && documentType == "national_id" {
		documentNumber = nationalCodeManual
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO kyc_documents (
			id, user_id, document_type, document_number, issuing_country,
			front_image_url, back_image_url, selfie_url, selfie_with_doc_url, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
	`, documentID, userID, documentType, documentNumber, country,
		frontImageURLDB, backImageURL, selfieURLDB, selfieWithDocURLDB)
	if err != nil {
		a.log().Error("Failed to insert kyc_documents", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Insert kyc_audit_log entry
	auditAction := "submitted"
	if isResubmission {
		auditAction = "resubmitted"
	}

	auditDetails, _ := json.Marshal(map[string]interface{}{
		"document_id":   documentID,
		"document_type": documentType,
		"ip_address":    getClientIP(r),
		"user_agent":    r.Header.Get("User-Agent"),
	})

	_, err = tx.ExecContext(ctx, `
		INSERT INTO kyc_audit_log (user_id, action, actor_id, details)
		VALUES ($1, $2, $3, $4)
	`, userID, auditAction, userID, auditDetails)
	if err != nil {
		a.log().Error("Failed to insert kyc_audit_log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	kycCommitted = true

	a.log().Info("KYC submission received",
		zap.String("user_id", userID),
		zap.String("document_id", documentID),
		zap.String("action", auditAction))

	writeJSON(w, http.StatusCreated, KYCSubmitResponse{
		Message:             "KYC documents submitted successfully",
		Status:              "pending",
		EstimatedReviewTime: "1-3 business days",
	})
}

// calculateAge calculates age in years from a date of birth.
func calculateAge(dob time.Time) int {
	return calculateAgeAt(dob, time.Now())
}

// calculateAgeAt calculates age in years from a date of birth relative to a reference time.
func calculateAgeAt(dob, now time.Time) int {
	years := now.Year() - dob.Year()

	dobMonth, dobDay := dob.Month(), dob.Day()

	// Handle Feb 29 birthdays: in non-leap years, treat the birthday as Feb 28.
	// This follows the convention used in most jurisdictions for age verification,
	// so a person born on Feb 29 turns their age on Feb 28 in non-leap years.
	if dobMonth == time.February && dobDay == 29 && !isLeapYear(now.Year()) {
		dobDay = 28
	}

	if now.Month() < dobMonth || (now.Month() == dobMonth && now.Day() < dobDay) {
		years--
	}
	return years
}

// isLeapYear returns true if the given year is a leap year.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// Image dimension constraints for KYC uploads.
const (
	kycMinImageDimension = 200
	kycMaxImageDimension = 8000
)

// kycFileValidationResult holds the result of KYC file validation.
type kycFileValidationResult struct {
	DetectedType string
	HasEXIF      bool
}

// validateKYCFile validates a KYC file upload and returns the detected content type based on magic bytes.
func validateKYCFile(header *multipart.FileHeader) (kycFileValidationResult, error) {
	// Check file size - reject files larger than 10MB
	if header.Size > kycMaxFileSize {
		sizeMB := float64(header.Size) / (1024 * 1024)
		return kycFileValidationResult{}, fmt.Errorf("file size (%.1fMB) exceeds the 10MB limit. Please compress or resize your image and try again", sizeMB)
	}

	// Check content type header (first pass)
	contentType := header.Header.Get("Content-Type")
	if !kycAllowedMimeTypes[contentType] {
		return kycFileValidationResult{}, fmt.Errorf("invalid file type '%s'. Allowed formats: JPEG, PNG, or WebP", contentType)
	}

	// Magic-byte validation (P0-4): verify actual file content, not just Content-Type header
	file, err := header.Open()
	if err != nil {
		return kycFileValidationResult{}, fmt.Errorf("failed to open file for validation: %w", err)
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return kycFileValidationResult{}, fmt.Errorf("failed to read file header: %w", err)
	}

	detectedType := http.DetectContentType(buf[:n])
	if !kycAllowedMimeTypes[detectedType] {
		return kycFileValidationResult{}, fmt.Errorf("file content does not match allowed types (detected: %s). Allowed formats: JPEG, PNG, or WebP", detectedType)
	}

	// Seek back to start for dimension validation
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return kycFileValidationResult{}, fmt.Errorf("failed to reset file position: %w", err)
		}
	}

	// Image dimension validation: decode header only (not full image) to check dimensions
	if detectedType == "image/jpeg" || detectedType == "image/png" {
		cfg, _, decErr := image.DecodeConfig(file)
		if decErr != nil {
			return kycFileValidationResult{}, fmt.Errorf("failed to read image dimensions: %w", decErr)
		}
		if cfg.Width < kycMinImageDimension || cfg.Height < kycMinImageDimension {
			return kycFileValidationResult{}, fmt.Errorf("image is too small (%dx%d). Minimum dimensions: %dx%d pixels",
				cfg.Width, cfg.Height, kycMinImageDimension, kycMinImageDimension)
		}
		if cfg.Width > kycMaxImageDimension || cfg.Height > kycMaxImageDimension {
			return kycFileValidationResult{}, fmt.Errorf("image is too large (%dx%d). Maximum dimensions: %dx%d pixels",
				cfg.Width, cfg.Height, kycMaxImageDimension, kycMaxImageDimension)
		}
	}
	// Note: WebP dimension check is skipped — golang.org/x/image/webp not in deps.
	// WebP files still pass magic-byte and size checks.

	// Seek back to start for subsequent reading
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return kycFileValidationResult{}, fmt.Errorf("failed to reset file position: %w", err)
		}
	}

	// EXIF detection: check presence of EXIF data for admin awareness
	// For JPEG, check for APP1 marker (0xFFE1) which contains EXIF
	exifFound := false
	if detectedType == "image/jpeg" && n >= 4 {
		if buf[0] == 0xFF && buf[1] == 0xD8 { // SOI marker
			for i := 2; i < n-1; i++ {
				if buf[i] == 0xFF && buf[i+1] == 0xE1 {
					// APP1 segment found — likely contains EXIF metadata
					exifFound = true
					break
				}
				if buf[i] == 0xFF && buf[i+1] == 0xDA {
					// SOS marker — no more metadata segments
					break
				}
			}
		}
	}

	return kycFileValidationResult{DetectedType: detectedType, HasEXIF: exifFound}, nil
}

// safeKYCValidationMessage converts a validateKYCFile error into a user-safe Persian message.
// Internal errors (file I/O, decode failures) are hidden; validation errors (size, format, dimensions) are translated.
func safeKYCValidationMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "exceeds the 10MB limit"):
		return "حجم فایل بیش از ۱۰ مگابایت است. لطفاً تصویر را فشرده یا تغییر اندازه دهید"
	case strings.Contains(msg, "invalid file type"):
		return "فرمت فایل نامعتبر است. فرمت‌های مجاز: JPEG، PNG یا WebP"
	case strings.Contains(msg, "does not match allowed types"):
		return "محتوای فایل با فرمت‌های مجاز مطابقت ندارد. فرمت‌های مجاز: JPEG، PNG یا WebP"
	case strings.Contains(msg, "too small"):
		return "تصویر خیلی کوچک است. حداقل ابعاد مجاز رعایت نشده"
	case strings.Contains(msg, "too large"):
		return "ابعاد تصویر خیلی بزرگ است. حداکثر ابعاد مجاز رعایت نشده"
	default:
		// Internal errors (failed to open, failed to read, etc.) — don't expose
		return "خطا در پردازش فایل. لطفاً فایل دیگری آپلود کنید"
	}
}

// kycS3KeyRegex validates generated S3 object keys for KYC uploads.
var kycS3KeyRegex = regexp.MustCompile(`^kyc/[a-f0-9-]+/(front|back|selfie|selfie_with_doc)_[a-f0-9-]+\.(jpg|png|webp)$`)

// validateKYCS3Key ensures the generated S3 key is safe (no path traversal, matches expected pattern).
func validateKYCS3Key(key string) error {
	if strings.Contains(key, "..") || strings.Contains(key, "\\") {
		return fmt.Errorf("invalid S3 key: path traversal detected")
	}
	if !kycS3KeyRegex.MatchString(key) {
		return fmt.Errorf("invalid S3 key format: %s", key)
	}
	return nil
}

// kycAllowedFormFileFields defines the set of permitted multipart file field names for KYC submission.
var kycAllowedFormFileFields = map[string]bool{
	"front_image":     true,
	"back_image":      true,
	"selfie":          true,
	"selfie_with_doc": true,
}

// validateKYCFormFileFields checks that the multipart form only contains expected file fields
// and that the total number of file parts does not exceed the limit.
func validateKYCFormFileFields(form *multipart.Form) error {
	if form == nil || form.File == nil {
		return nil
	}
	totalFiles := 0
	for fieldName, files := range form.File {
		if !kycAllowedFormFileFields[fieldName] {
			return fmt.Errorf("unexpected file field: %s", fieldName)
		}
		totalFiles += len(files)
	}
	if totalFiles > 4 {
		return fmt.Errorf("too many files uploaded (max 4, got %d)", totalFiles)
	}
	return nil
}

// kycExtFromContentType returns the file extension for a given content type.
func kycExtFromContentType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// ============================================================================
// Affiliate / Referral Handlers
// ============================================================================

// handleValidateReferral validates a referral code and returns referrer info.
// GET /api/user/referral/validate?code=ABC123
