package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	passwordSetTokenTTL      = 10 * time.Minute
	passwordResetDailyLimit  = 5
	passwordResetIPLimit     = 10
	passwordResetIPWindow    = time.Hour
	passwordResetDailyWindow = 24 * time.Hour
)

type passwordResetSession struct {
	UserID      string `json:"user_id"`
	CodeID      string `json:"code_id"`
	Channel     string `json:"channel"`
	Destination string `json:"destination"`
}

func pwResetSessionKey(token string) string {
	return "auth:user:password-reset:session:" + token
}
func pwResetSetTokenKey(token string) string {
	return "auth:user:password-reset:set-token:" + token
}
func pwResetDailyKey(userID string) string {
	return "auth:user:password-reset:daily:" + userID
}
func pwResetIPKey(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return "auth:user:password-reset:ip:" + hex.EncodeToString(sum[:])
}

var securityRateLimitScript = goredis.NewScript(`
local value = redis.call("INCR", KEYS[1])
if value == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return value
`)

var exchangeResetSessionScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
  return 0
end
if redis.call("EXISTS", KEYS[2]) == 1 then
  return 0
end
redis.call("DEL", KEYS[1])
redis.call("SET", KEYS[2], ARGV[2], "EX", ARGV[3])
return 1
`)

var consumePasswordSetTokenScript = goredis.NewScript(`
local userID = redis.call("GET", KEYS[1])
if not userID then
  return false
end
redis.call("DEL", KEYS[1])
return userID
`)

func (a *App) incrementSecurityRateLimit(ctx context.Context, key string, window time.Duration) (int64, error) {
	if a.redis == nil || a.redis.Client() == nil {
		return 0, errSecurityCodeUnavailable
	}
	value, err := securityRateLimitScript.Run(
		ctx, a.redis.Client(), []string{key}, int(window.Seconds()),
	).Int64()
	if err != nil {
		return 0, errSecurityCodeUnavailable
	}
	return value, nil
}

// handleForgotPasswordRequest always returns the same external response shape.
// Provider, storage, user existence, verified-channel, and country failures all
// fail closed internally without becoming an account-enumeration oracle.
func (a *App) handleForgotPasswordRequest(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}
	if a.config.ARCaptchaSiteKey != "" && a.config.ARCaptchaSecretKey != "" {
		if req.CaptchaToken == "" || len(req.CaptchaToken) > 4096 {
			writeErrorJSON(w, r, http.StatusBadRequest, msg.CaptchaFailed)
			return
		}
		valid, err := verifyCaptcha(
			r.Context(), a.config.ARCaptchaSiteKey, a.config.ARCaptchaSecretKey, req.CaptchaToken,
		)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.CaptchaFailed})
			return
		}
		if !valid {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.CaptchaFailed})
			return
		}
	}

	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.IdentifierRequired})
		return
	}
	ctx := r.Context()
	dummyToken := uuid.NewString()
	generic := func(token string) map[string]any {
		return map[string]any{"message": msg.PasswordResetCodeSent, "reset_token": token}
	}

	if count, err := a.incrementSecurityRateLimit(
		ctx, pwResetIPKey(getClientIP(r)), passwordResetIPWindow,
	); err != nil || count > passwordResetIPLimit {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	var query string
	var arg any
	switch {
	case strings.Contains(identifier, "@"):
		v := validation.New()
		identifier = v.Email("identifier", identifier)
		if v.HasErrors() {
			writeJSON(w, http.StatusOK, generic(dummyToken))
			return
		}
		query = `SELECT id, COALESCE(email, ''), COALESCE(phone, ''),
		                COALESCE(email_verified, false), COALESCE(phone_verified, false),
		                COALESCE(preferred_lang, 'fa'), country
		           FROM users WHERE email = $1`
		arg = validation.SanitizeEmail(identifier)
	case strings.HasPrefix(identifier, "09"), strings.HasPrefix(identifier, "+98"):
		query = `SELECT id, COALESCE(email, ''), COALESCE(phone, ''),
		                COALESCE(email_verified, false), COALESCE(phone_verified, false),
		                COALESCE(preferred_lang, 'fa'), country
		           FROM users WHERE phone = $1`
		arg = normalizePhoneForLookup(identifier)
	default:
		query = `SELECT id, COALESCE(email, ''), COALESCE(phone, ''),
		                COALESCE(email_verified, false), COALESCE(phone_verified, false),
		                COALESCE(preferred_lang, 'fa'), country
		           FROM users WHERE LOWER(username) = $1`
		arg = strings.ToLower(identifier)
	}

	var userID, email, phone, lang string
	var emailVerified, phoneVerified bool
	var country sql.NullString
	err := a.pool.Primary().QueryRowContext(ctx, query, arg).Scan(
		&userID, &email, &phone, &emailVerified, &phoneVerified, &lang, &country,
	)
	if err != nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	var channel, destination, countryCode string
	if phoneVerified && phone != "" {
		channel, destination = "sms", phone
	} else if emailVerified && email != "" && country.Valid {
		normalized, err := normalizeSupportedCountry(country.String)
		if err != nil {
			writeJSON(w, http.StatusOK, generic(dummyToken))
			return
		}
		channel, destination, countryCode = "email", email, normalized
	} else {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	if count, err := a.incrementSecurityRateLimit(
		ctx, pwResetDailyKey(userID), passwordResetDailyWindow,
	); err != nil || count > passwordResetDailyLimit {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	resetToken := uuid.NewString()
	code, err := generateVerificationCode()
	if err != nil || a.codeHasher == nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	digest := a.codeHasher.Digest(
		securityCodePurposePasswordReset, userID, destination, channel, resetToken, code,
	)

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	defer tx.Rollback()
	var recent bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM password_reset_codes
		    WHERE user_id = $1 AND created_at > NOW() - INTERVAL '60 seconds'
		 )`,
		userID,
	).Scan(&recent); err != nil || recent {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	var codeID string
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO password_reset_codes
		    (user_id, code_hash, channel, destination, expires_at, attempts)
		 VALUES ($1, $2, $3, $4, NOW(), 0)
		 RETURNING id`,
		userID, digest, channel, destination,
	).Scan(&codeID); err != nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	session := passwordResetSession{
		UserID: userID, CodeID: codeID, Channel: channel, Destination: destination,
	}
	sessionJSON, err := json.Marshal(session)
	if err != nil || a.redis == nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	sessionKey := pwResetSessionKey(resetToken)
	if err := a.redis.Client().Set(ctx, sessionKey, sessionJSON, securityCodeTTL).Err(); err != nil {
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	sendErr := a.deliverPasswordResetCode(sendCtx, channel, destination, countryCode, lang, code)
	cancel()
	if sendErr != nil {
		_ = a.redis.Client().Del(ctx, sessionKey).Err()
		_, _ = a.pool.Primary().ExecContext(ctx,
			`UPDATE password_reset_codes SET used_at = NOW(), expires_at = NOW() WHERE id = $1`,
			codeID,
		)
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	activateTx, err := a.pool.Begin(ctx)
	if err != nil {
		_ = a.redis.Client().Del(ctx, sessionKey).Err()
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	defer activateTx.Rollback()
	if _, err := activateTx.ExecContext(ctx,
		`UPDATE password_reset_codes SET used_at = NOW()
		  WHERE user_id = $1 AND id <> $2 AND used_at IS NULL`,
		userID, codeID,
	); err != nil {
		_ = a.redis.Client().Del(ctx, sessionKey).Err()
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	result, err := activateTx.ExecContext(ctx,
		`UPDATE password_reset_codes
		    SET expires_at = NOW() + INTERVAL '10 minutes', attempts = 0
		  WHERE id = $1 AND used_at IS NULL`,
		codeID,
	)
	if err != nil {
		_ = a.redis.Client().Del(ctx, sessionKey).Err()
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 || activateTx.Commit() != nil {
		_ = a.redis.Client().Del(ctx, sessionKey).Err()
		writeJSON(w, http.StatusOK, generic(dummyToken))
		return
	}

	a.log().Info("Password reset delivery accepted", zap.String("channel", channel))
	writeJSON(w, http.StatusOK, generic(resetToken))
}

func (a *App) deliverPasswordResetCode(
	ctx context.Context,
	channel, destination, country, lang, code string,
) error {
	switch channel {
	case "sms":
		if a.otpService == nil {
			return errSecurityCodeUnavailable
		}
		if err := a.otpService.Provider().SendOTP(ctx, destination, code); err != nil {
			return errSecurityCodeUnavailable
		}
		return nil
	case "email":
		if a.securityEmail == nil {
			return errSecurityCodeUnavailable
		}
		message, err := notification.RenderSecurityCodeEmail(
			securityCodePurposePasswordReset, code, lang,
		)
		if err != nil {
			return errSecurityCodeUnavailable
		}
		message.To = destination
		return a.securityEmail.Send(ctx, country, message)
	default:
		return errSecurityCodeUnavailable
	}
}

func (a *App) handleForgotPasswordVerify(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}
	resetToken := strings.TrimSpace(req.ResetToken)
	code := strings.TrimSpace(req.Code)
	if resetToken == "" || len(code) != 6 || !isDigitsOnly(code) ||
		a.redis == nil || a.codeHasher == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetCodeInvalid})
		return
	}
	ctx := r.Context()
	sessionKey := pwResetSessionKey(resetToken)
	rawSession, err := a.redis.Client().Get(ctx, sessionKey).Bytes()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetSessionExpired})
		return
	}
	var session passwordResetSession
	if json.Unmarshal(rawSession, &session) != nil || session.UserID == "" || session.CodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetSessionExpired})
		return
	}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()
	var stored, channel, destination string
	var attempts int
	err = tx.QueryRowContext(ctx,
		`SELECT code_hash, channel, destination, attempts
		   FROM password_reset_codes
		  WHERE id = $1 AND user_id = $2 AND used_at IS NULL AND expires_at > NOW()
		  FOR UPDATE`,
		session.CodeID, session.UserID,
	).Scan(&stored, &channel, &destination, &attempts)
	if err != nil || channel != session.Channel || destination != session.Destination {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetCodeExpired})
		return
	}
	if attempts >= securityCodeMaxAttempts {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.PasswordResetTooManyAttempts})
		return
	}
	if !a.codeHasher.Matches(
		stored, securityCodePurposePasswordReset, session.UserID,
		destination, channel, resetToken, code,
	) {
		result, err := tx.ExecContext(ctx,
			`UPDATE password_reset_codes
			    SET attempts = attempts + 1,
			        used_at = CASE WHEN attempts + 1 >= $2 THEN NOW() ELSE used_at END
			  WHERE id = $1`,
			session.CodeID, securityCodeMaxAttempts,
		)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
			return
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 || tx.Commit() != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetCodeInvalid})
		return
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE password_reset_codes SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
		session.CodeID,
	)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 || tx.Commit() != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}

	passwordSetToken := uuid.NewString()
	exchanged, err := exchangeResetSessionScript.Run(
		ctx,
		a.redis.Client(),
		[]string{sessionKey, pwResetSetTokenKey(passwordSetToken)},
		string(rawSession),
		session.UserID,
		int(passwordSetTokenTTL.Seconds()),
	).Int64()
	if err != nil || exchanged != 1 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password_set_token": passwordSetToken})
}

func (a *App) handleForgotPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordsMismatch})
		return
	}
	v := validation.New()
	v.Password("new_password", req.NewPassword, validation.DefaultPasswordConstraints())
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}
	token := strings.TrimSpace(req.PasswordSetToken)
	if token == "" || a.redis == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetSessionExpired})
		return
	}
	ctx := r.Context()
	userID, err := consumePasswordSetTokenScript.Run(
		ctx, a.redis.Client(), []string{pwResetSetTokenKey(token)},
	).Text()
	if err != nil || userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordResetSessionExpired})
		return
	}

	var currentHash string
	if err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID,
	).Scan(&currentHash); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	if currentHash != "" && a.auth.VerifyPassword(req.NewPassword, currentHash) == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordSameAsOld})
		return
	}
	newHash, err := a.auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}

	// User session invalidation is mandatory and happens before the password
	// commit. Admin sessions use a distinct SEC-001 namespace and are untouched.
	if a.auth.Session == nil || a.auth.Session.DeleteAllForUser(ctx, userID) != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE users
		    SET password_hash = $1, password_changed_at = NOW(), updated_at = NOW()
		  WHERE id = $2`,
		newHash, userID,
	); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE password_reset_codes SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}

	a.sendPasswordChangedNotification(ctx, userID, "forgot_password", r)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg.PasswordResetSuccess})
}

func (a *App) sendPasswordChangedNotification(ctx context.Context, userID, method string, _ *http.Request) {
	var email, phone string
	var emailVerified, phoneVerified bool
	_ = a.pool.Replica().QueryRowContext(ctx,
		`SELECT COALESCE(email, ''), COALESCE(phone, ''),
		        COALESCE(email_verified, false), COALESCE(phone_verified, false)
		   FROM users WHERE id = $1`,
		userID,
	).Scan(&email, &phone, &emailVerified, &phoneVerified)

	infra.SafeGo(a.log(), "password-changed-inapp", func() {
		createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		metadata := map[string]any{"method": method, "type": "security"}
		if err := inapp.CreateNotification(
			createCtx, a.pool.Primary(), userID, inapp.NotifTypePasswordChanged,
			"Password changed", "Your account password was changed.", metadata,
		); err != nil {
			a.log().Error("Failed to create password-change notification", zap.String("user_id", userID))
		}
	})
	if phoneVerified && phone != "" && a.otpService != nil {
		infra.SafeGo(a.log(), "password-changed-sms", func() {
			sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := a.otpService.Provider().SendMessage(
				sendCtx, phone, "Your Tragge account password was changed.",
			); err != nil {
				a.log().Error("Password-change SMS was not accepted", zap.String("user_id", userID))
			}
		})
	}
	if emailVerified && email != "" && a.email != nil {
		infra.SafeGo(a.log(), "password-changed-email", func() {
			sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := a.email.SendPasswordChanged(
				sendCtx, email, notification.PasswordChangedData{Method: method},
			); err != nil {
				a.log().Error("Password-change email was not accepted", zap.String("user_id", userID))
			}
		})
	}
}

func normalizePhoneForLookup(phone string) string {
	phone = strings.ReplaceAll(phone, " ", "")
	if strings.HasPrefix(phone, "09") {
		phone = "+98" + phone[1:]
	} else if strings.HasPrefix(phone, "0098") {
		phone = "+" + phone[2:]
	}
	return phone
}
