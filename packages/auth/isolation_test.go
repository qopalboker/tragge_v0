package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	fixtureUserAccess   = "F1x!Ua#9Qv$2Lm^8Za&4Nc*7Rt@5Wp_K3dB6"
	fixtureUserRefresh  = "G2y@Ub$8Rw%3Mn&7Yb*5Od!6Su#4Xq_L9eC1"
	fixtureAdminAccess  = "H3z#Ac%7Sx^4No*6Xc!8Pe@5Tv$3Yr_M2fD9"
	fixtureAdminRefresh = "J4w$Ad^6Ty&5Op!9Wd@7Qf#4Uw%2Zs_N8gE3"
)

func validIsolationConfig(environment string) IsolationConfig {
	return IsolationConfig{
		Environment: environment,
		User: ContextConfig{
			Context: ContextUser, AccessSecret: fixtureUserAccess, RefreshSecret: fixtureUserRefresh,
			Issuer: IssuerUser, Audience: AudienceUser, SessionPrefix: UserSessionPrefix,
			RevocationPrefix: UserRevocationPrefix, RefreshCookieName: UserRefreshCookieName,
			SessionHintCookieName: UserSessionHintCookieName, RefreshCookiePath: UserRefreshCookiePath,
			CSRFContext: UserCSRFContext, CSRFOrigin: "https://app.example.invalid",
		},
		Admin: ContextConfig{
			Context: ContextAdmin, AccessSecret: fixtureAdminAccess, RefreshSecret: fixtureAdminRefresh,
			Issuer: IssuerAdmin, Audience: AudienceAdmin, SessionPrefix: AdminSessionPrefix,
			RevocationPrefix: AdminRevocationPrefix, RefreshCookieName: AdminRefreshCookieName,
			SessionHintCookieName: AdminSessionHintCookieName, RefreshCookiePath: AdminRefreshCookiePath,
			CSRFContext: AdminCSRFContext, CSRFOrigin: "https://admin.example.invalid",
		},
	}
}

func newIsolatedAuthPair(t *testing.T, client redis.UniversalClient) (*Auth, *Auth, IsolationConfig) {
	t.Helper()
	config := validIsolationConfig("test")
	if err := config.Validate(); err != nil {
		t.Fatalf("valid test isolation config rejected: %v", err)
	}
	userAuth, err := NewContext(config.User, client)
	if err != nil {
		t.Fatalf("construct User context: %v", err)
	}
	adminAuth, err := NewContext(config.Admin, client)
	if err != nil {
		t.Fatalf("construct Admin context: %v", err)
	}
	return userAuth, adminAuth, config
}

func TestIsolationConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*IsolationConfig)
	}{
		{"missing User secret", func(c *IsolationConfig) { c.User.AccessSecret = "" }},
		{"missing Admin secret", func(c *IsolationConfig) { c.Admin.AccessSecret = "" }},
		{"equal access secrets", func(c *IsolationConfig) { c.Admin.AccessSecret = c.User.AccessSecret }},
		{"equal refresh secrets", func(c *IsolationConfig) { c.Admin.RefreshSecret = c.User.RefreshSecret }},
		{"same-context access and refresh", func(c *IsolationConfig) { c.User.RefreshSecret = c.User.AccessSecret }},
		{"weak secret", func(c *IsolationConfig) { c.User.AccessSecret = "short" }},
		{"long repeated secret", func(c *IsolationConfig) { c.User.AccessSecret = strings.Repeat("A", 64) }},
		{"placeholder secret", func(c *IsolationConfig) { c.User.AccessSecret = "CHANGE-ME-DEFAULT-PLACEHOLDER-1234567890!Aa" }},
		{"missing audience", func(c *IsolationConfig) { c.User.Audience = "" }},
		{"equal audience", func(c *IsolationConfig) { c.Admin.Audience = c.User.Audience }},
		{"equal issuer", func(c *IsolationConfig) { c.Admin.Issuer = c.User.Issuer }},
		{"cookie collision", func(c *IsolationConfig) { c.Admin.RefreshCookieName = c.User.RefreshCookieName }},
		{"session namespace collision", func(c *IsolationConfig) { c.Admin.SessionPrefix = c.User.SessionPrefix }},
		{"revocation namespace collision", func(c *IsolationConfig) { c.Admin.RevocationPrefix = c.User.RevocationPrefix }},
		{"CSRF context collision", func(c *IsolationConfig) { c.Admin.CSRFContext = c.User.CSRFContext }},
		{"CSRF origin collision", func(c *IsolationConfig) { c.Admin.CSRFOrigin = c.User.CSRFOrigin }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validIsolationConfig("production")
			test.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("invalid isolation config was accepted")
			}
		})
	}

	if err := validIsolationConfig("production").Validate(); err != nil {
		t.Fatalf("valid isolated production config rejected: %v", err)
	}
}

func TestLoadIsolationConfigHasNoProductionDefaults(t *testing.T) {
	values := map[string]string{}
	get := func(name string) string { return values[name] }
	config := LoadIsolationConfig("production", get, get)
	if err := config.Validate(); err == nil {
		t.Fatal("empty production config was accepted")
	}
	if config.User.Issuer != "" || config.Admin.Audience != "" {
		t.Fatal("production issuer or audience silently defaulted")
	}
}

func TestUserAdminAccessAndRefreshIsolation(t *testing.T) {
	userAuth, adminAuth, _ := newIsolatedAuthPair(t, nil)
	userPair, err := userAuth.Token.GenerateTokenPair("subject-1", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}
	adminPair, err := adminAuth.Token.GenerateTokenPair("subject-2", []string{"support_admin"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		validator func(string) (*Claims, error)
		token     string
		wantErr   bool
	}{
		{"User access accepted by User", userAuth.Token.ValidateAccessToken, userPair.AccessToken, false},
		{"Admin access accepted by Admin", adminAuth.Token.ValidateAccessToken, adminPair.AccessToken, false},
		{"User access rejected by Admin", adminAuth.Token.ValidateAccessToken, userPair.AccessToken, true},
		{"Admin access rejected by User", userAuth.Token.ValidateAccessToken, adminPair.AccessToken, true},
		{"User refresh accepted by User", userAuth.Token.ValidateRefreshToken, userPair.RefreshToken, false},
		{"Admin refresh accepted by Admin", adminAuth.Token.ValidateRefreshToken, adminPair.RefreshToken, false},
		{"User refresh rejected by Admin", adminAuth.Token.ValidateRefreshToken, userPair.RefreshToken, true},
		{"Admin refresh rejected by User", userAuth.Token.ValidateRefreshToken, adminPair.RefreshToken, true},
		{"wrong token purpose", userAuth.Token.ValidateAccessToken, userPair.RefreshToken, true},
		{"malformed token", userAuth.Token.ValidateAccessToken, "not-a-token", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims, err := test.validator(test.token)
			if test.wantErr && err == nil {
				t.Fatal("cross-context or invalid token accepted")
			}
			if !test.wantErr {
				if err != nil {
					t.Fatalf("valid token rejected: %v", err)
				}
				if claims.AuthContext == "" {
					t.Fatal("auth_context claim missing")
				}
			}
		})
	}
}

func TestUserValidatorRejectsCrossIssuerAudienceAndContext(t *testing.T) {
	_, _, config := newIsolatedAuthPair(t, nil)
	userAuth, _ := NewContext(config.User, nil)
	tests := []struct {
		name   string
		mutate func(*ContextConfig)
	}{
		{"cross issuer", func(c *ContextConfig) { c.Issuer = IssuerAdmin }},
		{"cross audience", func(c *ContextConfig) { c.Audience = AudienceAdmin }},
		{"cross context", func(c *ContextConfig) { c.Context = ContextAdmin }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attackerConfig := config.User
			test.mutate(&attackerConfig)
			attacker, err := NewContext(attackerConfig, nil)
			if err != nil {
				t.Fatal(err)
			}
			token, err := attacker.Token.GenerateAccessToken("subject", []string{"user"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := userAuth.Token.ValidateAccessToken(token); err == nil {
				t.Fatal("token from wrong issuer, audience, or context accepted")
			}
		})
	}
}

func TestModifiedRoleClaimWithoutValidSignatureRejected(t *testing.T) {
	userAuth, _, _ := newIsolatedAuthPair(t, nil)
	token, err := userAuth.Token.GenerateAccessToken("subject", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["roles"] = []string{"super_admin"}
	payload, _ = json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	if _, err := userAuth.Token.ValidateAccessToken(strings.Join(parts, ".")); err == nil {
		t.Fatal("modified role claim accepted without a valid signature")
	}
}

func TestTimeAlgorithmAudienceAndPurposeValidation(t *testing.T) {
	config := validIsolationConfig("test").User
	service := NewTokenService(&JWTConfig{
		Secret: []byte(config.AccessSecret), RefreshSecret: []byte(config.RefreshSecret),
		Issuer: config.Issuer, Audience: []string{config.Audience}, Context: config.Context,
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour,
	})
	now := time.Now()
	base := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: config.Issuer, Subject: "subject", Audience: jwt.ClaimStrings{config.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), IssuedAt: jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		UserID: "subject", TokenType: AccessToken, AuthContext: ContextUser,
	}

	tests := []struct {
		name   string
		claims Claims
		method jwt.SigningMethod
	}{
		{"expired", func() Claims { c := base; c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute)); return c }(), jwt.SigningMethodHS256},
		{"not before", func() Claims { c := base; c.NotBefore = jwt.NewNumericDate(now.Add(time.Hour)); return c }(), jwt.SigningMethodHS256},
		{"unknown purpose", func() Claims { c := base; c.TokenType = TokenType("unknown"); return c }(), jwt.SigningMethodHS256},
		{"multiple audience", func() Claims { c := base; c.Audience = jwt.ClaimStrings{config.Audience, "other"}; return c }(), jwt.SigningMethodHS256},
		{"wrong algorithm", base, jwt.SigningMethodHS384},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(test.method, test.claims).SignedString([]byte(config.AccessSecret))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ValidateAccessToken(token); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}

func TestSessionRefreshAndRevocationNamespacesAreIndependent(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	userAuth, adminAuth, _ := newIsolatedAuthPair(t, client)
	ctx := context.Background()

	userPair, userSession, err := userAuth.Login(ctx, "same-subject", []string{"user"}, "device", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	adminPair, adminSession, err := adminAuth.Login(ctx, "same-subject", []string{"support_admin"}, "device", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !mini.Exists(UserSessionPrefix+userSession) || !mini.Exists(AdminSessionPrefix+adminSession) {
		t.Fatal("context-namespaced sessions were not created")
	}
	if _, err := adminAuth.Refresh(ctx, adminSession, userPair.RefreshToken); err == nil {
		t.Fatal("User refresh token accepted by Admin refresh flow")
	}
	if _, err := adminAuth.Refresh(ctx, adminSession, adminPair.RefreshToken); err != nil {
		t.Fatalf("cross-context attempt mutated valid Admin session: %v", err)
	}
	if err := userAuth.Session.Delete(ctx, userSession); err != nil {
		t.Fatal(err)
	}
	if !mini.Exists(AdminSessionPrefix + adminSession) {
		t.Fatal("User logout revoked Admin session")
	}

	userBlacklist := NewTokenBlacklistWithPrefix(client, UserRevocationPrefix)
	adminBlacklist := NewTokenBlacklistWithPrefix(client, AdminRevocationPrefix)
	if err := userBlacklist.Add(ctx, "same-jti", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !userBlacklist.IsBlacklisted(ctx, "same-jti") || adminBlacklist.IsBlacklisted(ctx, "same-jti") {
		t.Fatal("revocation keyspaces are not isolated")
	}
}

func TestLegacySharedTrustRegression(t *testing.T) {
	legacyUser := DefaultConfig()
	legacyUser.JWTSecret = "legacy-shared-fixture"
	legacyUser.JWTRefreshSecret = "legacy-shared-fixture"
	legacyAdmin := DefaultConfig()
	legacyAdmin.JWTSecret = legacyUser.JWTSecret
	legacyAdmin.JWTRefreshSecret = legacyUser.JWTRefreshSecret
	userLegacyAuth := New(legacyUser)
	adminLegacyAuth := New(legacyAdmin)
	legacyToken, err := userLegacyAuth.Token.GenerateAccessToken("subject", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminLegacyAuth.Token.ValidateAccessToken(legacyToken); err != nil {
		t.Fatalf("regression fixture did not reproduce shared cryptographic trust: %v", err)
	}

	userAuth, adminAuth, _ := newIsolatedAuthPair(t, nil)
	isolatedToken, _ := userAuth.Token.GenerateAccessToken("subject", []string{"user"})
	if _, err := adminAuth.Token.ValidateAccessToken(isolatedToken); err == nil {
		t.Fatal("corrected Admin validator accepted User token")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/representative-admin", nil)
	req.Header.Set("Authorization", "Bearer "+isolatedToken)
	rec := httptest.NewRecorder()
	adminAuth.Middleware.RequireAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("representative Admin endpoint returned %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "user") || strings.Contains(rec.Body.String(), "signature") {
		t.Fatal("cross-context response leaked validation detail")
	}
}

func TestIsolationErrorsDoNotLeakSecrets(t *testing.T) {
	config := validIsolationConfig("production")
	config.Admin.AccessSecret = config.User.AccessSecret
	err := config.Validate()
	if err == nil {
		t.Fatal("expected collision error")
	}
	for _, secret := range []string{fixtureUserAccess, fixtureUserRefresh, fixtureAdminAccess, fixtureAdminRefresh} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("secret value leaked in validation error")
		}
	}
	if !errors.Is(err, ErrInvalidAuthIsolation) {
		t.Fatalf("unexpected error type: %v", err)
	}
}
