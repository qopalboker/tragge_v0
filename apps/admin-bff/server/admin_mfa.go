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

	"github.com/Parsaeffatravesh/tragge/packages/auth"

	"github.com/go-chi/chi/v5"
)

const (
	adminMFAChallengeField       = "challenge"
	adminMFAErrorKey             = "error"
	adminMFAExpiresAtField       = "expires_at"
	adminMFAFailureCategoryField = "failure_category"
	adminMFAFailureResponse      = "additional authentication failed"
	adminMFAReasonField          = "reason"
	adminMFAResetValue           = "reset"
	adminMFAStatusActive         = "active"
	adminMFAStatusField          = "status"
	adminMFAStatusPending        = "pending"
	adminMFAUnavailableResponse  = "additional authentication unavailable"
)

type adminMFALoginResponse struct {
	MFARequired        bool      `json:"mfa_required"`
	EnrollmentRequired bool      `json:"enrollment_required"`
	Challenge          string    `json:"challenge"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type adminMFAChallengeRequest struct {
	Challenge    string `json:"challenge"`
	Code         string `json:"code,omitempty"`
	RecoveryCode string `json:"recovery_code,omitempty"`
}

func adminMFAClientBinding(r *http.Request) string {
	sum := sha256.Sum256([]byte(getAdminClientIP(r) + "\x00" + r.Header.Get("User-Agent")))
	return hex.EncodeToString(sum[:])
}

func (a *App) issueAdminMFAChallenge(ctx context.Context, r *http.Request, userID, email string, state adminSecurityState, stage, encryptedSecret string) (string, time.Time, error) {
	if a.mfaChallenges == nil {
		return "", time.Time{}, auth.ErrAdminMFAUnavailable
	}
	now := time.Now().UTC()
	expiresAt := now.Add(a.config.AdminMFA.ChallengeTTL)
	challenge, err := a.mfaChallenges.Issue(ctx, auth.AdminMFAChallenge{
		UserID: userID, Email: email, Roles: state.Roles,
		Permissions: effectiveAdminPermissions(state), SecurityFingerprint: state.fingerprint(), Stage: stage,
		SecretCiphertext: encryptedSecret, ClientBinding: adminMFAClientBinding(r),
		IssuedAt: now, ExpiresAt: expiresAt,
	})
	return challenge, expiresAt, err
}

func (a *App) validateAdminMFAChallenge(r *http.Request, raw, stage string) (*auth.AdminMFAChallenge, error) {
	if a.mfaChallenges == nil {
		return nil, auth.ErrAdminMFAUnavailable
	}
	challenge, err := a.mfaChallenges.Get(r.Context(), raw)
	if err != nil || challenge.Stage != stage || challenge.ClientBinding != adminMFAClientBinding(r) {
		return nil, auth.ErrAdminMFAInvalid
	}
	state, err := a.loadAdminSecurityState(r.Context(), challenge.UserID)
	if err != nil || state.Status != adminMFAStatusActive || !state.hasRole(auth.RoleSuperAdmin) || state.fingerprint() != challenge.SecurityFingerprint || !sameStringSet(state.Roles, challenge.Roles) || !sameStringSet(effectiveAdminPermissions(state), challenge.Permissions) {
		return nil, auth.ErrAdminMFAInvalid
	}
	return challenge, nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	want := make(map[string]int, len(left))
	for _, value := range left {
		want[value]++
	}
	for _, value := range right {
		want[value]--
		if want[value] < 0 {
			return false
		}
	}
	return true
}

func (a *App) consumeAdminMFAChallenge(ctx context.Context, raw string) error {
	_, err := a.mfaChallenges.Consume(ctx, raw)
	return err
}

func (a *App) completeSuperAdminMFALogin(w http.ResponseWriter, r *http.Request, challenge *auth.AdminMFAChallenge) {
	if err := a.distributedLoginLockout.Success(r.Context(), "ip:"+getAdminClientIP(r), "account:"+challenge.Email); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMsg.InternalError})
		return
	}
	a.failedAdminLoginTracker.recordSuccess(getAdminClientIP(r))
	pair, sessionID, err := a.auth.LoginWithPermissionsAndMFA(
		r.Context(), challenge.UserID, challenge.Roles, challenge.Permissions,
		auth.MFAAssuranceSuperAdminTOTPV1, r.Header.Get("User-Agent"), getAdminClientIP(r),
	)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMsg.InternalError})
		return
	}
	if _, err := a.pool.Primary().ExecContext(r.Context(), `INSERT INTO audit_logs (actor_user_id,action,target_type,target_id,payload_json) VALUES ($1,'admin.mfa.login.succeeded','auth',$1,'{"assurance":"super_admin_totp_v1"}')`, challenge.UserID); err != nil {
		_ = a.auth.Session.Delete(r.Context(), sessionID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	setAdminRefreshTokenCookie(w, r, pair.RefreshToken)
	writeJSON(w, http.StatusOK, adminAuthResponse{AccessToken: pair.AccessToken, ExpiresAt: pair.ExpiresAt})
}

func (a *App) recordAdminMFAFailure(r *http.Request, challenge *auth.AdminMFAChallenge, category string) {
	if challenge == nil {
		return
	}
	if a.distributedLoginLockout != nil {
		_, _ = a.distributedLoginLockout.Failure(r.Context(), "ip:"+getAdminClientIP(r), "account:"+challenge.Email)
	}
	if a.failedAdminLoginTracker != nil {
		a.failedAdminLoginTracker.recordFailure(getAdminClientIP(r))
	}
	a.logAuditEvent(r.Context(), challenge.UserID, "admin.mfa.login.denied", "auth", challenge.UserID, map[string]string{adminMFAFailureCategoryField: category})
}

func (a *App) handleAdminMFAEnrollmentStart(w http.ResponseWriter, r *http.Request) {
	var req adminMFAChallengeRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&req) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	initial, err := a.validateAdminMFAChallenge(r, req.Challenge, "enroll")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	var alreadyEnabled bool
	if err := a.pool.Primary().QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM admin_mfa_credentials WHERE user_id=$1)`, initial.UserID).Scan(&alreadyEnabled); err != nil || alreadyEnabled {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	if err := a.consumeAdminMFAChallenge(r.Context(), req.Challenge); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	secret, err := auth.GenerateAdminTOTPSecret()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	encrypted, err := auth.EncryptAdminTOTPSecret(secret, a.config.AdminMFA.EncryptionKey)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	state, err := a.loadAdminSecurityState(r.Context(), initial.UserID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	challenge, expiresAt, err := a.issueAdminMFAChallenge(r.Context(), r, initial.UserID, initial.Email, state, "enroll_verify", encrypted)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	a.logAuditEvent(r.Context(), initial.UserID, "admin.mfa.enrollment.started", "auth", initial.UserID, map[string]string{adminMFAStatusField: adminMFAStatusPending})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		adminMFAChallengeField: challenge, adminMFAExpiresAtField: expiresAt, "secret": secret,
		"provisioning_uri": auth.AdminMFAProvisioningURI(a.config.AdminMFA.Issuer, initial.Email, secret),
	})
}

func (a *App) handleAdminMFAEnrollmentVerify(w http.ResponseWriter, r *http.Request) {
	var req adminMFAChallengeRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&req) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	challenge, err := a.validateAdminMFAChallenge(r, req.Challenge, "enroll_verify")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	secret, err := auth.DecryptAdminTOTPSecret(challenge.SecretCiphertext, a.config.AdminMFA.EncryptionKey)
	counter, valid := auth.MatchAdminTOTPCounter(secret, strings.TrimSpace(req.Code), time.Now().UTC())
	if err != nil || !valid {
		a.recordAdminMFAFailure(r, challenge, "invalid_enrollment_code")
		a.logAuditEvent(r.Context(), challenge.UserID, "admin.mfa.enrollment.failed", "auth", challenge.UserID, map[string]string{adminMFAFailureCategoryField: "invalid_code"})
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	if err := a.consumeAdminMFAChallenge(r.Context(), req.Challenge); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	codes, err := auth.GenerateAdminMFARecoveryCodes()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	tx, err := a.pool.Primary().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO admin_mfa_credentials (user_id, secret_ciphertext, last_totp_counter, enabled_at) VALUES ($1,$2,$3,NOW())`, challenge.UserID, challenge.SecretCiphertext, counter); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	for _, code := range codes {
		digest, digestErr := auth.AdminMFARecoveryDigest(code, a.config.AdminMFA.RecoveryPepper)
		if digestErr != nil {
			err = digestErr
			break
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO admin_mfa_recovery_codes (user_id,generation,code_digest) VALUES ($1,1,$2)`, challenge.UserID, digest)
		if err != nil {
			break
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO audit_logs (actor_user_id,action,target_type,target_id,payload_json) VALUES ($1,'admin.mfa.enrollment.completed','auth',$1,'{"assurance":"super_admin_totp_v1"}')`, challenge.UserID)
	}
	if err != nil || tx.Commit() != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMFAUnavailableResponse})
		return
	}
	// Recovery codes are returned exactly once and are never stored in plaintext.
	writeAdminMFAEnrollmentSuccess(w, r, a, challenge, codes)
}

func writeAdminMFAEnrollmentSuccess(w http.ResponseWriter, r *http.Request, a *App, challenge *auth.AdminMFAChallenge, codes []string) {
	pair, _, err := a.auth.LoginWithPermissionsAndMFA(r.Context(), challenge.UserID, challenge.Roles, challenge.Permissions, auth.MFAAssuranceSuperAdminTOTPV1, r.Header.Get("User-Agent"), getAdminClientIP(r))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: adminMsg.InternalError})
		return
	}
	setAdminRefreshTokenCookie(w, r, pair.RefreshToken)
	writeJSON(w, http.StatusOK, map[string]interface{}{"access_token": pair.AccessToken, adminMFAExpiresAtField: pair.ExpiresAt, "recovery_codes": codes})
}

func (a *App) handleAdminMFAVerify(w http.ResponseWriter, r *http.Request) {
	var req adminMFAChallengeRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&req) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	challenge, err := a.validateAdminMFAChallenge(r, req.Challenge, "verify")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	if strings.TrimSpace(req.Code) != "" && strings.TrimSpace(req.RecoveryCode) != "" {
		a.recordAdminMFAFailure(r, challenge, "ambiguous_credential")
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	if strings.TrimSpace(req.Code) != "" {
		var encrypted string
		if err := a.pool.Primary().QueryRowContext(r.Context(), `SELECT secret_ciphertext FROM admin_mfa_credentials WHERE user_id=$1`, challenge.UserID).Scan(&encrypted); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
			return
		}
		secret, err := auth.DecryptAdminTOTPSecret(encrypted, a.config.AdminMFA.EncryptionKey)
		counter, valid := auth.MatchAdminTOTPCounter(secret, strings.TrimSpace(req.Code), time.Now().UTC())
		if err != nil || !valid {
			a.recordAdminMFAFailure(r, challenge, "invalid_code")
			writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
			return
		}
		result, err := a.pool.Primary().ExecContext(r.Context(), `UPDATE admin_mfa_credentials SET last_totp_counter=$2,updated_at=NOW() WHERE user_id=$1 AND (last_totp_counter IS NULL OR last_totp_counter < $2)`, challenge.UserID, counter)
		if err != nil || rowsAffected(result) != 1 {
			a.recordAdminMFAFailure(r, challenge, "code_replay")
			writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
			return
		}
	} else if strings.TrimSpace(req.RecoveryCode) != "" {
		digest, err := auth.AdminMFARecoveryDigest(req.RecoveryCode, a.config.AdminMFA.RecoveryPepper)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
			return
		}
		result, err := a.pool.Primary().ExecContext(r.Context(), `UPDATE admin_mfa_recovery_codes SET used_at=NOW() WHERE user_id=$1 AND generation=(SELECT recovery_generation FROM admin_mfa_credentials WHERE user_id=$1) AND code_digest=$2 AND used_at IS NULL`, challenge.UserID, digest)
		if err != nil || rowsAffected(result) != 1 {
			a.recordAdminMFAFailure(r, challenge, "invalid_recovery_code")
			writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
			return
		}
	} else {
		a.recordAdminMFAFailure(r, challenge, "missing_credential")
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	if err := a.consumeAdminMFAChallenge(r.Context(), req.Challenge); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{adminMFAErrorKey: adminMFAFailureResponse})
		return
	}
	a.completeSuperAdminMFALogin(w, r, challenge)
}

func rowsAffected(result sql.Result) int64 {
	if result == nil {
		return 0
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return count
}

func (a *App) handleAdminMFAReset(w http.ResponseWriter, r *http.Request) {
	actorID := auth.GetUserID(r.Context())
	targetID := chi.URLParam(r, "user_id")
	var reason string
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&struct {
		Reason *string `json:"reason"`
	}{Reason: &reason}) != nil || strings.TrimSpace(reason) == "" {
		a.auditSensitiveDenial(r.Context(), actorID, actionAdminMFAReset, targetID, "mandatory_reason_denied")
		writeJSON(w, http.StatusBadRequest, map[string]string{adminMFAErrorKey: "reason is required"})
		return
	}
	var targetIsSuper bool
	if err := a.pool.Primary().QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$1 AND ro.name='super_admin')`, targetID).Scan(&targetIsSuper); err != nil || !targetIsSuper {
		writeJSON(w, http.StatusNotFound, map[string]string{adminMFAErrorKey: "resource not found"})
		return
	}
	tx, err := a.pool.Primary().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: "MFA reset unavailable"})
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM admin_mfa_credentials WHERE user_id=$1`, targetID); err == nil {
		payload, _ := json.Marshal(map[string]string{adminMFAReasonField: strings.TrimSpace(reason), "result": adminMFAResetValue})
		_, err = tx.ExecContext(r.Context(), `INSERT INTO audit_logs (actor_user_id,action,target_type,target_id,payload_json) VALUES ($1,'admin.mfa.reset','user',$2,$3)`, actorID, targetID, payload)
	}
	if err != nil || tx.Commit() != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{adminMFAErrorKey: "MFA reset unavailable"})
		return
	}
	if a.auth.Session != nil {
		_ = a.auth.Session.DeleteAllForUser(r.Context(), targetID)
	}
	if a.reauthentication != nil {
		_ = a.reauthentication.RevokeActor(r.Context(), targetID)
	}
	if a.mfaChallenges != nil {
		_ = a.mfaChallenges.RevokeUser(r.Context(), targetID)
	}
	writeJSON(w, http.StatusOK, map[string]string{adminMFAStatusField: adminMFAResetValue})
}
