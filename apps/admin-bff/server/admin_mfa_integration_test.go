package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- RFC 6238 compatibility requires HMAC-SHA1.
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	redis "github.com/redis/go-redis/v9"
)

const (
	sec007CodeField           = "code"
	sec007RecoveryCodeField   = "recovery_code"
	sec007SuperAdminID        = "11111111-1111-4111-8111-111111111111"
	sec007UsersEditPermission = "users.edit"
)

func requireSEC007Runtime(t *testing.T) (string, string) {
	t.Helper()
	dsn, redisAddr := os.Getenv("SEC007_POSTGRES_DSN"), os.Getenv("SEC007_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("SEC007_POSTGRES_DSN and SEC007_REDIS_ADDR are required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing non-local or non-test PostgreSQL DSN")
	}
	if !strings.HasPrefix(redisAddr, "127.0.0.1:") && !strings.HasPrefix(redisAddr, "localhost:") {
		t.Fatal("refusing non-local Redis")
	}
	return dsn, redisAddr
}

func sec007Hex(seed byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return hex.EncodeToString(raw)
}

func sec007TOTP(secret string, counter int64) string {
	if counter < 0 {
		return ""
	}
	raw, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(counter))
	mac := hmac.New(sha1.New, raw)
	_, _ = mac.Write(message)
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := (uint32(digest[offset])&0x7f)<<24 | uint32(digest[offset+1])<<16 | uint32(digest[offset+2])<<8 | uint32(digest[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func sec007WrongTOTP(valid string) string {
	last := byte('0')
	if valid[len(valid)-1] == '0' {
		last = '1'
	}
	return valid[:len(valid)-1] + string(last)
}

type sec007Harness struct {
	t        *testing.T
	ctx      context.Context
	db       *sql.DB
	redis    *redis.Client
	app      *App
	password string
}

func newSEC007Harness(t *testing.T) *sec007Harness {
	t.Helper()
	dsn, redisAddr := requireSEC007Runtime(t)
	ctx := context.Background()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 13})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`DROP TABLE IF EXISTS admin_mfa_recovery_codes,admin_mfa_credentials,audit_logs,role_permissions,user_roles,permissions,roles,users CASCADE`,
		`CREATE TABLE users (id UUID PRIMARY KEY,email TEXT UNIQUE NOT NULL,password_hash TEXT NOT NULL,status TEXT NOT NULL,is_system_account BOOLEAN NOT NULL DEFAULT FALSE,totp_secret TEXT)`,
		`CREATE TABLE roles (id BIGSERIAL PRIMARY KEY,name TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE permissions (id BIGSERIAL PRIMARY KEY,name TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE user_roles (user_id UUID REFERENCES users(id),role_id BIGINT REFERENCES roles(id),PRIMARY KEY(user_id,role_id))`,
		`CREATE TABLE role_permissions (role_id BIGINT REFERENCES roles(id),permission_id BIGINT REFERENCES permissions(id),PRIMARY KEY(role_id,permission_id))`,
		`CREATE TABLE audit_logs (id BIGSERIAL PRIMARY KEY,actor_user_id UUID,action TEXT NOT NULL,target_type TEXT NOT NULL,target_id UUID,payload_json JSONB,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}
	for _, statement := range statements {
		if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	migrationPath := filepath.Join("..", "..", "..", "packages", "db", "migrations", "0100_admin_super_mfa.up.sql")
	// #nosec G304 -- the path is a fixed repository migration, not user input.
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	loginCredential := "SEC007-local-" + strings.Repeat("x", 32)
	hash, err := auth.HashPassword(loginCredential, auth.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct{ id, email, role string }{
		{sec007SuperAdminID, "super@test.invalid", auth.RoleSuperAdmin},
		{"22222222-2222-4222-8222-222222222222", "support@test.invalid", auth.RoleSupportAdmin},
		{"33333333-3333-4333-8333-333333333333", "concurrent@test.invalid", auth.RoleSuperAdmin},
	} {
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,status) VALUES($1,$2,$3,'active')`, seed.id, seed.email, hash); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO roles(name) VALUES($1) ON CONFLICT DO NOTHING`, seed.role); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO user_roles SELECT $1,id FROM roles WHERE name=$2`, seed.id, seed.role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO permissions(name) VALUES('users.edit'); INSERT INTO role_permissions SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='super_admin'`); err != nil {
		t.Fatal(err)
	}
	authConfig := auth.DefaultConfig()
	authConfig.Context = auth.ContextAdmin
	authConfig.JWTSecret = "SEC007-admin-access-key-material-not-production"
	authConfig.JWTRefreshSecret = "SEC007-admin-refresh-key-material-not-production"
	authConfig.JWTIssuer = "tragge-admin-auth"
	authConfig.JWTAudience = auth.AudienceAdmin
	authConfig.Redis = rdb
	authConfig.SessionPrefix = auth.AdminSessionPrefix
	authConfig.SessionTTL = time.Hour
	authService := auth.New(authConfig)
	keyHex, pepperHex := sec007Hex(1), sec007Hex(65)
	mfaConfig, err := auth.ValidateAdminMFAConfig("test", keyHex, pepperHex, auth.AdminMFAIssuer, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	lockout, err := ratelimit.NewLoginLockout(rdb, ratelimit.LockoutConfig{Namespace: "sec007", Threshold: 20, LockFor: time.Minute, Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observability.New(ctx, observability.Config{Service: "sec007-test", Env: "test", LogLevel: adminMFAErrorKey})
	if err != nil {
		t.Fatal(err)
	}
	reauth, err := auth.NewReauthenticationService(auth.NewRedisReauthenticationGrantStore(rdb, auth.AdminReauthenticationPrefix), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{pool: db.NewPoolFromDB(sqlDB), auth: authService, config: &Config{AdminMFA: mfaConfig}, obs: obs, circuits: NewCircuitBreakers(CircuitBreakerConfig{Logger: obs.Logger.Logger}), failedAdminLoginTracker: newFailedAdminLoginTracker(), distributedLoginLockout: lockout, reauthentication: reauth, mfaChallenges: auth.NewRedisAdminMFAChallengeStore(rdb, auth.AdminMFAChallengePrefix)}
	h := &sec007Harness{t: t, ctx: ctx, db: sqlDB, redis: rdb, app: app, password: loginCredential}
	t.Cleanup(func() {
		app.failedAdminLoginTracker.stop()
		_ = obs.Shutdown(context.Background())
		_, _ = sqlDB.ExecContext(context.Background(), statements[0])
		_ = rdb.FlushDB(context.Background()).Err()
		_ = rdb.Close()
		_ = sqlDB.Close()
	})
	return h
}

func (h *sec007Harness) post(path string, body interface{}, handler http.HandlerFunc) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequestWithContext(h.ctx, http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sec007-test-agent")
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func decodeSEC007(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode status=%d body=%s: %v", rec.Code, rec.Body.String(), err)
	}
	return result
}

func (h *sec007Harness) login(email string) (*httptest.ResponseRecorder, map[string]interface{}) {
	rec := h.post("/api/admin/auth/login", adminLoginRequest{Email: email, Password: h.password}, h.app.handleAdminLogin)
	return rec, decodeSEC007(h.t, rec)
}

//nolint:gocyclo // The linear scenario intentionally shares one real PostgreSQL/Redis lifecycle.
func TestSEC007SuperAdminMFAPostgresRedisRuntime(t *testing.T) {
	h := newSEC007Harness(t)
	t.Run("support admin remains isolated and permission limited", func(t *testing.T) {
		rec, data := h.login("support@test.invalid")
		if rec.Code != http.StatusOK || data["access_token"] == nil || data["mfa_required"] != nil {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	rec, login := h.login("super@test.invalid")
	if rec.Code != http.StatusAccepted || login["mfa_required"] != true || login["enrollment_required"] != true {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	preMFASessions, err := h.app.auth.Session.GetUserSessions(h.ctx, sec007SuperAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preMFASessions) != 0 {
		t.Fatalf("password-only Super Admin obtained %d sessions", len(preMFASessions))
	}
	start := h.post("/api/admin/auth/mfa/enrollment/start", map[string]string{adminMFAChallengeField: login[adminMFAChallengeField].(string)}, h.app.handleAdminMFAEnrollmentStart)
	startData := decodeSEC007(t, start)
	if start.Code != http.StatusOK || !strings.HasPrefix(startData["provisioning_uri"].(string), "otpauth://totp/") {
		t.Fatalf("start=%d %s", start.Code, start.Body.String())
	}
	secret := startData["secret"].(string)
	enrollmentChallenge := startData[adminMFAChallengeField].(string)
	counter := time.Now().UTC().Unix() / 30
	validEnrollmentCode := sec007TOTP(secret, counter)
	badEnrollment := h.post("/api/admin/auth/mfa/enrollment/verify", map[string]string{adminMFAChallengeField: enrollmentChallenge, sec007CodeField: sec007WrongTOTP(validEnrollmentCode)}, h.app.handleAdminMFAEnrollmentVerify)
	if badEnrollment.Code != http.StatusUnauthorized || strings.Contains(badEnrollment.Body.String(), secret) {
		t.Fatalf("bad enrollment=%d %s", badEnrollment.Code, badEnrollment.Body.String())
	}
	attemptKeys, err := h.redis.Keys(h.ctx, "sec006:lockout:sec007:*:attempts").Result()
	if err != nil || len(attemptKeys) != 2 {
		t.Fatalf("MFA failure counters keys=%d err=%v", len(attemptKeys), err)
	}
	for _, key := range attemptKeys {
		if attempts, getErr := h.redis.Get(h.ctx, key).Int(); getErr != nil || attempts != 1 {
			t.Fatalf("MFA failure counter before password retry=%d err=%v", attempts, getErr)
		}
	}
	passwordRetry, _ := h.login("super@test.invalid")
	if passwordRetry.Code != http.StatusAccepted {
		t.Fatalf("password retry status=%d", passwordRetry.Code)
	}
	for _, key := range attemptKeys {
		if attempts, getErr := h.redis.Get(h.ctx, key).Int(); getErr != nil || attempts != 1 {
			t.Fatalf("password step cleared MFA failure counter: attempts=%d err=%v", attempts, getErr)
		}
	}
	enroll := h.post("/api/admin/auth/mfa/enrollment/verify", map[string]string{adminMFAChallengeField: enrollmentChallenge, sec007CodeField: validEnrollmentCode}, h.app.handleAdminMFAEnrollmentVerify)
	enrolled := decodeSEC007(t, enroll)
	if enroll.Code != http.StatusOK || enrolled["access_token"] == nil || len(enrolled["recovery_codes"].([]interface{})) != auth.AdminMFARecoveryCodeCount {
		t.Fatalf("enroll=%d %s", enroll.Code, enroll.Body.String())
	}
	claims, err := h.app.auth.Token.ValidateAccessToken(enrolled["access_token"].(string))
	if err != nil || claims.MFAAssurance != auth.MFAAssuranceSuperAdminTOTPV1 {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
	if strings.Contains(enroll.Body.String(), secret) || strings.Contains(enroll.Body.String(), "provisioning_uri") {
		t.Fatal("enrollment completion returned the TOTP secret")
	}
	var stored, legacy sql.NullString
	if err := h.db.QueryRowContext(h.ctx, `SELECT c.secret_ciphertext,u.totp_secret FROM admin_mfa_credentials c JOIN users u ON u.id=c.user_id WHERE c.user_id=$1`, sec007SuperAdminID).Scan(&stored, &legacy); err != nil || !strings.HasPrefix(stored.String, auth.AdminMFACiphertextPrefix) || legacy.Valid {
		t.Fatalf("stored=%v legacy=%v err=%v", stored, legacy, err)
	}
	if replay := h.post("/api/admin/auth/mfa/enrollment/verify", map[string]string{adminMFAChallengeField: enrollmentChallenge, sec007CodeField: sec007TOTP(secret, counter)}, h.app.handleAdminMFAEnrollmentVerify); replay.Code != http.StatusUnauthorized {
		t.Fatalf("enrollment replay=%d", replay.Code)
	}

	t.Run("concurrent enrollment creates one credential", func(t *testing.T) {
		const attempts = 6
		challenges := make([]string, attempts)
		secrets := make([]string, attempts)
		for index := 0; index < attempts; index++ {
			loginRec, loginData := h.login("concurrent@test.invalid")
			if loginRec.Code != http.StatusAccepted {
				t.Fatalf("login %d", loginRec.Code)
			}
			startRec := h.post("/api/admin/auth/mfa/enrollment/start", map[string]string{adminMFAChallengeField: loginData[adminMFAChallengeField].(string)}, h.app.handleAdminMFAEnrollmentStart)
			startData := decodeSEC007(t, startRec)
			if startRec.Code != http.StatusOK {
				t.Fatalf("start %d", startRec.Code)
			}
			challenges[index] = startData[adminMFAChallengeField].(string)
			secrets[index] = startData["secret"].(string)
		}
		var succeeded atomic.Int32
		var wg sync.WaitGroup
		for index := range challenges {
			wg.Add(1)
			go func(challenge, candidateSecret string) {
				defer wg.Done()
				code := sec007TOTP(candidateSecret, time.Now().UTC().Unix()/30)
				if rec := h.post("/api/admin/auth/mfa/enrollment/verify", map[string]string{adminMFAChallengeField: challenge, sec007CodeField: code}, h.app.handleAdminMFAEnrollmentVerify); rec.Code == http.StatusOK {
					succeeded.Add(1)
				}
			}(challenges[index], secrets[index])
		}
		wg.Wait()
		if succeeded.Load() != 1 {
			t.Fatalf("successful concurrent enrollments=%d", succeeded.Load())
		}
	})

	t.Run("password and permission changes invalidate pending challenges", func(t *testing.T) {
		_, passwordLogin := h.login("super@test.invalid")
		var originalHash string
		if err := h.db.QueryRowContext(h.ctx, `SELECT password_hash FROM users WHERE id=$1`, sec007SuperAdminID).Scan(&originalHash); err != nil {
			t.Fatal(err)
		}
		changedHash, err := auth.HashPassword("different-local-password", auth.DefaultArgon2idParams())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.ExecContext(h.ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, changedHash, sec007SuperAdminID); err != nil {
			t.Fatal(err)
		}
		if rec := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: passwordLogin[adminMFAChallengeField].(string), sec007CodeField: sec007TOTP(secret, counter+1)}, h.app.handleAdminMFAVerify); rec.Code != http.StatusUnauthorized {
			t.Fatalf("password-change challenge=%d", rec.Code)
		}
		if _, err := h.db.ExecContext(h.ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, originalHash, sec007SuperAdminID); err != nil {
			t.Fatal(err)
		}
		_, permissionLogin := h.login("super@test.invalid")
		if _, err := h.db.ExecContext(h.ctx, `DELETE FROM role_permissions WHERE role_id=(SELECT id FROM roles WHERE name='super_admin')`); err != nil {
			t.Fatal(err)
		}
		if rec := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: permissionLogin[adminMFAChallengeField].(string), sec007CodeField: sec007TOTP(secret, counter+1)}, h.app.handleAdminMFAVerify); rec.Code != http.StatusUnauthorized {
			t.Fatalf("permission-change challenge=%d", rec.Code)
		}
		if _, err := h.db.ExecContext(h.ctx, `INSERT INTO role_permissions SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='super_admin' ON CONFLICT DO NOTHING`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("TOTP counter is single-use under concurrency", func(t *testing.T) {
		const attempts = 12
		challenges := make([]string, attempts)
		for i := range challenges {
			rec, data := h.login("super@test.invalid")
			if rec.Code != http.StatusAccepted {
				t.Fatalf("login %d", rec.Code)
			}
			challenges[i] = data[adminMFAChallengeField].(string)
		}
		code := sec007TOTP(secret, counter+1)
		var success atomic.Int32
		var wg sync.WaitGroup
		for _, challenge := range challenges {
			wg.Add(1)
			go func(ch string) {
				defer wg.Done()
				rec := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: ch, sec007CodeField: code}, h.app.handleAdminMFAVerify)
				if rec.Code == http.StatusOK {
					success.Add(1)
				}
			}(challenge)
		}
		wg.Wait()
		if success.Load() != 1 {
			t.Fatalf("successful TOTP consumers=%d", success.Load())
		}
	})

	t.Run("recovery code is single-use under concurrency", func(t *testing.T) {
		// The preceding replay test intentionally crosses the local brute-force
		// threshold. Reset only this synthetic actor before exercising the separate
		// recovery-code concurrency property.
		h.app.failedAdminLoginTracker.recordSuccess("192.0.2.1")
		if err := h.app.distributedLoginLockout.Success(h.ctx, "ip:192.0.2.1", "account:super@test.invalid"); err != nil {
			t.Fatal(err)
		}
		code := enrolled["recovery_codes"].([]interface{})[0].(string)
		const attempts = 8
		challenges := make([]string, attempts)
		for index := range challenges {
			loginRec, login := h.login("super@test.invalid")
			if loginRec.Code != http.StatusAccepted {
				t.Fatalf("recovery login=%d %s", loginRec.Code, loginRec.Body.String())
			}
			challenges[index] = login[adminMFAChallengeField].(string)
		}
		var succeeded atomic.Int32
		var wg sync.WaitGroup
		for _, challenge := range challenges {
			wg.Add(1)
			go func(ch string) {
				defer wg.Done()
				if rec := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: ch, sec007RecoveryCodeField: code}, h.app.handleAdminMFAVerify); rec.Code == http.StatusOK {
					succeeded.Add(1)
				}
			}(challenge)
		}
		wg.Wait()
		if succeeded.Load() != 1 {
			t.Fatalf("successful recovery consumers=%d", succeeded.Load())
		}
		h.app.failedAdminLoginTracker.recordSuccess("192.0.2.1")
		if err := h.app.distributedLoginLockout.Success(h.ctx, "ip:192.0.2.1", "account:super@test.invalid"); err != nil {
			t.Fatal(err)
		}
		replayRec, replayLogin := h.login("super@test.invalid")
		if replayRec.Code != http.StatusAccepted {
			t.Fatalf("replay login=%d %s", replayRec.Code, replayRec.Body.String())
		}
		if replay := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: replayLogin[adminMFAChallengeField].(string), sec007RecoveryCodeField: code}, h.app.handleAdminMFAVerify); replay.Code != http.StatusUnauthorized {
			t.Fatalf("recovery replay=%d", replay.Code)
		}
	})

	t.Run("mandatory login audit failure revokes the new session", func(t *testing.T) {
		actor := sec007SuperAdminID
		before, err := h.app.auth.Session.GetUserSessions(h.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.ExecContext(h.ctx, `CREATE OR REPLACE FUNCTION sec007_reject_login_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='admin.mfa.login.succeeded' THEN RAISE EXCEPTION 'controlled'; END IF; RETURN NEW; END $$; CREATE TRIGGER sec007_reject_login_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION sec007_reject_login_audit()`); err != nil {
			t.Fatal(err)
		}
		_, login := h.login("super@test.invalid")
		code := enrolled["recovery_codes"].([]interface{})[1].(string)
		denied := h.post("/api/admin/auth/mfa/verify", map[string]string{adminMFAChallengeField: login[adminMFAChallengeField].(string), sec007RecoveryCodeField: code}, h.app.handleAdminMFAVerify)
		if denied.Code != http.StatusServiceUnavailable {
			t.Fatalf("audit-failed login=%d %s", denied.Code, denied.Body.String())
		}
		after, err := h.app.auth.Session.GetUserSessions(h.ctx, actor)
		if err != nil || len(after) != len(before) {
			t.Fatalf("session leaked after audit failure: before=%d after=%d err=%v", len(before), len(after), err)
		}
		if _, err := h.db.ExecContext(h.ctx, `DROP TRIGGER sec007_reject_login_audit ON audit_logs; DROP FUNCTION sec007_reject_login_audit()`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("audited reset rolls back on audit failure then revokes state", func(t *testing.T) {
		actor := sec007SuperAdminID
		supportDenied := httptest.NewRecorder()
		supportRequest := httptest.NewRequestWithContext(h.ctx, http.MethodPost, "/api/admin/users/"+actor+"/mfa/reset", nil)
		supportContext := context.WithValue(supportRequest.Context(), auth.ClaimsKey, &auth.Claims{Roles: []string{auth.RoleSupportAdmin}})
		h.app.auth.Middleware.RequireSuperAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("Support Admin reached MFA reset handler")
		})).ServeHTTP(supportDenied, supportRequest.WithContext(supportContext))
		if supportDenied.Code != http.StatusForbidden {
			t.Fatalf("Support Admin reset status=%d", supportDenied.Code)
		}
		sessionID, err := h.app.auth.Session.Create(h.ctx, &auth.Session{UserID: actor, Roles: []string{auth.RoleSuperAdmin}, Permissions: []string{sec007UsersEditPermission}, MFAAssurance: auth.MFAAssuranceSuperAdminTOTPV1})
		if err != nil {
			t.Fatal(err)
		}
		state, err := h.app.loadAdminSecurityState(h.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		issue := func() string {
			grant, _, err := h.app.reauthentication.Issue(h.ctx, auth.ReauthenticationExpectation{Context: auth.ContextAdmin, ActorID: actor, SessionID: sessionID, Action: actionAdminMFAReset, ResourceID: actor, SecurityFingerprint: state.fingerprint()})
			if err != nil {
				t.Fatal(err)
			}
			return grant
		}
		invoke := func(grant string) *httptest.ResponseRecorder {
			body, _ := json.Marshal(map[string]string{adminMFAReasonField: "controlled SEC-007 recovery"})
			req := httptest.NewRequestWithContext(h.ctx, http.MethodPost, "/api/admin/users/"+actor+"/mfa/reset", bytes.NewReader(body))
			route := chi.NewRouteContext()
			route.URLParams.Add("user_id", actor)
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, route)
			ctx = context.WithValue(ctx, auth.UserIDKey, actor)
			ctx = context.WithValue(ctx, auth.SessionIDKey, sessionID)
			req = req.WithContext(ctx)
			req.Header.Set(adminReauthenticationHeader, grant)
			rec := httptest.NewRecorder()
			h.app.requireSensitiveAction(actionAdminMFAReset, sec007UsersEditPermission, "user_id")(http.HandlerFunc(h.app.handleAdminMFAReset)).ServeHTTP(rec, req)
			return rec
		}
		if _, err := h.db.ExecContext(h.ctx, `CREATE OR REPLACE FUNCTION sec007_reject_reset_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='admin.mfa.reset' THEN RAISE EXCEPTION 'controlled'; END IF; RETURN NEW; END $$; CREATE TRIGGER sec007_reject_reset_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION sec007_reject_reset_audit()`); err != nil {
			t.Fatal(err)
		}
		failed := invoke(issue())
		if failed.Code != http.StatusServiceUnavailable {
			t.Fatalf("failed reset=%d %s", failed.Code, failed.Body.String())
		}
		var count int
		if err := h.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM admin_mfa_credentials WHERE user_id=$1`, actor).Scan(&count); err != nil || count != 1 {
			t.Fatalf("rollback count=%d err=%v", count, err)
		}
		if _, err := h.db.ExecContext(h.ctx, `DROP TRIGGER sec007_reject_reset_audit ON audit_logs; DROP FUNCTION sec007_reject_reset_audit()`); err != nil {
			t.Fatal(err)
		}
		succeeded := invoke(issue())
		if succeeded.Code != http.StatusOK {
			t.Fatalf("reset=%d %s", succeeded.Code, succeeded.Body.String())
		}
		if err := h.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM admin_mfa_credentials WHERE user_id=$1`, actor).Scan(&count); err != nil || count != 0 {
			t.Fatalf("final count=%d err=%v", count, err)
		}
		if _, err := h.app.auth.Session.Get(h.ctx, sessionID); err != auth.ErrSessionNotFound {
			t.Fatalf("session not revoked: %v", err)
		}
	})
}
