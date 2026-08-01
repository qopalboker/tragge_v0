package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/sms"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var renderedSecurityCodePattern = regexp.MustCompile(
	`(?s)letter-spacing:[^>]*>\s*([0-9]{6})\s*</span>`,
)

func TestSEC003IntegrationPostgresRedisLifecycle(t *testing.T) {
	postgresDSN := os.Getenv("SEC003_POSTGRES_DSN")
	redisAddr := os.Getenv("SEC003_REDIS_ADDR")
	if postgresDSN == "" || redisAddr == "" {
		t.Skip("SEC003_POSTGRES_DSN and SEC003_REDIS_ADDR are required for the isolated runtime test")
	}
	requireIsolatedSEC003Targets(t, postgresDSN, redisAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	sqlDB, err := sql.Open("pgx", postgresDSN)
	if err != nil {
		t.Fatal("open isolated PostgreSQL test database:", err)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal("ping isolated PostgreSQL test database:", err)
	}
	createSEC003IntegrationSchema(t, ctx, sqlDB)

	redisConfig := pkgredis.DefaultConfig()
	redisConfig.Addr = redisAddr
	redisClient, err := pkgredis.NewClient(redisConfig)
	if err != nil {
		t.Fatal("construct isolated Redis client:", err)
	}
	defer redisClient.Close()
	if err := redisClient.PingCheck(ctx); err != nil {
		t.Fatal("ping isolated Redis test instance:", err)
	}
	if err := redisClient.Client().FlushDB(ctx).Err(); err != nil {
		t.Fatal("clear isolated Redis test database:", err)
	}

	logger, err := observability.NewLogger(observability.LogConfig{
		Service: "sec003-integration", Env: "test", Level: "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	pool := db.NewPoolFromDB(sqlDB)
	codeHasher, err := newSecurityCodeHasher("test-only-integration-HMAC-key-0123456789-ABCDEFG")
	if err != nil {
		t.Fatal(err)
	}
	userAuth := auth.New(&auth.Config{
		Context:          auth.ContextUser,
		JWTSecret:        "test-only-user-access-key-0123456789-ABCDEFG",
		JWTRefreshSecret: "test-only-user-refresh-key-0123456789-ABCDEFG",
		JWTIssuer:        "sec003-user-test",
		JWTAudience:      auth.AudienceUser,
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  time.Hour,
		Redis:            redisClient.Client(),
		SessionPrefix:    auth.UserSessionPrefix,
		SessionTTL:       time.Hour,
	})
	adminAuth := auth.New(&auth.Config{
		Context:          auth.ContextAdmin,
		JWTSecret:        "test-only-admin-access-key-0123456789-ABCDEFG",
		JWTRefreshSecret: "test-only-admin-refresh-key-0123456789-ABCDEFG",
		JWTIssuer:        "sec003-admin-test",
		JWTAudience:      auth.AudienceAdmin,
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  time.Hour,
		Redis:            redisClient.Client(),
		SessionPrefix:    auth.AdminSessionPrefix,
		SessionTTL:       time.Hour,
	})
	mailerino := &recordingSecurityEmailProvider{name: "mailerino"}
	resend := &recordingSecurityEmailProvider{name: "resend"}
	app := &App{
		pool: pool, redis: redisClient, auth: userAuth,
		securityEmail: &countrySecurityEmailRouter{mailerino: mailerino, resend: resend},
		codeHasher:    codeHasher, codeClock: systemSecurityCodeClock{},
		config: &Config{}, obs: &observability.Observability{Logger: logger},
	}

	initialHash, err := userAuth.HashPassword("Initial-test-password!345")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO users
		    (id, email, country, email_verified, phone_verified, preferred_lang, password_hash)
		 VALUES ('user-ir', 'ir-recipient@example.test', 'IR', false, false, 'en', $1),
		        ('user-ca', 'ca-recipient@example.test', 'CA', true, false, 'en', $1),
		        ('user-fail', 'fail-recipient@example.test', 'IR', false, false, 'en', $1)`,
		initialHash,
	); err != nil {
		t.Fatal("seed isolated users:", err)
	}

	t.Run("registration country is committed before provider selection", func(t *testing.T) {
		irProvider := &registrationCheckingSecurityEmailProvider{name: "mailerino", db: sqlDB, expectedCountry: "IR"}
		foreignProvider := &registrationCheckingSecurityEmailProvider{name: "resend", db: sqlDB, expectedCountry: "CA"}
		registrationApp := *app
		registrationApp.securityEmail = &countrySecurityEmailRouter{mailerino: irProvider, resend: foreignProvider}

		for _, invalid := range []RegisterRequest{
			{Email: "missing-country@example.test", Password: "Valid-test-password!345", AgreeTerms: true, AgeConfirm: true},
			{Email: "invalid-country@example.test", Password: "Valid-test-password!345", Country: "ZZ", AgreeTerms: true, AgeConfirm: true},
		} {
			response := registerUser(t, &registrationApp, invalid)
			if response.Code == http.StatusCreated {
				t.Fatal("registration accepted a missing or unsupported country")
			}
		}
		if irProvider.calls != 0 || foreignProvider.calls != 0 {
			t.Fatal("invalid country reached a delivery provider")
		}

		response := registerUser(t, &registrationApp, RegisterRequest{
			Email: "registration-ir@example.test", Password: "Valid-test-password!345",
			Country: " ir ", AgreeTerms: true, AgeConfirm: true,
		})
		if response.Code != http.StatusCreated || irProvider.calls != 1 || foreignProvider.calls != 0 {
			t.Fatalf("Iranian registration response=%d mailerino=%d resend=%d", response.Code, irProvider.calls, foreignProvider.calls)
		}
		response = registerUser(t, &registrationApp, RegisterRequest{
			Email: "registration-ca@example.test", Password: "Valid-test-password!345",
			Country: "ca", AgreeTerms: true, AgeConfirm: true,
		})
		if response.Code != http.StatusCreated || irProvider.calls != 1 || foreignProvider.calls != 1 {
			t.Fatalf("foreign registration response=%d mailerino=%d resend=%d", response.Code, irProvider.calls, foreignProvider.calls)
		}

		irProvider.err = errors.New("controlled provider rejection")
		response = registerUser(t, &registrationApp, RegisterRequest{
			Email: "registration-provider-fail@example.test", Password: "Valid-test-password!345",
			Country: "IR", AgreeTerms: true, AgeConfirm: true,
		})
		irProvider.err = nil
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("provider rejection response=%d", response.Code)
		}
		var active int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM verification_codes
			  WHERE destination = 'registration-provider-fail@example.test'
			    AND verified_at IS NULL AND expires_at > NOW()`,
		).Scan(&active); err != nil || active != 0 {
			t.Fatalf("provider rejection active codes=%d err=%v", active, err)
		}

		irProvider.deleteReservation = true
		response = registerUser(t, &registrationApp, RegisterRequest{
			Email: "registration-activation-fail@example.test", Password: "Valid-test-password!345",
			Country: "IR", AgreeTerms: true, AgeConfirm: true,
		})
		irProvider.deleteReservation = false
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("activation failure was reported as success: %d", response.Code)
		}

		if _, err := sqlDB.ExecContext(ctx, `UPDATE roles SET name = 'disabled-user-role' WHERE name = 'user'`); err != nil {
			t.Fatal(err)
		}
		response = registerUser(t, &registrationApp, RegisterRequest{
			Email: "registration-transaction-fail@example.test", Password: "Valid-test-password!345",
			Country: "IR", AgreeTerms: true, AgeConfirm: true,
		})
		if _, err := sqlDB.ExecContext(ctx, `UPDATE roles SET name = 'user' WHERE name = 'disabled-user-role'`); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("transaction failure response=%d", response.Code)
		}
		var rolledBack int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE email = 'registration-transaction-fail@example.test'`,
		).Scan(&rolledBack); err != nil || rolledBack != 0 {
			t.Fatalf("registration transaction was not rolled back: rows=%d err=%v", rolledBack, err)
		}
	})
	t.Run("country-routed verification and one-time consume", func(t *testing.T) {
		if _, err := app.issueVerificationCode(ctx, "user-ir", "email"); err != nil {
			t.Fatal("issue Iranian verification:", err)
		}
		if mailerino.calls != 1 || resend.calls != 0 {
			t.Fatal("Iranian verification did not route exclusively to Mailerino")
		}
		code := extractRenderedSecurityCode(t, mailerino.message)
		var storedHash string
		var expiresAt time.Time
		var maxAttempts int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT code_hash, expires_at, max_attempts FROM verification_codes
			  WHERE user_id = 'user-ir' AND verified_at IS NULL`).Scan(&storedHash, &expiresAt, &maxAttempts); err != nil {
			t.Fatal("inspect activated verification code:", err)
		}
		remaining := time.Until(expiresAt)
		if storedHash == code || len(storedHash) != 64 || maxAttempts != securityCodeMaxAttempts ||
			remaining < 9*time.Minute || remaining > securityCodeTTL {
			t.Fatal("activated verification state did not match canonical HMAC/TTL/attempt policy")
		}
		if err := app.verifyVerificationCode(ctx, "user-ir", "email", code); err != nil {
			t.Fatal("consume verification code:", err)
		}
		if err := app.verifyVerificationCode(ctx, "user-ir", "email", code); err == nil {
			t.Fatal("verification-code replay succeeded")
		}
		var emailOwned, phoneOwned bool
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT email_verified, phone_verified FROM users WHERE id = 'user-ir'`,
		).Scan(&emailOwned, &phoneOwned); err != nil || !emailOwned || phoneOwned {
			t.Fatalf("email ownership isolation: email=%v phone=%v err=%v", emailOwned, phoneOwned, err)
		}
	})

	t.Run("phone ownership uses real Redis and cannot mark email ownership", func(t *testing.T) {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO users (id, phone, country, email_verified, phone_verified, preferred_lang, password_hash)
			 VALUES ('user-phone', '+989120000200', 'IR', false, false, 'en', $1)`, initialHash,
		); err != nil {
			t.Fatal(err)
		}
		otpProvider := sms.NewFake()
		otpService, err := sms.NewOTPService(otpProvider, redisClient.Client(), sms.DefaultOTPConfig(
			"test-only-integration-HMAC-key-0123456789-ABCDEFG",
		))
		if err != nil {
			t.Fatal(err)
		}
		app.otpService = otpService
		if err := otpService.SendOTP(ctx, "+989120000200"); err != nil {
			t.Fatal("real Redis phone issue:", err)
		}
		redisCode := otpProvider.LastCode()
		if ok, err := otpService.VerifyOTP(ctx, "+989120000200", redisCode); !ok || err != nil {
			t.Fatalf("real Redis phone consume: ok=%v err=%v", ok, err)
		}
		if _, err := app.issueVerificationCode(ctx, "user-phone", "sms"); err != nil {
			t.Fatal("issue database-bound phone verification:", err)
		}
		if err := app.verifyVerificationCode(ctx, "user-phone", "sms", otpProvider.LastCode()); err != nil {
			t.Fatal("consume database-bound phone verification:", err)
		}
		var emailOwned, phoneOwned bool
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT email_verified, phone_verified FROM users WHERE id = 'user-phone'`,
		).Scan(&emailOwned, &phoneOwned); err != nil || emailOwned || !phoneOwned {
			t.Fatalf("phone ownership isolation: email=%v phone=%v err=%v", emailOwned, phoneOwned, err)
		}
	})

	t.Run("PostgreSQL attempts resend and concurrent consume", func(t *testing.T) {
		for _, user := range []struct{ id, email string }{
			{id: "user-attempts", email: "attempts@example.test"},
			{id: "user-resend", email: "resend@example.test"},
			{id: "user-concurrent", email: "concurrent@example.test"},
			{id: "user-wrong-context", email: "wrong-context@example.test"},
		} {
			if _, err := sqlDB.ExecContext(ctx,
				`INSERT INTO users (id, email, country, email_verified, phone_verified, preferred_lang, password_hash)
				 VALUES ($1, $2, 'CA', false, false, 'en', $3)`, user.id, user.email, initialHash,
			); err != nil {
				t.Fatal(err)
			}
		}

		if _, err := app.issueVerificationCode(ctx, "user-attempts", "email"); err != nil {
			t.Fatal(err)
		}
		attemptCode := extractRenderedSecurityCode(t, resend.message)
		wrongCode := "000000"
		if attemptCode == wrongCode {
			wrongCode = "111111"
		}
		for attempt := 1; attempt < securityCodeMaxAttempts; attempt++ {
			if err := app.verifyVerificationCode(ctx, "user-attempts", "email", wrongCode); !errors.Is(err, errWrongCode) {
				t.Fatalf("wrong attempt %d: %v", attempt, err)
			}
		}
		if err := app.verifyVerificationCode(ctx, "user-attempts", "email", wrongCode); !errors.Is(err, errCodeExhausted) {
			t.Fatalf("fifth attempt did not exhaust: %v", err)
		}
		if err := app.verifyVerificationCode(ctx, "user-attempts", "email", attemptCode); !errors.Is(err, errNoActiveCode) {
			t.Fatalf("exhausted code remained active: %v", err)
		}

		if _, err := app.issueVerificationCode(ctx, "user-resend", "email"); err != nil {
			t.Fatal(err)
		}
		first := extractRenderedSecurityCode(t, resend.message)
		if _, err := sqlDB.ExecContext(ctx,
			`UPDATE verification_codes SET created_at = NOW() - INTERVAL '61 seconds'
			  WHERE user_id = 'user-resend'`,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := app.issueVerificationCode(ctx, "user-resend", "email"); err != nil {
			t.Fatal(err)
		}
		second := extractRenderedSecurityCode(t, resend.message)
		if first == second {
			t.Fatal("CSPRNG collision made resend test indeterminate")
		}
		if err := app.verifyVerificationCode(ctx, "user-resend", "email", first); !errors.Is(err, errWrongCode) {
			t.Fatalf("previous code remained usable: %v", err)
		}
		if err := app.verifyVerificationCode(ctx, "user-resend", "email", second); err != nil {
			t.Fatalf("replacement code failed: %v", err)
		}

		if _, err := app.issueVerificationCode(ctx, "user-concurrent", "email"); err != nil {
			t.Fatal(err)
		}
		concurrentCode := extractRenderedSecurityCode(t, resend.message)
		if err := app.verifyVerificationCode(ctx, "user-wrong-context", "email", concurrentCode); !errors.Is(err, errNoActiveCode) {
			t.Fatalf("wrong user context result: %v", err)
		}
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- app.verifyVerificationCode(ctx, "user-concurrent", "email", concurrentCode)
			}()
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else if !errors.Is(err, errNoActiveCode) {
				t.Fatalf("unexpected concurrent consume result: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("concurrent PostgreSQL consume successes=%d", successes)
		}
	})

	t.Run("Iranian reset routes only to Mailerino", func(t *testing.T) {
		beforeMailerino, beforeResend := mailerino.calls, resend.calls
		_ = requestPasswordReset(t, app, "ir-recipient@example.test")
		if mailerino.calls != beforeMailerino+1 || resend.calls != beforeResend {
			t.Fatal("Iranian password reset did not route exclusively to Mailerino")
		}
	})

	t.Run("provider rejection leaves no active verification code", func(t *testing.T) {
		mailerino.err = errors.New("fixture provider rejection")
		_, issueErr := app.issueVerificationCode(ctx, "user-fail", "email")
		mailerino.err = nil
		if !errors.Is(issueErr, errSecurityCodeUnavailable) {
			t.Fatal("provider rejection did not fail closed")
		}
		var active int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM verification_codes
			  WHERE user_id = 'user-fail' AND verified_at IS NULL AND expires_at > NOW()`).Scan(&active); err != nil {
			t.Fatal("inspect failed-delivery compensation:", err)
		}
		if active != 0 {
			t.Fatal("provider rejection left a usable verification code")
		}
	})

	t.Run("password-reset provider failure compensates real state", func(t *testing.T) {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO users (id, email, country, email_verified, phone_verified, preferred_lang, password_hash)
			 VALUES ('user-reset-fail', 'reset-fail@example.test', 'IR', true, false, 'en', $1)`, initialHash,
		); err != nil {
			t.Fatal(err)
		}
		mailerino.err = errors.New("controlled reset-provider rejection")
		response := requestPasswordResetResponse(t, app, "reset-fail@example.test")
		mailerino.err = nil
		if response.Code != http.StatusOK {
			t.Fatalf("anti-enumeration response=%d", response.Code)
		}
		var active int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM password_reset_codes
			  WHERE user_id = 'user-reset-fail' AND used_at IS NULL AND expires_at > NOW()`,
		).Scan(&active); err != nil || active != 0 {
			t.Fatalf("failed reset delivery active codes=%d err=%v", active, err)
		}
	})
	t.Run("foreign reset anti-enumeration and session invalidation", func(t *testing.T) {
		beforeMailerino, beforeResend := mailerino.calls, resend.calls
		userSession, err := userAuth.Session.Create(ctx, &auth.Session{
			UserID: "user-ca", RefreshToken: "test-only-user-refresh-fixture",
		})
		if err != nil {
			t.Fatal("create User session:", err)
		}
		adminSession, err := adminAuth.Session.Create(ctx, &auth.Session{
			UserID: "user-ca", RefreshToken: "test-only-admin-refresh-fixture",
		})
		if err != nil {
			t.Fatal("create Admin session:", err)
		}

		resetToken := requestPasswordReset(t, app, "ca-recipient@example.test")
		if resend.calls != beforeResend+1 || mailerino.calls != beforeMailerino {
			t.Fatal("foreign reset did not route exclusively to Resend")
		}
		code := extractRenderedSecurityCode(t, resend.message)
		var resetDigest string
		var resetExpiry time.Time
		var resetAttempts int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT code_hash, expires_at, attempts FROM password_reset_codes
			  WHERE user_id = 'user-ca' AND used_at IS NULL`,
		).Scan(&resetDigest, &resetExpiry, &resetAttempts); err != nil || resetDigest == code || len(resetDigest) != 64 ||
			resetAttempts != 0 || time.Until(resetExpiry) < 9*time.Minute || time.Until(resetExpiry) > securityCodeTTL {
			t.Fatalf("password-reset storage policy mismatch: digest_length=%d attempts=%d err=%v", len(resetDigest), resetAttempts, err)
		}
		passwordSetToken := verifyPasswordResetCode(t, app, resetToken, code)
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO password_reset_codes
			    (user_id, code_hash, channel, destination, expires_at, attempts)
			 VALUES ('user-ca', repeat('a', 64), 'email', 'ca-recipient@example.test', NOW() + INTERVAL '10 minutes', 0)`,
		); err != nil {
			t.Fatal(err)
		}
		resetPassword(t, app, passwordSetToken, "Replacement-test-password!678")
		var activeResetCodes int
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM password_reset_codes
			  WHERE user_id = 'user-ca' AND used_at IS NULL AND expires_at > NOW()`,
		).Scan(&activeResetCodes); err != nil || activeResetCodes != 0 {
			t.Fatalf("successful reset left active reset codes=%d err=%v", activeResetCodes, err)
		}

		if _, err := userAuth.Session.Get(ctx, userSession); err == nil {
			t.Fatal("successful reset left a User session active")
		}
		if _, err := adminAuth.Session.Get(ctx, adminSession); err != nil {
			t.Fatal("User password reset invalidated an isolated Admin session")
		}
		var newHash string
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT password_hash FROM users WHERE id = 'user-ca'`,
		).Scan(&newHash); err != nil {
			t.Fatal(err)
		}
		if err := userAuth.VerifyPassword("Replacement-test-password!678", newHash); err != nil {
			t.Fatal("password was not changed")
		}

		replay := httptest.NewRecorder()
		body, _ := json.Marshal(ForgotPasswordResetRequest{
			PasswordSetToken: passwordSetToken,
			NewPassword:      "Another-test-password!901",
			ConfirmPassword:  "Another-test-password!901",
		})
		app.handleForgotPasswordReset(replay, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
		if replay.Code != http.StatusBadRequest {
			t.Fatal("password-set token replay did not fail closed")
		}

		before := resend.calls
		unknown := requestPasswordResetResponse(t, app, "absent-recipient@example.test")
		if unknown.Code != http.StatusOK || resend.calls != before {
			t.Fatal("unknown-account reset did not preserve anti-enumeration behavior")
		}
	})
}

type registrationCheckingSecurityEmailProvider struct {
	name              string
	db                *sql.DB
	expectedCountry   string
	calls             int
	message           notification.SecurityEmailMessage
	err               error
	deleteReservation bool
}

func (p *registrationCheckingSecurityEmailProvider) ProviderName() string { return p.name }

func (p *registrationCheckingSecurityEmailProvider) SendSecurityEmail(
	ctx context.Context,
	message notification.SecurityEmailMessage,
) error {
	var country string
	if err := p.db.QueryRowContext(ctx, `SELECT country FROM users WHERE email = $1`, message.To).Scan(&country); err != nil {
		return fmt.Errorf("country was not persisted before provider call: %w", err)
	}
	if country != p.expectedCountry {
		return fmt.Errorf("provider observed country %q, want %q", country, p.expectedCountry)
	}
	p.calls++
	p.message = message
	if p.deleteReservation {
		if _, err := p.db.ExecContext(ctx, `DELETE FROM verification_codes WHERE destination = $1`, message.To); err != nil {
			return fmt.Errorf("controlled activation failure: %w", err)
		}
	}
	return p.err
}

func registerUser(t *testing.T, app *App, request RegisterRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	app.handleRegister(recorder, httpRequest)
	return recorder
}
func requireIsolatedSEC003Targets(t *testing.T, postgresDSN, redisAddr string) {
	t.Helper()
	parsed, err := url.Parse(postgresDSN)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatal("SEC-003 PostgreSQL target is not an identifiable PostgreSQL URL")
	}
	databaseName := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if !strings.Contains(strings.ToLower(databaseName), "test") ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("SEC-003 PostgreSQL target must be a loopback test database")
	}
	redisHost, _, err := net.SplitHostPort(redisAddr)
	if err != nil || redisHost != "127.0.0.1" && redisHost != "localhost" {
		t.Fatal("SEC-003 Redis target must be an identifiable loopback endpoint")
	}
}

func createSEC003IntegrationSchema(t *testing.T, ctx context.Context, sqlDB *sql.DB) {
	t.Helper()
	statements := []string{
		`DROP TABLE IF EXISTS notifications, password_reset_codes, verification_codes, user_roles, roles, users CASCADE`,
		`CREATE TABLE users (
			id text PRIMARY KEY DEFAULT gen_random_uuid()::text,
			email text UNIQUE,
			phone text,
			country text,
			email_verified boolean NOT NULL DEFAULT false,
			email_verified_at timestamptz,
			phone_verified boolean NOT NULL DEFAULT false,
			preferred_lang text NOT NULL DEFAULT 'en',
			password_hash text NOT NULL DEFAULT '',
			password_changed_at timestamptz,
			terms_accepted_at timestamptz,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE roles (
			id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			name text NOT NULL UNIQUE
		)`,
		`CREATE TABLE user_roles (
			user_id text NOT NULL REFERENCES users(id),
			role_id integer NOT NULL REFERENCES roles(id),
			PRIMARY KEY (user_id, role_id)
		)`,
		`INSERT INTO roles (name) VALUES ('user')`,
		`CREATE TABLE verification_codes (
			id text PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id text NOT NULL REFERENCES users(id),
			code_hash varchar(64) NOT NULL,
			method text NOT NULL,
			destination text NOT NULL,
			expires_at timestamptz NOT NULL,
			attempts integer NOT NULL DEFAULT 0,
			max_attempts integer NOT NULL DEFAULT 5,
			verified_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE password_reset_codes (
			id text PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id text NOT NULL REFERENCES users(id),
			code_hash varchar(64) NOT NULL,
			channel text NOT NULL,
			destination text NOT NULL,
			expires_at timestamptz NOT NULL,
			attempts integer NOT NULL DEFAULT 0,
			used_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE notifications (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			type text NOT NULL,
			title text NOT NULL,
			message text,
			metadata jsonb,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
	}
	for _, statement := range statements {
		if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
			t.Fatal("prepare isolated SEC-003 schema:", err)
		}
	}
}

func extractRenderedSecurityCode(t *testing.T, message notification.SecurityEmailMessage) string {
	t.Helper()
	match := renderedSecurityCodePattern.FindStringSubmatch(message.HTML)
	if len(match) != 2 {
		match = renderedSecurityCodePattern.FindStringSubmatch(message.Text)
	}
	if len(match) != 2 {
		t.Fatal("security email did not contain a generated six-digit code")
	}
	return match[1]
}

func TestExtractRenderedSecurityCodeIgnoresHTMLNumericEntities(t *testing.T) {
	message := notification.SecurityEmailMessage{
		HTML: `&#128274;<span style="letter-spacing: 12px;">654321</span>`,
	}
	if got := extractRenderedSecurityCode(t, message); got != "654321" {
		t.Fatalf("extracted code = %q, want rendered code", got)
	}
}

func requestPasswordReset(t *testing.T, app *App, identifier string) string {
	t.Helper()
	recorder := requestPasswordResetResponse(t, app, identifier)
	if recorder.Code != http.StatusOK {
		t.Fatal("password-reset request did not return its generic response")
	}
	var response struct {
		ResetToken string `json:"reset_token"`
	}
	if json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.ResetToken == "" {
		t.Fatal("password-reset request did not return an opaque session handle")
	}
	return response.ResetToken
}

func requestPasswordResetResponse(t *testing.T, app *App, identifier string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(ForgotPasswordRequest{Identifier: identifier})
	recorder := httptest.NewRecorder()
	app.handleForgotPasswordRequest(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	return recorder
}

func verifyPasswordResetCode(t *testing.T, app *App, resetToken, code string) string {
	t.Helper()
	body, _ := json.Marshal(ForgotPasswordVerifyRequest{ResetToken: resetToken, Code: code})
	recorder := httptest.NewRecorder()
	app.handleForgotPasswordVerify(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatal("password-reset code verification failed")
	}
	var response struct {
		PasswordSetToken string `json:"password_set_token"`
	}
	if json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.PasswordSetToken == "" {
		t.Fatal("password-reset verification did not return a one-time password-set handle")
	}
	return response.PasswordSetToken
}

func resetPassword(t *testing.T, app *App, passwordSetToken, password string) {
	t.Helper()
	body, _ := json.Marshal(ForgotPasswordResetRequest{
		PasswordSetToken: passwordSetToken, NewPassword: password, ConfirmPassword: password,
	})
	recorder := httptest.NewRecorder()
	app.handleForgotPasswordReset(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatal("password reset did not complete")
	}
}
