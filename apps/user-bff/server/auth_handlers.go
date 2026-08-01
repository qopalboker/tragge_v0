package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/models"
	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/service"
	"github.com/Parsaeffatravesh/tragge/packages/audit"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// Step 7: user-panel cookies carry the _user suffix so the admin
	// panel — served from a distinct origin — can coexist without
	// browser cookie collisions. A browser with cookies for both
	// panels under the same eTLD+1 gets refresh_token_user for the
	// user panel and refresh_token_admin for the admin panel; neither
	// ever sees the other's secret.
	refreshTokenCookieName = auth.UserRefreshCookieName
	// sessionHintCookieName is a non-HttpOnly sibling of refresh_token_user
	// that lets the frontend cheaply detect that a refresh cookie
	// likely exists, so anonymous cold loads can skip a guaranteed-401
	// /auth/refresh POST. The cookie itself carries no secret (fixed
	// value "1") — the real refresh token stays in the HttpOnly sibling.
	sessionHintCookieName  = auth.UserSessionHintCookieName
	sessionHintCookieValue = "1"
	// sessionCookieTTLSeconds matches RefreshTokenTTL in packages/auth/auth.go
	// so the hint and the refresh token expire together and asymmetric
	// storage eviction can't desync them.
	sessionCookieTTLSeconds = 7 * 24 * 3600
)

// resolveCookieSecurity picks SameSite + Secure for the refresh-token
// cookie pair from the *request* itself, not from environment vars.
// The reason: the same workspace can be reached over both HTTP (VS
// Code's local port forward at http://127.0.0.1:8080) and HTTPS (the
// public Codespaces URL). A blanket "CODESPACES=true → Secure cookies"
// rule produced cookies the browser silently dropped on the HTTP
// origin. Inspecting r tells us what the caller actually got.
//
// Two buckets:
//
//   - Request arrived over HTTPS (r.TLS != nil, or X-Forwarded-Proto
//     is "https"): SameSite=None + Secure=true. None is required for
//     cross-origin XHR with credentials (Codespaces public URLs put
//     each port on a different hostname, future preview environments
//     do similar); the spec then mandates Secure=true.
//
//   - Request arrived over HTTP: SameSite=Lax + Secure=false. A
//     Secure cookie would be silently dropped by the browser, so we
//     must turn it off. Lax is the right CSRF posture for plain-HTTP
//     same-origin localhost dev.
//
// Note: behind nginx, X-Forwarded-Proto is set by the gateway via
// $scheme — which reflects what *nginx* received, not necessarily
// what the original browser sent. If the public Codespaces URL ever
// stops being detected as HTTPS at this layer, the fix is in
// nginx.conf (honor upstream X-Forwarded-Proto via a map, not just
// $scheme). Today, accessing via http://127.0.0.1:8080 produces
// X-Forwarded-Proto=http here, which is correct for that case.
func resolveCookieSecurity(r *http.Request) (http.SameSite, bool) {
	if config.IsProduction() {
		return http.SameSiteNoneMode, true
	}
	isHTTPS := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	if isHTTPS {
		return http.SameSiteNoneMode, true
	}
	return http.SameSiteLaxMode, false
}

// setRefreshTokenCookie sets the refresh token as an httpOnly secure cookie
// and a paired non-HttpOnly session hint so the frontend can skip a
// guaranteed-to-fail refresh POST on anonymous cold loads.
func (a *App) setRefreshTokenCookie(w http.ResponseWriter, r *http.Request, refreshToken string) {
	sameSite, secure := resolveCookieSecurity(r)

	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    refreshToken,
		Path:     auth.UserRefreshCookiePath,
		MaxAge:   sessionCookieTTLSeconds,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})

	// Path=/ so every route (not just /api/user/auth) can read the hint.
	// HttpOnly=false so the SPA's hasSessionHint() can read it from JS.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    sessionHintCookieValue,
		Path:     "/",
		MaxAge:   sessionCookieTTLSeconds,
		Secure:   secure,
		HttpOnly: false,
		SameSite: sameSite,
	})
}

// clearRefreshTokenCookie removes both the refresh token cookie and the
// paired session hint cookie. Call from every path that invalidates the
// session: explicit logout and refresh-failure. Mirror the security
// attributes from the current request — eviction with mismatched
// attributes can fail to overwrite the original cookie, leaving a
// zombie that the browser keeps sending.
func (a *App) clearRefreshTokenCookie(w http.ResponseWriter, r *http.Request) {
	sameSite, secure := resolveCookieSecurity(r)

	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     auth.UserRefreshCookiePath,
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: false,
		SameSite: sameSite,
	})
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Verify ARCaptcha token (if captcha is configured)
	if a.config.ARCaptchaSiteKey != "" && a.config.ARCaptchaSecretKey != "" {
		if req.CaptchaToken == "" {
			writeErrorJSON(w, r, http.StatusBadRequest, msg.CaptchaFailed)
			return
		}
		if len(req.CaptchaToken) > 4096 {
			writeErrorJSON(w, r, http.StatusBadRequest, msg.CaptchaFailed)
			return
		}
		captchaValid, captchaErr := verifyCaptcha(
			r.Context(),
			a.config.ARCaptchaSiteKey,
			a.config.ARCaptchaSecretKey,
			req.CaptchaToken,
		)
		if captchaErr != nil {
			// Fail-open: allow registration when captcha service is unreachable.
			// This prevents blocking all registrations if ARCaptcha goes down.
			a.log().Warn("Captcha API unreachable, allowing registration (fail-open)",
				zap.Error(captchaErr),
				zap.String("email", req.Email),
				zap.String("ip", getClientIP(r)))
			// Continue with registration — rate limiter still protects against abuse
		} else if !captchaValid {
			a.log().Warn("Captcha verification rejected",
				zap.String("email", req.Email),
				zap.String("ip", getClientIP(r)))
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.CaptchaFailed})
			return
		}
	}

	// Validate input using validation package
	v := validation.New()
	req.Email = v.Email("email", req.Email)
	req.Country = strings.ToUpper(strings.TrimSpace(req.Country))
	v.Required("country", req.Country)
	if req.Country != "" && !validCountryCodes[req.Country] {
		v.AddError("country", "invalid_format", "Country must be an ISO 3166-1 alpha-2 code")
	}
	v.Password("password", req.Password, validation.DefaultPasswordConstraints())
	if !req.AgreeTerms {
		v.AddError("agree_terms", "required", "You must agree to the Terms & Conditions")
	}
	if !req.AgeConfirm {
		v.AddError("age_confirm", "required", "You must confirm you are 18 years or older")
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Sanitize email (already normalized by validator)
	req.Email = validation.SanitizeEmail(req.Email)

	// Hash password
	passwordHash, err := a.auth.HashPassword(req.Password)
	if err != nil {
		a.log().Error("Failed to hash password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	ctx := r.Context()

	// Begin transaction on primary (writes)
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer tx.Rollback()

	// Detect language from Accept-Language header for email preferences
	lang := "fa" // default for Iranian platform
	if acceptLang := r.Header.Get("Accept-Language"); acceptLang != "" {
		if strings.Contains(strings.ToLower(acceptLang), "en") {
			lang = "en"
		}
	}

	// Insert user with email_verified = FALSE
	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, email_verified, terms_accepted_at, preferred_lang, country) VALUES ($1, $2, FALSE, NOW(), $3, $4) RETURNING id`,
		req.Email, passwordHash, lang, req.Country,
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			// Intentional: return identical response as success to prevent account enumeration.
			// Send notification to existing account holder instead.
			tx.Rollback()
			a.log().Info("Registration attempted with existing email",
				zap.String("email", req.Email),
				zap.String("remote_addr", r.RemoteAddr))

			// Send email to existing user asynchronously
			if a.email != nil {
				infra.SafeGo(a.log(), "registration-attempt-email", func() {
					_ = a.email.SendEmail(context.Background(), []string{req.Email},
						"Registration Attempt on Tragge",
						"<p>Someone tried to register with your email address. If this was you, please login instead. If not, you can safely ignore this email.</p>",
					)
				})
			}

			// Mimic similar timing to a successful registration to prevent timing-based enumeration
			writeJSON(w, http.StatusCreated, AuthResponse{
				AccessToken:   "",
				RefreshToken:  "",
				EmailVerified: false,
			})
			return
		}
		a.log().Error("Failed to insert user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get the "user" role ID and assign it
	var roleID int
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = 'user'`).Scan(&roleID)
	if err != nil {
		a.log().Error("Failed to get user role", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
		userID, roleID,
	)
	if err != nil {
		a.log().Error("Failed to assign role", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Handle referral code if provided (silently ignore invalid codes)
	if req.ReferralCode != "" {
		// Validate and get the referrer info
		var referrerID string
		var codeIsActive bool
		err = tx.QueryRowContext(ctx,
			`SELECT user_id, is_active FROM referral_codes WHERE code = $1`,
			strings.ToUpper(req.ReferralCode),
		).Scan(&referrerID, &codeIsActive)

		// Only create referral if code is valid, active, and referrer is not the new user
		if err == nil && codeIsActive && referrerID != userID {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO referrals (referrer_id, referred_id, code, status)
				VALUES ($1, $2, $3, 'pending')
				ON CONFLICT (referred_id) DO NOTHING
			`, referrerID, userID, strings.ToUpper(req.ReferralCode))
			if err != nil {
				// Log but don't fail registration - referral is optional
				a.log().Warn("Failed to create referral entry",
					zap.String("referral_code", req.ReferralCode),
					zap.String("referred_id", userID),
					zap.Error(err))
			} else {
				a.log().Info("Referral created",
					zap.String("referrer_id", referrerID),
					zap.String("referred_id", userID),
					zap.String("code", req.ReferralCode))
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get device info and IP address
	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := r.RemoteAddr
	// Try to get real IP from X-Forwarded-For or X-Real-IP headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ipAddress = strings.Split(xff, ",")[0]
	} else if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ipAddress = xri
	}

	// Login using Auth service (creates session if Redis is available)
	roles := []string{"user"}
	tokenPair, sessionID, err := a.auth.Login(ctx, userID, roles, deviceInfo, ipAddress)
	if err != nil {
		a.log().Error("Failed to login after registration", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Include session_id in response if available
	if sessionID != "" {
		a.log().Info("User registered with session",
			zap.String("user_id", userID),
			zap.String("session_id", sessionID))
	}

	// Registration reports success only after the country-routed provider has
	// accepted delivery and the new code has been activated.
	if _, err := a.issueVerificationCode(ctx, userID, "email"); err != nil {
		if a.auth.Session != nil {
			_ = a.auth.Session.DeleteAllForUser(ctx, userID)
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verification_delivery_unavailable"})
		return
	}

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusCreated, AuthResponse{
		AccessToken:          tokenPair.AccessToken,
		RefreshToken:         "", // Sent via httpOnly cookie
		ExpiresAt:            tokenPair.ExpiresAt,
		EmailVerified:        false,
		RetryAfterSeconds:    int(securityCodeCooldown.Seconds()),
		RequiresVerification: true,
		AvailableMethods:     []string{"email"},
		MaskedEmail:          maskEmail(req.Email),
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	if a.distributedLoginLockout == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	if allowed, retryAfter, err := a.distributedLoginLockout.Check(r.Context(), "ip:"+clientIP); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	} else if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.TooManyLoginAttempts})
		return
	}

	// Check if client IP is currently locked out due to too many failures
	if locked, retryAfter := a.failedLoginTracker.checkLocked(clientIP); locked {
		a.log().Warn("Login blocked due to too many failed attempts",
			zap.String("ip", clientIP),
			zap.Duration("retry_after", retryAfter))
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":       "too many failed attempts",
			"message":     msg.AccountLocked,
			"retry_after": int(retryAfter.Seconds()) + 1,
		})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Validate and sanitize input
	v := validation.New()
	req.Email = v.Email("email", req.Email)
	v.Required("password", req.Password)

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Sanitize email
	req.Email = validation.SanitizeEmail(req.Email)

	ctx := r.Context()
	lockoutIdentities := []string{"ip:" + clientIP, "account:" + req.Email}
	if allowed, retryAfter, err := a.distributedLoginLockout.Check(ctx, lockoutIdentities...); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	} else if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg.TooManyLoginAttempts})
		return
	}

	// Get user by email (use Primary for auth - ensures latest password hash)
	var userID, passwordHash string
	var emailVerified bool
	var isSystemAccount bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT id, password_hash, COALESCE(email_verified, FALSE), COALESCE(is_system_account, FALSE) FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &passwordHash, &emailVerified, &isSystemAccount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Perform dummy hash to prevent timing-based user enumeration.
			// This ensures the response time is similar whether the user exists or not.
			_ = auth.VerifyPassword(req.Password, auth.DummyHash)

			// Record failed attempt and log for security monitoring
			if _, lockErr := a.distributedLoginLockout.Failure(ctx, lockoutIdentities...); lockErr != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
				return
			}
			delay := a.failedLoginTracker.recordFailure(clientIP)
			a.log().Warn("Failed login attempt - user not found",
				zap.String("email", req.Email),
				zap.String("ip", clientIP),
				zap.Duration("next_delay", delay))
			a.auditLogger.LogFromRequest(r, "", audit.EventLoginFailed, map[string]interface{}{
				"email": req.Email, "reason": "user_not_found",
			})

			// Return 429 immediately if progressive delay applies (avoid blocking goroutine)
			if delay > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
				writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
					"error":       "too many failed attempts",
					"message":     msg.TooManyLoginAttempts,
					"retry_after": int(delay.Seconds()) + 1,
				})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidCredentials})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Block system accounts from logging in
	if isSystemAccount {
		a.log().Warn("Login attempt for system account blocked",
			zap.String("user_id", userID),
			zap.String("email", req.Email),
			zap.String("ip", clientIP))
		writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.SystemAccountBlocked})
		return
	}

	// Verify password
	if err := a.auth.VerifyPassword(req.Password, passwordHash); err != nil {
		// Record failed attempt and log for security monitoring
		if _, lockErr := a.distributedLoginLockout.Failure(ctx, lockoutIdentities...); lockErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
			return
		}
		delay := a.failedLoginTracker.recordFailure(clientIP)
		a.log().Warn("Failed login attempt - invalid password",
			zap.String("user_id", userID),
			zap.String("email", req.Email),
			zap.String("ip", clientIP),
			zap.Duration("next_delay", delay))
		a.auditLogger.LogFromRequest(r, userID, audit.EventLoginFailed, map[string]interface{}{
			"email": req.Email, "reason": "invalid_password",
		})

		// Return 429 immediately if progressive delay applies (avoid blocking goroutine)
		if delay > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
				"error":       "too many failed attempts",
				"message":     msg.TooManyLoginAttempts,
				"retry_after": int(delay.Seconds()) + 1,
			})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidCredentials})
		return
	}

	// Check if 2FA is enabled for this user
	var totpEnabled bool
	if err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT COALESCE(totp_enabled, false) FROM users WHERE id = $1`, userID,
	).Scan(&totpEnabled); err != nil {
		a.log().Error("Failed to check 2FA status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if totpEnabled {
		// Rate-limit ticket issuance per user to prevent brute-force 2FA attempts
		if a.redis != nil {
			pendingKey := fmt.Sprintf("auth:user:2fa:pending:%s", userID)
			pendingCount, _ := a.redis.Get(ctx, pendingKey).Int()
			if pendingCount >= 10 {
				a.log().Warn("Too many 2FA ticket requests",
					zap.String("user_id", userID),
					zap.String("ip", clientIP))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": msg.TwoFATooManyAttempts,
				})
				return
			}
			a.redis.Incr(ctx, pendingKey)
			a.redis.Expire(ctx, pendingKey, 15*time.Minute)
		}

		// 2FA is enabled — issue a single-use ticket instead of tokens
		ticket := uuid.New().String()
		ticketData, _ := json.Marshal(map[string]interface{}{
			"user_id":        userID,
			"email_verified": emailVerified,
			"ip":             clientIP,
			"device_info":    r.Header.Get("User-Agent"),
		})
		if a.redis != nil {
			ticketKey := fmt.Sprintf("auth:user:2fa:login-ticket:%s", ticket)
			if err := a.redis.Set(ctx, ticketKey, string(ticketData), 5*time.Minute).Err(); err != nil {
				a.log().Error("Failed to store 2FA login ticket", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
				return
			}
		}

		a.log().Info("2FA login ticket issued",
			zap.String("user_id", userID),
			zap.String("ip", clientIP))
		a.auditLogger.LogFromRequest(r, userID, audit.EventLogin, map[string]interface{}{
			"email": req.Email, "requires_2fa": true,
		})

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requires_2fa": true,
			"ticket":       ticket,
		})
		return
	}

	// Successful login without 2FA - clear failed attempt tracking
	if err := a.distributedLoginLockout.Success(ctx, lockoutIdentities...); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.InternalError})
		return
	}
	a.failedLoginTracker.recordSuccess(clientIP)

	a.auditLogger.LogFromRequest(r, userID, audit.EventLogin, map[string]interface{}{
		"email": req.Email,
	})

	// Get user roles
	roles, err := a.getUserRoles(ctx, userID)
	if err != nil {
		a.log().Error("Failed to get user roles", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get device info and IP address
	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := clientIP

	// Login using Auth service (creates session if Redis is available)
	tokenPair, sessionID, err := a.auth.Login(ctx, userID, roles, deviceInfo, ipAddress)
	if err != nil {
		a.log().Error("Failed to login", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	response := AuthResponse{
		AccessToken:   tokenPair.AccessToken,
		RefreshToken:  "", // Sent via httpOnly cookie
		ExpiresAt:     tokenPair.ExpiresAt,
		EmailVerified: emailVerified,
	}

	// If email not verified, add verification info to response
	if !emailVerified {
		var phone sql.NullString
		var phoneVerified bool
		_ = a.pool.Replica().QueryRowContext(ctx,
			`SELECT phone, COALESCE(phone_verified, false) FROM users WHERE id = $1`, userID,
		).Scan(&phone, &phoneVerified)

		response.RequiresVerification = true
		response.AvailableMethods = availableVerificationMethods(
			sql.NullString{String: req.Email, Valid: true}, phone, emailVerified, phoneVerified,
		)
		response.MaskedEmail = maskEmail(req.Email)
		if phone.Valid && phone.String != "" {
			response.MaskedPhone = maskPhone(phone.String)
		}
	}

	// Log successful login
	a.log().Info("User logged in successfully",
		zap.String("user_id", userID),
		zap.String("ip", clientIP))

	// Include session_id in response if available
	if sessionID != "" {
		a.log().Debug("Session created",
			zap.String("user_id", userID),
			zap.String("session_id", sessionID))
	}

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, response)
}

// handle2FALoginVerify verifies a TOTP code during login and completes authentication.
// POST /api/user/auth/2fa/login
func (a *App) handle2FALoginVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ticket == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFATicketRequired})
		return
	}

	// Validate code format (6 digits)
	if len(req.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFACodeDigits})
		return
	}
	for _, c := range req.Code {
		if c < '0' || c > '9' {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFACodeDigits})
			return
		}
	}

	// Retrieve and delete ticket from Redis (single-use via GetDel)
	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.TwoFAServiceUnavailable})
		return
	}

	ticketKey := fmt.Sprintf("auth:user:2fa:login-ticket:%s", req.Ticket)
	ticketDataStr, err := a.redis.GetDel(ctx, ticketKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidOrExpiredToken})
			return
		}
		a.log().Error("Failed to retrieve 2FA login ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Parse ticket data
	var ticketData struct {
		UserID        string `json:"user_id"`
		EmailVerified bool   `json:"email_verified"`
		IP            string `json:"ip"`
		DeviceInfo    string `json:"device_info"`
	}
	if err := json.Unmarshal([]byte(ticketDataStr), &ticketData); err != nil {
		a.log().Error("Failed to parse 2FA login ticket data", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Brute force protection per user
	if a.redis != nil {
		attemptKey := fmt.Sprintf("auth:user:2fa:login-attempts:%s", ticketData.UserID)
		attempts, _ := a.redis.Get(ctx, attemptKey).Int()
		if attempts >= 5 {
			a.log().Warn("2FA login verification locked due to too many failed attempts",
				zap.String("user_id", ticketData.UserID))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": msg.TwoFATooManyAttempts,
			})
			return
		}
	}

	// Get user's TOTP secret
	var totpSecret sql.NullString
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT totp_secret FROM users WHERE id = $1`, ticketData.UserID,
	).Scan(&totpSecret)
	if err != nil {
		a.log().Error("Failed to query TOTP secret for login verification", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if !totpSecret.Valid || totpSecret.String == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.TwoFANotConfigured})
		return
	}

	// Decrypt TOTP secret if encrypted
	plaintextSecret := totpSecret.String
	if a.totpEncryptionKey != nil {
		decrypted, decErr := auth.DecryptTOTPSecret(totpSecret.String, a.totpEncryptionKey)
		if decErr != nil {
			a.log().Error("Failed to decrypt TOTP secret", zap.Error(decErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		plaintextSecret = decrypted
	}

	// Verify TOTP code
	if !auth.VerifyTOTP(plaintextSecret, req.Code, time.Now()) {
		// Track failed attempt per user
		if a.redis != nil {
			attemptKey := fmt.Sprintf("auth:user:2fa:login-attempts:%s", ticketData.UserID)
			a.redis.Incr(ctx, attemptKey)
			a.redis.Expire(ctx, attemptKey, 15*time.Minute)
		}
		a.log().Warn("Failed 2FA login verification",
			zap.String("user_id", ticketData.UserID),
			zap.String("ip", getClientIP(r)))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.TwoFAInvalidCode})
		return
	}

	// 2FA verified — complete login flow
	// Clear failed attempt counter and pending counter
	if a.redis != nil {
		a.redis.Del(ctx, fmt.Sprintf("auth:user:2fa:login-attempts:%s", ticketData.UserID))
		a.redis.Del(ctx, fmt.Sprintf("auth:user:2fa:pending:%s", ticketData.UserID))
	}
	a.failedLoginTracker.recordSuccess(getClientIP(r))

	// Get user roles
	roles, err := a.getUserRoles(ctx, ticketData.UserID)
	if err != nil {
		a.log().Error("Failed to get user roles", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Use device info and IP from original login request
	deviceInfo := ticketData.DeviceInfo
	ipAddress := ticketData.IP

	// Login using Auth service
	tokenPair, sessionID, err := a.auth.Login(ctx, ticketData.UserID, roles, deviceInfo, ipAddress)
	if err != nil {
		a.log().Error("Failed to login after 2FA", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.auditLogger.LogFromRequest(r, ticketData.UserID, audit.EventLogin, map[string]interface{}{
		"2fa_verified": true,
	})

	a.log().Info("User logged in successfully with 2FA",
		zap.String("user_id", ticketData.UserID),
		zap.String("ip", getClientIP(r)))

	if sessionID != "" {
		a.log().Debug("Session created",
			zap.String("user_id", ticketData.UserID),
			zap.String("session_id", sessionID))
	}

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:   tokenPair.AccessToken,
		RefreshToken:  "", // Sent via httpOnly cookie
		ExpiresAt:     tokenPair.ExpiresAt,
		EmailVerified: ticketData.EmailVerified,
	})
}

// handleRefresh handles token refresh requests.
// POST /api/user/auth/refresh
func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	// Read refresh token from httpOnly cookie (primary) or request body (legacy fallback)
	var refreshTokenValue string

	if cookie, err := r.Cookie(refreshTokenCookieName); err == nil && cookie.Value != "" {
		refreshTokenValue = cookie.Value
	} else {
		// Legacy fallback: read from request body (for clients not yet updated)
		var req RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
			refreshTokenValue = req.RefreshToken
		}
	}

	if refreshTokenValue == "" {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidRefreshToken)
		return
	}

	ctx := r.Context()

	// Validate the refresh token to get the session ID
	claims, err := a.auth.Token.ValidateRefreshToken(refreshTokenValue)
	if err != nil {
		a.log().Warn("Invalid refresh token",
			zap.Error(err),
			zap.String("ip", getClientIP(r)))
		// Do NOT clear cookies on 401. Cookies are origin-wide, and wiping
		// them here would destroy valid sessions in sibling tabs that just
		// refreshed successfully. The client decides when to clear state
		// (see auth.ts refreshAccessToken) — server just replies 401.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidRefreshToken})
		return
	}

	// Use the Auth service to refresh tokens (handles session validation)
	tokenPair, err := a.auth.Refresh(ctx, claims.SessionID, refreshTokenValue)
	if err != nil {
		a.log().Warn("Token refresh failed",
			zap.Error(err),
			zap.String("user_id", claims.UserID),
			zap.String("session_id", claims.SessionID),
			zap.String("ip", getClientIP(r)))
		// Same rationale: don't wipe cookies for sibling tabs. Client
		// handles invalidation based on the 401 response.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.RefreshFailed})
		return
	}

	// Calculate expires_in in seconds (access token TTL is 48 hours)
	expiresIn := int(time.Until(tokenPair.ExpiresAt).Seconds())

	a.log().Info("Token refreshed successfully",
		zap.String("user_id", claims.UserID),
		zap.String("session_id", claims.SessionID),
		zap.String("ip", getClientIP(r)))

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, RefreshResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: "", // Sent via httpOnly cookie
		ExpiresIn:    expiresIn,
	})
}

// handleCreateTicket creates a short-lived, single-use auth ticket for cross-origin navigation.
// POST /api/user/auth/ticket (protected)
// The ticket can be exchanged for an access token within 30 seconds.
func (a *App) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Ensure Redis is available (tickets require Redis)
	if a.redis == nil {
		writeErrorJSON(w, r, http.StatusServiceUnavailable, "ticket service unavailable")
		return
	}

	// Extract the raw access token from the Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		writeErrorJSON(w, r, http.StatusUnauthorized, "missing authorization header")
		return
	}
	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Generate a cryptographically random ticket
	ticket := uuid.New().String()
	redisKey := "auth_ticket:" + ticket

	// Store the ticket in Redis with a 30-second TTL
	if err := a.redis.Set(ctx, redisKey, accessToken, 30*time.Second).Err(); err != nil {
		a.log().Error("Failed to store auth ticket in Redis",
			zap.Error(err),
			zap.String("user_id", auth.GetUserID(ctx)),
			zap.String("ip", getClientIP(r)))
		writeErrorJSON(w, r, http.StatusInternalServerError, "failed to create ticket")
		return
	}

	a.log().Info("Auth ticket created",
		zap.String("user_id", auth.GetUserID(ctx)),
		zap.String("ticket", ticket),
		zap.String("ip", getClientIP(r)))

	writeJSON(w, http.StatusOK, AuthTicketResponse{
		Ticket: ticket,
	})
}

// handleExchangeTicket exchanges a short-lived ticket for an access token.
// POST /api/user/auth/exchange-ticket (public)
// Tickets are single-use and expire after 30 seconds.
func (a *App) handleExchangeTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Ensure Redis is available
	if a.redis == nil {
		writeErrorJSON(w, r, http.StatusServiceUnavailable, "ticket service unavailable")
		return
	}

	var req ExchangeTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	if req.Ticket == "" {
		writeErrorJSON(w, r, http.StatusBadRequest, "ticket is required")
		return
	}

	// Validate ticket format (must be a valid UUID)
	if _, err := uuid.Parse(req.Ticket); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, "invalid ticket format")
		return
	}

	redisKey := "auth_ticket:" + req.Ticket

	// Retrieve and delete the ticket atomically (single-use)
	accessToken, err := a.redis.GetDel(ctx, redisKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			a.log().Warn("Invalid or expired auth ticket exchange attempt",
				zap.String("ticket", req.Ticket),
				zap.String("ip", getClientIP(r)))
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidOrExpiredToken})
			return
		}
		a.log().Error("Failed to retrieve auth ticket from Redis",
			zap.Error(err),
			zap.String("ip", getClientIP(r)))
		writeErrorJSON(w, r, http.StatusInternalServerError, "failed to exchange ticket")
		return
	}

	a.log().Info("Auth ticket exchanged successfully",
		zap.String("ticket", req.Ticket),
		zap.String("ip", getClientIP(r)))

	writeJSON(w, http.StatusOK, ExchangeTicketResponse{
		AccessToken:  accessToken,
		RefreshToken: nil,
	})
}

// generateVerificationCode generates a cryptographically secure 6-digit code.
func generateVerificationCode() (string, error) {
	max := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	code := n.Int64() + 100000
	return fmt.Sprintf("%d", code), nil
}

// isDigitsOnly checks if a string contains only ASCII digits.
func isDigitsOnly(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NewUserInitParams contains parameters for post-registration user initialization.
type NewUserInitParams struct {
	UserID        string
	Email         string
	EmailVerified bool   // If true (OAuth), skip verification token generation
	Lang          string // Language code: "en" or "fa"
}

// initializeNewUser performs post-registration initialization for a new user.
// This is called from both regular registration and OAuth registration paths.
// Currently sends a welcome email (with or without verification code).
func (a *App) initializeNewUser(ctx context.Context, params NewUserInitParams) {
	if params.Lang == "" {
		params.Lang = "en"
	}

	// Password registration performs synchronous security-code delivery before
	// this helper is reached. OAuth calls this only for an already verified email.
	if !params.EmailVerified {
		return
	}
	emailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	infra.SafeGo(a.log(), "welcome-email", func() {
		defer cancel()
		a.sendWelcomeEmailWithoutVerification(emailCtx, params.UserID, params.Email, params.Lang)
	})
}

// sendWelcomeEmailWithoutVerification sends a welcome email without a verification code.
// Used for OAuth users whose email is already verified by the provider.
func (a *App) sendWelcomeEmailWithoutVerification(ctx context.Context, userID, email, lang string) {
	if a.email == nil {
		a.log().Warn("Welcome email requested but email notifier not configured",
			zap.String("user_id", userID))
		return
	}

	if lang == "" {
		lang = "en"
	}

	dashboardURL := fmt.Sprintf("%s/user/dashboard", a.config.FrontendURL)

	err := a.email.SendWelcomeEmail(ctx, email, notification.WelcomeEmailData{
		UserEmail:    email,
		DashboardURL: dashboardURL,
		Lang:         lang,
	})
	if err != nil {
		a.log().Error("Failed to send welcome email for OAuth user",
			zap.String("email", email),
			zap.String("user_id", userID),
			zap.String("lang", lang),
			zap.Error(err))
	} else {
		a.log().Info("Welcome email sent (OAuth, no verification needed)",
			zap.String("email", email),
			zap.String("user_id", userID),
			zap.String("lang", lang))
	}
}

// handleVerifyEmail preserves the legacy route while delegating to the same
// HMAC-bound, atomic verification lifecycle as /verify-code.
func (a *App) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	var req VerifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidBody})
		return
	}
	if err := a.verifyVerificationCode(r.Context(), userID, "email", strings.TrimSpace(req.Code)); err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, errWrongCode) && !errors.Is(err, errNoActiveCode) && !errors.Is(err, errCodeExhausted) {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]string{"error": "invalid_or_expired_code"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg.EmailVerifiedSuccess})
}

// handleResendVerification uses the canonical synchronous issuance path.
func (a *App) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	if _, err := a.issueVerificationCode(r.Context(), userID, "email"); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, errVerificationCooldown) {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, map[string]string{"error": "verification_delivery_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":             msg.VerificationCodeSent,
		"retry_after_seconds": int(securityCodeCooldown.Seconds()),
	})
}
func (a *App) handle2FAStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var totpEnabled bool
	var totpVerifiedAt sql.NullTime
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT COALESCE(totp_enabled, false), totp_verified_at FROM users WHERE id = $1`, userID,
	).Scan(&totpEnabled, &totpVerifiedAt)
	if err != nil {
		a.log().Error("Failed to query 2FA status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	resp := map[string]interface{}{
		"enabled": totpEnabled,
	}
	if totpVerifiedAt.Valid {
		resp["verified_at"] = totpVerifiedAt.Time.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handle2FASetup initiates 2FA setup by generating a TOTP secret.
// Returns the secret and provisioning URI for QR code generation.
func (a *App) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check if 2FA is already enabled
	var totpEnabled bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT COALESCE(totp_enabled, false) FROM users WHERE id = $1`, userID,
	).Scan(&totpEnabled)
	if err != nil {
		a.log().Error("Failed to query 2FA status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if totpEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFAAlreadyEnabled})
		return
	}

	// Generate a random 20-byte secret (base32 encoded for TOTP)
	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		a.log().Error("Failed to generate TOTP secret", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)

	// Get user email or phone for provisioning URI
	var email sql.NullString
	var phone sql.NullString
	_ = a.pool.Replica().QueryRowContext(ctx,
		`SELECT email, phone FROM users WHERE id = $1`, userID,
	).Scan(&email, &phone)
	accountName := email.String
	if accountName == "" {
		accountName = phone.String
	}

	// Encrypt secret before storing in DB — refuse to store plaintext
	if a.totpEncryptionKey == nil {
		a.log().Error("TOTP_ENCRYPTION_KEY not configured, refusing to store plaintext TOTP secret")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg.TwoFAServiceUnavailable})
		return
	}
	secretToStore := secret
	encrypted, encErr := encryptTOTPSecret(secret, a.totpEncryptionKey)
	if encErr != nil {
		a.log().Error("Failed to encrypt TOTP secret", zap.Error(encErr))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	secretToStore = encrypted

	// Store (encrypted) secret in DB
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET totp_secret = $1, totp_enabled = false WHERE id = $2`,
		secretToStore, userID,
	)
	if err != nil {
		a.log().Error("Failed to store TOTP secret", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Build otpauth:// URI for QR code generation on the client side
	issuer := "Tragge"
	provisioningURI := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, accountName, secret, issuer)

	writeJSON(w, http.StatusOK, map[string]string{
		"secret":           secret,
		"provisioning_uri": provisioningURI,
	})
}

// handle2FAVerify verifies a TOTP code and enables 2FA.
// Also generates backup codes.
func (a *App) handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check 2FA attempt rate limit (brute-force protection)
	if a.redis != nil {
		attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
		attempts, _ := a.redis.Get(ctx, attemptKey).Int()
		if attempts >= 5 {
			a.log().Warn("2FA verification locked due to too many failed attempts",
				zap.String("user_id", userID),
				zap.Int("attempts", attempts))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": msg.TwoFATooManyAttempts,
			})
			return
		}
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.VerificationCodeRequired})
		return
	}

	// Validate code format (6 digits)
	if len(req.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFACodeDigits})
		return
	}
	for _, c := range req.Code {
		if c < '0' || c > '9' {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFACodeDigits})
			return
		}
	}

	// Get stored secret
	var totpSecret sql.NullString
	var totpEnabled bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT totp_secret, COALESCE(totp_enabled, false) FROM users WHERE id = $1`, userID,
	).Scan(&totpSecret, &totpEnabled)
	if err != nil {
		a.log().Error("Failed to query TOTP secret", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if totpEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFAAlreadyEnabled})
		return
	}
	if !totpSecret.Valid || totpSecret.String == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.TwoFASetupNotInitiated})
		return
	}

	// Decrypt TOTP secret if encrypted
	plaintextSecret := totpSecret.String
	if a.totpEncryptionKey != nil {
		decrypted, decErr := auth.DecryptTOTPSecret(totpSecret.String, a.totpEncryptionKey)
		if decErr != nil {
			a.log().Error("Failed to decrypt TOTP secret", zap.Error(decErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		plaintextSecret = decrypted
	} else if !strings.HasPrefix(totpSecret.String, "enc:") {
		// CRITICAL: Found a plaintext TOTP secret in the database — encryption key must be set
		a.log().Error("CRITICAL: Found plaintext TOTP secret in database — TOTP_ENCRYPTION_KEY must be set",
			zap.String("user_id", userID))
	}

	// Verify TOTP code using HMAC-SHA1 (RFC 6238)
	if !auth.VerifyTOTP(plaintextSecret, req.Code, time.Now()) {
		// Track failed 2FA attempt
		if a.redis != nil {
			attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
			a.redis.Incr(ctx, attemptKey)
			a.redis.Expire(ctx, attemptKey, 15*time.Minute)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.TwoFAInvalidCode})
		return
	}

	// Clear failed 2FA attempt counter on success
	if a.redis != nil {
		attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
		a.redis.Del(ctx, attemptKey)
	}

	// Generate backup codes (10 codes, 8 characters each)
	backupCodes := make([]string, 10)
	hashedCodes := make([]string, 10)
	for i := 0; i < 10; i++ {
		codeBytes := make([]byte, 4)
		rand.Read(codeBytes)
		code := fmt.Sprintf("%08x", codeBytes)
		backupCodes[i] = code
		hash := sha256.Sum256([]byte(code))
		hashedCodes[i] = fmt.Sprintf("%x", hash)
	}

	// Enable 2FA and store hashed backup codes
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET totp_enabled = true, totp_verified_at = NOW(), backup_codes = $1 WHERE id = $2`,
		hashedCodes, userID,
	)
	if err != nil {
		a.log().Error("Failed to enable 2FA", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("2FA enabled",
		zap.String("user_id", userID))
	a.auditLogger.LogFromRequest(r, userID, audit.Event2FAEnable, nil)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":      true,
		"backup_codes": backupCodes, // Show once, never again
		"message":      msg.TwoFAEnabledSuccess,
	})
}

// handle2FADisable disables 2FA for the authenticated user.
// Requires current password for security.
func (a *App) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check 2FA attempt rate limit (brute-force protection)
	if a.redis != nil {
		attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
		attempts, _ := a.redis.Get(ctx, attemptKey).Int()
		if attempts >= 5 {
			a.log().Warn("2FA disable locked due to too many failed attempts",
				zap.String("user_id", userID),
				zap.Int("attempts", attempts))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": msg.TwoFATooManyAttempts,
			})
			return
		}
	}

	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"` // TOTP code or backup code
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.PasswordRequired})
		return
	}

	// Verify password
	var passwordHash string
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID,
	).Scan(&passwordHash)
	if err != nil {
		a.log().Error("Failed to query user password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	if err := a.auth.VerifyPassword(req.Password, passwordHash); err != nil {
		// Track failed attempt
		if a.redis != nil {
			attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
			a.redis.Incr(ctx, attemptKey)
			a.redis.Expire(ctx, attemptKey, 15*time.Minute)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidPassword})
		return
	}

	// Clear failed attempt counter on success
	if a.redis != nil {
		attemptKey := fmt.Sprintf("auth:user:2fa:attempts:%s", userID)
		a.redis.Del(ctx, attemptKey)
	}

	// Disable 2FA
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET totp_enabled = false, totp_secret = NULL, totp_verified_at = NULL, backup_codes = NULL WHERE id = $1`,
		userID,
	)
	if err != nil {
		a.log().Error("Failed to disable 2FA", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("2FA disabled",
		zap.String("user_id", userID))

	writeJSON(w, http.StatusOK, map[string]string{"message": msg.TwoFADisabledSuccess})
}

// ============================================================================
// KYC ENDPOINTS
// ============================================================================

// KYC constants
const (
	kycMaxFileSize          = 10 * 1024 * 1024 // 10MB per file
	kycMaxSubmissionsPerDay = 3                // Max KYC submissions per 24 hours
)

// Allowed MIME types for KYC uploads
var kycAllowedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// Valid ISO 3166-1 alpha-2 country codes (common subset)
var validCountryCodes = map[string]bool{
	"AF": true, "AL": true, "DZ": true, "AD": true, "AO": true, "AR": true, "AM": true, "AU": true,
	"AT": true, "AZ": true, "BH": true, "BD": true, "BY": true, "BE": true, "BZ": true, "BJ": true,
	"BT": true, "BO": true, "BA": true, "BW": true, "BR": true, "BN": true, "BG": true, "BF": true,
	"BI": true, "KH": true, "CM": true, "CA": true, "CV": true, "CF": true, "TD": true, "CL": true,
	"CN": true, "CO": true, "KM": true, "CG": true, "CD": true, "CR": true, "CI": true, "HR": true,
	"CU": true, "CY": true, "CZ": true, "DK": true, "DJ": true, "DM": true, "DO": true, "EC": true,
	"EG": true, "SV": true, "GQ": true, "ER": true, "EE": true, "ET": true, "FJ": true, "FI": true,
	"FR": true, "GA": true, "GM": true, "GE": true, "DE": true, "GH": true, "GR": true, "GD": true,
	"GT": true, "GN": true, "GW": true, "GY": true, "HT": true, "HN": true, "HU": true, "IS": true,
	"IN": true, "ID": true, "IR": true, "IQ": true, "IE": true, "IL": true, "IT": true, "JM": true,
	"JP": true, "JO": true, "KZ": true, "KE": true, "KI": true, "KP": true, "KR": true, "KW": true,
	"KG": true, "LA": true, "LV": true, "LB": true, "LS": true, "LR": true, "LY": true, "LI": true,
	"LT": true, "LU": true, "MK": true, "MG": true, "MW": true, "MY": true, "MV": true, "ML": true,
	"MT": true, "MH": true, "MR": true, "MU": true, "MX": true, "FM": true, "MD": true, "MC": true,
	"MN": true, "ME": true, "MA": true, "MZ": true, "MM": true, "NA": true, "NR": true, "NP": true,
	"NL": true, "NZ": true, "NI": true, "NE": true, "NG": true, "NO": true, "OM": true, "PK": true,
	"PW": true, "PA": true, "PG": true, "PY": true, "PE": true, "PH": true, "PL": true, "PT": true,
	"QA": true, "RO": true, "RU": true, "RW": true, "KN": true, "LC": true, "VC": true, "WS": true,
	"SM": true, "ST": true, "SA": true, "SN": true, "RS": true, "SC": true, "SL": true, "SG": true,
	"SK": true, "SI": true, "SB": true, "SO": true, "ZA": true, "SS": true, "ES": true, "LK": true,
	"SD": true, "SR": true, "SZ": true, "SE": true, "CH": true, "SY": true, "TW": true, "TJ": true,
	"TZ": true, "TH": true, "TL": true, "TG": true, "TO": true, "TT": true, "TN": true, "TR": true,
	"TM": true, "TV": true, "UG": true, "UA": true, "AE": true, "GB": true, "US": true, "UY": true,
	"UZ": true, "VU": true, "VA": true, "VE": true, "VN": true, "YE": true, "ZM": true, "ZW": true,
}

// Valid KYC document types
var validDocumentTypes = map[string]bool{
	"passport":          true,
	"national_id":       true,
	"drivers_license":   true,
	"residence_permit":  true,
	"birth_certificate": true,
}

// documentNumberRegex validates alphanumeric document numbers (5-20 characters)
var documentNumberRegex = regexp.MustCompile(`^[A-Za-z0-9]{5,20}$`)

// nameRegex validates names: 2-100 chars, no digits
var nameRegex = regexp.MustCompile(`^[^\d]{2,100}$`)

// persianNameRegex validates Persian/Arabic names: 2-100 chars, Persian/Arabic Unicode + spaces + ZWNJ, no digits.
var persianNameRegex = regexp.MustCompile(`^[\x{0600}-\x{06FF}\x{FB50}-\x{FDFF}\x{FE70}-\x{FEFF}\x{200C}\s]{2,100}$`)

// handleKYCStatus returns the current KYC status for the authenticated user.
func (a *App) getGoogleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     a.config.GoogleClientID,
		ClientSecret: a.config.GoogleClientSecret,
		RedirectURL:  a.config.GoogleRedirectURI,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

// generateOAuthState generates a secure random state parameter for CSRF protection.
func generateOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// handleGoogleAuth initiates the Google OAuth flow.
// GET /api/user/auth/google
func (a *App) handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	// Check if Google OAuth is configured
	if a.config.GoogleClientID == "" || a.config.GoogleClientSecret == "" {
		a.log().Error("Google OAuth not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.OAuthNotConfigured,
		})
		return
	}

	// Generate secure state parameter for CSRF protection
	state, err := generateOAuthState()
	if err != nil {
		a.log().Error("Failed to generate OAuth state", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Store state in Redis with TTL for validation during callback
	ctx := r.Context()
	clientIP := getClientIP(r)

	if a.redis == nil {
		a.log().Warn("Google OAuth unavailable: Redis not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.OAuthServiceUnavailable,
		})
		return
	}

	stateKey := oauthStateKeyPrefix + state

	// Store metadata with the state (IP address for additional validation)
	stateData := map[string]string{
		"ip":         clientIP,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	stateJSON, _ := json.Marshal(stateData)

	if err := a.redis.Set(ctx, stateKey, string(stateJSON), oauthStateTTL).Err(); err != nil {
		a.log().Error("Failed to store OAuth state in Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Generate the Google OAuth URL
	oauthConfig := a.getGoogleOAuthConfig()
	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	a.log().Info("Google OAuth initiated",
		zap.String("ip", clientIP),
		zap.String("state_prefix", state[:8]+"..."))

	writeJSON(w, http.StatusOK, GoogleAuthResponse{
		AuthURL: authURL,
		State:   state,
	})
}

// handleGoogleCallback handles the Google OAuth callback.
// POST /api/user/auth/google/callback
func (a *App) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	// Check if client IP is currently locked out due to too many failures
	if locked, retryAfter := a.failedLoginTracker.checkLocked(clientIP); locked {
		a.log().Warn("OAuth callback blocked due to too many failed attempts",
			zap.String("ip", clientIP),
			zap.Duration("retry_after", retryAfter))
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":       "too many failed attempts",
			"message":     msg.AccountLocked,
			"retry_after": int(retryAfter.Seconds()) + 1,
		})
		return
	}

	// Parse request body
	var req GoogleAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Validate required fields
	if req.Code == "" {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.OAuthInvalidCode)
		return
	}
	if req.State == "" {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.OAuthInvalidState)
		return
	}

	ctx := r.Context()

	if a.redis == nil {
		a.log().Warn("Google OAuth callback unavailable: Redis not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.OAuthServiceUnavailable,
		})
		return
	}

	// Validate state parameter (CSRF protection)
	stateKey := oauthStateKeyPrefix + req.State
	stateJSON, err := a.redis.Get(ctx, stateKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Record failed attempt
			delay := a.failedLoginTracker.recordFailure(clientIP)
			a.log().Warn("Invalid OAuth state - state not found or expired",
				zap.String("ip", clientIP),
				zap.Duration("next_delay", delay))
			if delay > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
				writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
					"error":       "too many failed attempts",
					"message":     msg.TooManyLoginAttempts,
					"retry_after": int(delay.Seconds()) + 1,
				})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": msg.OAuthInvalidState,
			})
			return
		}
		a.log().Error("Failed to retrieve OAuth state from Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Delete the state immediately to prevent replay attacks
	a.redis.Del(ctx, stateKey)

	// Parse state data for additional validation (optional: IP check)
	var stateData map[string]string
	_ = json.Unmarshal([]byte(stateJSON), &stateData)

	// Check if Google OAuth is configured
	if a.config.GoogleClientID == "" || a.config.GoogleClientSecret == "" {
		a.log().Error("Google OAuth not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.OAuthNotConfigured,
		})
		return
	}

	// Exchange the authorization code for tokens
	oauthConfig := a.getGoogleOAuthConfig()
	token, err := oauthConfig.Exchange(ctx, req.Code)
	if err != nil {
		// Record failed attempt
		delay := a.failedLoginTracker.recordFailure(clientIP)
		a.log().Warn("Failed to exchange OAuth code",
			zap.Error(err),
			zap.String("ip", clientIP),
			zap.Duration("next_delay", delay))
		if delay > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
				"error":       "too many failed attempts",
				"message":     msg.TooManyLoginAttempts,
				"retry_after": int(delay.Seconds()) + 1,
			})
			return
		}

		// Check for specific error types
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": msg.OAuthGoogleFailed,
			})
			return
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": msg.OAuthInvalidCode,
		})
		return
	}

	// Get user info from Google
	googleUser, err := a.getGoogleUserInfo(ctx, token)
	if err != nil {
		a.log().Error("Failed to get Google user info", zap.Error(err))
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": msg.OAuthUserInfoFailed,
		})
		return
	}

	// Validate that email is verified
	if !googleUser.VerifiedEmail {
		a.log().Warn("Google OAuth - unverified email",
			zap.String("google_id", googleUser.ID),
			zap.String("email", googleUser.Email))
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": msg.OAuthEmailNotVerified,
		})
		return
	}

	// Sanitize email
	email := validation.SanitizeEmail(strings.ToLower(googleUser.Email))

	// Build OAuth user info for the service
	oauthUserInfo := models.OAuthUserInfo{
		ProviderUserID: googleUser.ID,
		Email:          email,
		EmailVerified:  googleUser.VerifiedEmail,
		Name:           googleUser.Name,
		GivenName:      googleUser.GivenName,
		FamilyName:     googleUser.FamilyName,
		Picture:        googleUser.Picture,
	}

	// Build OAuth tokens
	oauthTokens := &models.OAuthTokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	// Detect language from Accept-Language header for new OAuth users
	oauthLang := "fa"
	if acceptLang := r.Header.Get("Accept-Language"); acceptLang != "" {
		if strings.Contains(strings.ToLower(acceptLang), "en") {
			oauthLang = "en"
		}
	}

	// Process OAuth login/registration using the service
	oauthResult, err := a.oauthService.ProcessOAuthLogin(ctx, models.OAuthProviderGoogle, oauthUserInfo, oauthTokens, oauthLang)
	if err != nil {
		if errors.Is(err, service.ErrOAuthAccountExists) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":   "oauth_account_exists",
				"message": msg.OAuthAccountExists,
			})
			return
		}
		a.log().Error("Failed to process Google OAuth",
			zap.Error(err),
			zap.String("google_id", googleUser.ID),
			zap.String("email", email))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Successful OAuth - clear failed attempt tracking
	a.failedLoginTracker.recordSuccess(clientIP)

	// Get user roles
	roles, err := a.getUserRoles(ctx, oauthResult.UserID)
	if err != nil {
		a.log().Error("Failed to get user roles", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Get device info and IP address
	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := clientIP

	// Login using Auth service (creates session if Redis is available)
	tokenPair, sessionID, err := a.auth.Login(ctx, oauthResult.UserID, roles, deviceInfo, ipAddress)
	if err != nil {
		a.log().Error("Failed to create session", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": msg.InternalError,
		})
		return
	}

	// Log successful OAuth login
	action := "logged in"
	if oauthResult.IsNewUser {
		action = "registered"
	} else if oauthResult.WasLinked {
		action = "linked OAuth to existing account"
	}
	a.log().Info("User "+action+" via Google OAuth",
		zap.String("user_id", oauthResult.UserID),
		zap.String("google_id", googleUser.ID),
		zap.String("ip", clientIP),
		zap.Bool("new_user", oauthResult.IsNewUser),
		zap.Bool("was_linked", oauthResult.WasLinked),
		zap.Bool("has_password", oauthResult.HasPassword))

	// Initialize new OAuth users (sends welcome email without verification link)
	if oauthResult.IsNewUser {
		a.initializeNewUser(ctx, NewUserInitParams{
			UserID:        oauthResult.UserID,
			Email:         email,
			EmailVerified: true,
			Lang:          oauthLang,
		})
	}

	if sessionID != "" {
		a.log().Debug("Session created",
			zap.String("user_id", oauthResult.UserID),
			zap.String("session_id", sessionID))
	}

	a.setRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:   tokenPair.AccessToken,
		RefreshToken:  "", // Sent via httpOnly cookie
		ExpiresAt:     tokenPair.ExpiresAt,
		EmailVerified: true, // Google emails are verified
	})
}

// getGoogleUserInfo fetches user info from Google's userinfo API.
func (a *App) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*GoogleUserInfo, error) {
	client := a.getGoogleOAuthConfig().Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google userinfo API returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &userInfo, nil
}

// Note: OAuth user management is now handled by service.OAuthService
// See: internal/service/oauth_service.go

// handleGetContestMyResult returns the authenticated user's result for a specific contest.
