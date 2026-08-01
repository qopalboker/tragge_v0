package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"go.uber.org/zap"
)

var (
	errVerificationCooldown  = errors.New("verification resend cooldown is active")
	errAlreadyVerified       = errors.New("destination is already verified")
	errNoVerifiedDestination = errors.New("verification destination is unavailable")
	errNoActiveCode          = errors.New("verification code is unavailable")
	errWrongCode             = errors.New("verification code is invalid")
	errCodeExhausted         = errors.New("verification attempts are exhausted")
)

type SendVerificationRequest struct {
	Method string `json:"method"`
}

type SendVerificationResponse struct {
	Message               string `json:"message"`
	DestinationMasked     string `json:"destination_masked"`
	ExpiresInSeconds      int    `json:"expires_in_seconds"`
	ResendCooldownSeconds int    `json:"resend_cooldown_seconds"`
}

type VerifyCodeRequest struct {
	Code string `json:"code"`
}

type VerifyCodeResponse struct {
	Message string `json:"message"`
}

type verificationIssueResult struct {
	MaskedDestination string
}

func (a *App) handleSendVerification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	var req SendVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}
	method := strings.ToLower(strings.TrimSpace(req.Method))
	if method != "sms" && method != "email" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_verification_method"})
		return
	}

	result, err := a.issueVerificationCode(ctx, userID, method)
	if err != nil {
		switch {
		case errors.Is(err, errVerificationCooldown):
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": "rate_limit_exceeded", "retry_after_seconds": int(securityCodeCooldown.Seconds()),
			})
		case errors.Is(err, errAlreadyVerified), errors.Is(err, errNoVerifiedDestination), errors.Is(err, errUnsupportedCountry):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "verification_unavailable"})
		case errors.Is(err, errSecurityCodeUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "delivery_unavailable"})
		default:
			a.log().Error("Verification issuance failed", zap.String("user_id", userID), zap.String("method", method))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		}
		return
	}
	writeJSON(w, http.StatusOK, SendVerificationResponse{
		Message:               "verification_code_sent",
		DestinationMasked:     result.MaskedDestination,
		ExpiresInSeconds:      int(securityCodeTTL.Seconds()),
		ResendCooldownSeconds: int(securityCodeCooldown.Seconds()),
	})
}

// issueVerificationCode creates an unusable reservation first, delivers
// synchronously, then activates exactly one code. A failed provider call never
// leaves its code usable. Failed reservations count toward the 60-second abuse
// cooldown but remain expired.
func (a *App) issueVerificationCode(
	ctx context.Context,
	userID string,
	method string,
) (verificationIssueResult, error) {
	if a.codeHasher == nil {
		return verificationIssueResult{}, errSecurityCodeUnavailable
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return verificationIssueResult{}, err
	}
	defer tx.Rollback()

	var email, phone, country sql.NullString
	var emailVerified, phoneVerified bool
	var preferredLang string
	err = tx.QueryRowContext(ctx,
		`SELECT email, phone, country, COALESCE(email_verified, false),
		        COALESCE(phone_verified, false), COALESCE(preferred_lang, 'fa')
		   FROM users WHERE id = $1 FOR UPDATE`,
		userID,
	).Scan(&email, &phone, &country, &emailVerified, &phoneVerified, &preferredLang)
	if err != nil {
		return verificationIssueResult{}, err
	}

	var destination, masked, purpose string
	switch method {
	case "email":
		if emailVerified {
			return verificationIssueResult{}, errAlreadyVerified
		}
		if !email.Valid || strings.TrimSpace(email.String) == "" {
			return verificationIssueResult{}, errNoVerifiedDestination
		}
		if !country.Valid {
			return verificationIssueResult{}, errUnsupportedCountry
		}
		if _, err := normalizeSupportedCountry(country.String); err != nil {
			return verificationIssueResult{}, err
		}
		destination, masked, purpose = email.String, maskEmail(email.String), securityCodePurposeEmailVerification
	case "sms":
		if phoneVerified {
			return verificationIssueResult{}, errAlreadyVerified
		}
		if !phone.Valid || strings.TrimSpace(phone.String) == "" {
			return verificationIssueResult{}, errNoVerifiedDestination
		}
		destination, masked, purpose = phone.String, maskPhone(phone.String), securityCodePurposePhoneVerification
	default:
		return verificationIssueResult{}, errNoVerifiedDestination
	}

	var recent bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM verification_codes
		    WHERE user_id = $1 AND method = $2
		      AND created_at > NOW() - INTERVAL '60 seconds'
		 )`,
		userID, method,
	).Scan(&recent); err != nil {
		return verificationIssueResult{}, err
	}
	if recent {
		return verificationIssueResult{}, errVerificationCooldown
	}

	code, err := generateVerificationCode()
	if err != nil {
		return verificationIssueResult{}, err
	}
	now := a.securityCodeNow()
	digest := a.codeHasher.Digest(purpose, userID, destination, method, "", code)
	var codeID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO verification_codes
		    (user_id, code_hash, method, destination, expires_at, attempts, max_attempts)
		 VALUES ($1, $2, $3, $4, $5, 0, $6)
		 RETURNING id`,
		userID, digest, method, destination, now, securityCodeMaxAttempts,
	).Scan(&codeID)
	if err != nil {
		return verificationIssueResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return verificationIssueResult{}, err
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.deliverVerificationCode(sendCtx, method, destination, country.String, preferredLang, code); err != nil {
		_, _ = a.pool.Primary().ExecContext(ctx,
			`UPDATE verification_codes SET expires_at = NOW(), verified_at = NOW() WHERE id = $1`,
			codeID,
		)
		return verificationIssueResult{}, errSecurityCodeUnavailable
	}

	activateTx, err := a.pool.Begin(ctx)
	if err != nil {
		return verificationIssueResult{}, err
	}
	defer activateTx.Rollback()
	activationTime := a.securityCodeNow()
	if _, err := activateTx.ExecContext(ctx,
		`UPDATE verification_codes SET expires_at = $2
		  WHERE user_id = $1 AND verified_at IS NULL AND expires_at > $2`,
		userID, activationTime,
	); err != nil {
		return verificationIssueResult{}, err
	}
	result, err := activateTx.ExecContext(ctx,
		`UPDATE verification_codes
		    SET expires_at = $3, attempts = 0, max_attempts = $2
		  WHERE id = $1 AND verified_at IS NULL`,
		codeID, securityCodeMaxAttempts, activationTime.Add(securityCodeTTL),
	)
	if err != nil {
		return verificationIssueResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return verificationIssueResult{}, errSecurityCodeUnavailable
	}
	if err := activateTx.Commit(); err != nil {
		return verificationIssueResult{}, err
	}

	a.log().Info("Verification code delivery accepted",
		zap.String("user_id", userID), zap.String("method", method), zap.String("destination_masked", masked))
	return verificationIssueResult{MaskedDestination: masked}, nil
}

func (a *App) deliverVerificationCode(
	ctx context.Context,
	method, destination, country, lang, code string,
) error {
	switch method {
	case "email":
		if a.securityEmail == nil {
			return errSecurityCodeUnavailable
		}
		message, err := notification.RenderSecurityCodeEmail(securityCodePurposeEmailVerification, code, lang)
		if err != nil {
			return errSecurityCodeUnavailable
		}
		message.To = destination
		return a.securityEmail.Send(ctx, country, message)
	case "sms":
		if a.otpService == nil {
			return errSecurityCodeUnavailable
		}
		if err := a.otpService.Provider().SendOTP(ctx, destination, code); err != nil {
			return errSecurityCodeUnavailable
		}
		return nil
	default:
		return errSecurityCodeUnavailable
	}
}

func (a *App) handleVerifyCode(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	var req VerifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}
	if err := a.verifyVerificationCode(r.Context(), userID, "", strings.TrimSpace(req.Code)); err != nil {
		switch {
		case errors.Is(err, errCodeExhausted):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "code_exhausted"})
		case errors.Is(err, errNoActiveCode):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_valid_code"})
		case errors.Is(err, errWrongCode):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_code"})
		default:
			a.log().Error("Verification consume failed", zap.String("user_id", userID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		}
		return
	}
	writeJSON(w, http.StatusOK, VerifyCodeResponse{Message: "verified"})
}

func (a *App) verifyVerificationCode(ctx context.Context, userID, expectedMethod, code string) error {
	if len(code) != 6 || !isDigitsOnly(code) || a.codeHasher == nil {
		return errWrongCode
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `SELECT id, code_hash, method, destination, attempts, max_attempts
	            FROM verification_codes
	           WHERE user_id = $1 AND expires_at > NOW() AND verified_at IS NULL`
	args := []any{userID}
	if expectedMethod != "" {
		query += ` AND method = $2`
		args = append(args, expectedMethod)
	}
	query += ` ORDER BY created_at DESC LIMIT 1 FOR UPDATE`

	var codeID, stored, method, destination string
	var attempts, maxAttempts int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(
		&codeID, &stored, &method, &destination, &attempts, &maxAttempts,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNoActiveCode
		}
		return err
	}
	if attempts >= securityCodeMaxAttempts || maxAttempts != securityCodeMaxAttempts {
		return errCodeExhausted
	}
	purpose := securityCodePurposeEmailVerification
	if method == "sms" {
		purpose = securityCodePurposePhoneVerification
	}
	if !a.codeHasher.Matches(stored, purpose, userID, destination, method, "", code) {
		result, err := tx.ExecContext(ctx,
			`UPDATE verification_codes
			    SET attempts = attempts + 1,
			        expires_at = CASE WHEN attempts + 1 >= $2 THEN NOW() ELSE expires_at END
			  WHERE id = $1`,
			codeID, securityCodeMaxAttempts,
		)
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errSecurityCodeUnavailable
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if attempts+1 >= securityCodeMaxAttempts {
			return errCodeExhausted
		}
		return errWrongCode
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE verification_codes SET verified_at = NOW() WHERE id = $1 AND verified_at IS NULL`,
		codeID,
	)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return errNoActiveCode
	}
	switch method {
	case "email":
		_, err = tx.ExecContext(ctx,
			`UPDATE users SET email_verified = TRUE, email_verified_at = NOW() WHERE id = $1`,
			userID,
		)
	case "sms":
		_, err = tx.ExecContext(ctx,
			`UPDATE users SET phone_verified = TRUE WHERE id = $1`,
			userID,
		)
	default:
		return fmt.Errorf("unsupported stored verification method")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func availableVerificationMethods(email, phone sql.NullString, emailVerified, phoneVerified bool) []string {
	var methods []string
	if email.Valid && email.String != "" && !emailVerified {
		methods = append(methods, "email")
	}
	if phone.Valid && phone.String != "" && !phoneVerified {
		methods = append(methods, "sms")
	}
	return methods
}
