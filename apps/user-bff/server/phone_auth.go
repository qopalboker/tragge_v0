package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/sms"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"go.uber.org/zap"
)

// cbSMSProvider wraps an SMSProvider with circuit breaker protection.
type cbSMSProvider struct {
	inner    sms.SMSProvider
	circuits *CircuitBreakers
}

func (p *cbSMSProvider) SendOTP(ctx context.Context, phone string, code string) error {
	return p.circuits.SMS.Execute(func() error {
		return p.inner.SendOTP(ctx, phone, code)
	})
}

func (p *cbSMSProvider) SendMessage(ctx context.Context, phone string, message string) error {
	return p.circuits.SMS.Execute(func() error {
		return p.inner.SendMessage(ctx, phone, message)
	})
}

func (p *cbSMSProvider) HealthCheck() error {
	return p.inner.HealthCheck()
}

// SendOTPRequest is the request body for requesting an OTP code.
type SendOTPRequest struct {
	Phone string `json:"phone"`
}

// SendOTPResponse is the response after sending an OTP code.
type SendOTPResponse struct {
	Message     string `json:"message"`
	CooldownSec int    `json:"cooldown_sec"`
}

// VerifyOTPRequest is the request body for verifying an OTP code.
type VerifyOTPRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

// RegisterPhoneRequest is the request body for phone-based registration/login.
type RegisterPhoneRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// handleSendOTP sends an OTP code to the given phone number.
func (a *App) handleSendOTP(w http.ResponseWriter, r *http.Request) {
	if a.otpService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.SMSServiceUnavailable})
		return
	}

	var req SendOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	phone, err := validation.ValidateIranPhone(req.Phone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidPhone})
		return
	}

	if err := a.otpService.SendOTP(r.Context(), phone); err != nil {
		a.log().Warn("OTP send failed",
			zap.String("phone", maskPhone(phone)),
			zap.Error(err))
		if errors.Is(err, ErrCircuitOpen) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": msg.OTPServiceTemporarilyUnavailable,
			})
			return
		}
		if errors.Is(err, sms.ErrOTPCooldown) || errors.Is(err, sms.ErrOTPExhausted) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.OTPPleaseWait})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.OTPSendFailed})
		return
	}

	// Audit log
	_, _ = a.pool.Primary().ExecContext(r.Context(),
		`INSERT INTO otp_logs (phone, ip_address, user_agent) VALUES ($1, $2, $3)`,
		phone, getClientIP(r), r.UserAgent())

	writeJSON(w, http.StatusOK, SendOTPResponse{
		Message:     msg.OTPSent,
		CooldownSec: 60,
	})
}

// handleVerifyOTP verifies an OTP code without registration/login.
func (a *App) handleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	if a.otpService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.SMSServiceUnavailable})
		return
	}

	var req VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	phone, err := validation.ValidateIranPhone(req.Phone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidPhone})
		return
	}

	if !sms.OTPCodeRegex.MatchString(req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.OTPCodeInvalid})
		return
	}

	valid, err := a.otpService.VerifyOTP(r.Context(), phone, req.Code)
	if err != nil {
		if errors.Is(err, sms.ErrOTPExhausted) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.OTPPleaseWait})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.OTPVerifyFailed})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"verified": valid})
}

// handleRegisterWithPhone handles phone-based registration or login via OTP.
func (a *App) handleRegisterWithPhone(w http.ResponseWriter, r *http.Request) {
	if a.otpService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.SMSServiceUnavailable})
		return
	}

	var req RegisterPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}

	phone, err := validation.ValidateIranPhone(req.Phone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidPhone})
		return
	}

	if !sms.OTPCodeRegex.MatchString(req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.OTPCodeInvalid})
		return
	}

	// Verify OTP
	valid, err := a.otpService.VerifyOTP(r.Context(), phone, req.Code)
	if err != nil {
		if errors.Is(err, sms.ErrOTPExhausted) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.OTPPleaseWait})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.OTPVerifyFailed})
		return
	}
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.OTPCodeWrong})
		return
	}

	ctx := r.Context()

	// Check if user already exists with this phone
	var existingUserID string
	err = a.pool.Replica().QueryRowContext(ctx,
		`SELECT id FROM users WHERE phone = $1`, phone).Scan(&existingUserID)

	if err == nil {
		// Existing user — log them in
		a.loginExistingUser(w, r, existingUserID)
		return
	}

	if err != sql.ErrNoRows {
		a.log().Error("Failed to check existing user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// New user — register
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()

	// Generate username: use random bytes instead of phone digits
	username := strings.TrimSpace(req.Username)
	if username == "" {
		randBytes := make([]byte, 4)
		_, _ = rand.Read(randBytes)
		username = "user_" + hex.EncodeToString(randBytes)
	}

	// Optional password hash (with validation)
	var passwordHash *string
	if req.Password != "" {
		if ok, _ := validation.ValidatePassword(req.Password, validation.DefaultPasswordConstraints()); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.OTPPasswordWeak})
			return
		}
		h, hashErr := a.auth.HashPassword(req.Password)
		if hashErr != nil {
			a.log().Error("Failed to hash password", zap.Error(hashErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		passwordHash = &h
	}

	// Detect language from Accept-Language header
	phoneLang := "fa" // default for Iranian platform (KaveNegar serves Iranian users)
	if acceptLang := r.Header.Get("Accept-Language"); acceptLang != "" {
		if strings.Contains(strings.ToLower(acceptLang), "en") {
			phoneLang = "en"
		}
	}

	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (phone, phone_verified, username, password_hash, status, preferred_lang, country, created_at, updated_at)
		 VALUES ($1, true, $2, $3, 'active', $4, 'IR', NOW(), NOW())
		 RETURNING id`,
		phone, username, passwordHash, phoneLang,
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			// Race condition: another request registered this phone — fall back to login
			tx.Rollback()
			var raceUserID string
			if scanErr := a.pool.Replica().QueryRowContext(ctx,
				`SELECT id FROM users WHERE phone = $1`, phone).Scan(&raceUserID); scanErr == nil {
				a.loginExistingUser(w, r, raceUserID)
				return
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": msg.UniqueViolation})
			return
		}
		a.log().Error("Failed to insert user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Assign "user" role
	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE name = 'user'`,
		userID)
	if err != nil {
		a.log().Error("Failed to assign role", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("User registered via phone OTP",
		zap.String("user_id", userID),
		zap.String("phone", maskPhone(phone)))

	a.loginAndRespond(w, r, userID, http.StatusCreated)
}

// loginExistingUser generates tokens for an existing user and responds.
func (a *App) loginExistingUser(w http.ResponseWriter, r *http.Request, userID string) {
	a.loginAndRespond(w, r, userID, http.StatusOK)
}

// loginAndRespond generates JWT tokens and writes the auth response.
func (a *App) loginAndRespond(w http.ResponseWriter, r *http.Request, userID string, statusCode int) {
	ctx := r.Context()

	// Fetch user roles
	rows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT r.name FROM roles r JOIN user_roles ur ON r.id = ur.role_id WHERE ur.user_id = $1`,
		userID)
	if err != nil {
		a.log().Error("Failed to get user roles", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err == nil {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := getClientIP(r)

	tokenPair, _, err := a.auth.Login(ctx, userID, roles, deviceInfo, ipAddress)
	if err != nil {
		a.log().Error("Failed to generate tokens", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Check email verification status
	var emailVerified bool
	_ = a.pool.Replica().QueryRowContext(ctx,
		`SELECT COALESCE(email_verified, false) FROM users WHERE id = $1`, userID,
	).Scan(&emailVerified)

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, statusCode, AuthResponse{
		AccessToken:   tokenPair.AccessToken,
		RefreshToken:  "", // Sent via httpOnly cookie
		ExpiresAt:     tokenPair.ExpiresAt,
		EmailVerified: emailVerified,
	})
}
