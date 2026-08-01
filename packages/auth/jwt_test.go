package auth

import (
	"testing"
	"time"
)

func TestGenerateTokenPair(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	userID := "user-123"
	roles := []string{"user", "admin"}

	pair, err := service.GenerateTokenPair(userID, roles)
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Error("AccessToken and RefreshToken should be different")
	}
	if pair.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}
}

func TestValidateAccessToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	userID := "user-456"
	roles := []string{"user", "moderator"}

	pair, _ := service.GenerateTokenPair(userID, roles)

	claims, err := service.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("UserID mismatch: got %s, want %s", claims.UserID, userID)
	}
	if len(claims.Roles) != len(roles) {
		t.Errorf("Roles length mismatch: got %d, want %d", len(claims.Roles), len(roles))
	}
	if claims.TokenType != AccessToken {
		t.Errorf("TokenType should be AccessToken, got %s", claims.TokenType)
	}
}

func TestValidateRefreshToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	userID := "user-789"
	roles := []string{"user"}

	pair, _ := service.GenerateTokenPair(userID, roles)

	claims, err := service.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}

	if claims.TokenType != RefreshToken {
		t.Errorf("TokenType should be RefreshToken, got %s", claims.TokenType)
	}
}

func TestValidateAccessTokenWithRefreshToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	pair, _ := service.GenerateTokenPair("user-123", []string{"user"})

	// Trying to validate refresh token as access token should fail
	_, err := service.ValidateAccessToken(pair.RefreshToken)
	if err == nil {
		t.Error("ValidateAccessToken should fail for refresh token")
	}
	if err != ErrInvalidTokenType {
		t.Errorf("Expected ErrInvalidTokenType, got: %v", err)
	}
}

func TestValidateRefreshTokenWithAccessToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	pair, _ := service.GenerateTokenPair("user-123", []string{"user"})

	// Trying to validate access token as refresh token should fail
	_, err := service.ValidateRefreshToken(pair.AccessToken)
	if err == nil {
		t.Error("ValidateRefreshToken should fail for access token")
	}
	if err != ErrInvalidTokenType {
		t.Errorf("Expected ErrInvalidTokenType, got: %v", err)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	testCases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-valid-jwt"},
		{"wrong-parts", "header.payload"},
		{"malformed", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.ValidateToken(tc.token)
			if err == nil {
				t.Error("ValidateToken should fail for invalid token")
			}
		})
	}
}

func TestValidateWrongSecret(t *testing.T) {
	config1 := DefaultJWTConfig("secret-one")
	config2 := DefaultJWTConfig("secret-two")

	service1 := NewTokenService(config1)
	service2 := NewTokenService(config2)

	pair, _ := service1.GenerateTokenPair("user-123", []string{"user"})

	// Token signed with different secret should fail
	_, err := service2.ValidateToken(pair.AccessToken)
	if err == nil {
		t.Error("ValidateToken should fail for token signed with different secret")
	}
}

func TestExpiredToken(t *testing.T) {
	config := &JWTConfig{
		Secret:          []byte("test-secret"),
		Issuer:          "test",
		AccessTokenTTL:  -1 * time.Hour, // Already expired
		RefreshTokenTTL: -1 * time.Hour,
	}
	service := NewTokenService(config)

	token, _ := service.GenerateAccessToken("user-123", []string{"user"})

	_, err := service.ValidateToken(token)
	if err == nil {
		t.Error("ValidateToken should fail for expired token")
	}
	if err != ErrExpiredToken {
		t.Errorf("Expected ErrExpiredToken, got: %v", err)
	}
}

func TestClaimsHasRole(t *testing.T) {
	claims := &Claims{
		Roles: []string{"user", "admin"},
	}

	if !claims.HasRole("user") {
		t.Error("HasRole should return true for 'user'")
	}
	if !claims.HasRole("admin") {
		t.Error("HasRole should return true for 'admin'")
	}
	if claims.HasRole("moderator") {
		t.Error("HasRole should return false for 'moderator'")
	}
}

func TestClaimsHasAnyRole(t *testing.T) {
	claims := &Claims{
		Roles: []string{"user"},
	}

	if !claims.HasAnyRole("admin", "user") {
		t.Error("HasAnyRole should return true when user has one of the roles")
	}
	if claims.HasAnyRole("admin", "moderator") {
		t.Error("HasAnyRole should return false when user has none of the roles")
	}
}

func TestClaimsIsAdmin(t *testing.T) {
	adminClaims := &Claims{Roles: []string{"admin"}}
	userClaims := &Claims{Roles: []string{"user"}}

	if !adminClaims.IsAdmin() {
		t.Error("IsAdmin should return true for admin role")
	}
	if userClaims.IsAdmin() {
		t.Error("IsAdmin should return false for user role")
	}
}

func TestClaimsIsModerator(t *testing.T) {
	modClaims := &Claims{Roles: []string{"moderator"}}
	userClaims := &Claims{Roles: []string{"user"}}

	if !modClaims.IsModerator() {
		t.Error("IsModerator should return true for moderator role")
	}
	if userClaims.IsModerator() {
		t.Error("IsModerator should return false for user role")
	}
}

func TestTokenIssuer(t *testing.T) {
	config := &JWTConfig{
		Secret:          []byte("test-secret"),
		Issuer:          "custom-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	service := NewTokenService(config)

	token, _ := service.GenerateAccessToken("user-123", []string{"user"})
	claims, _ := service.ValidateToken(token)

	if claims.Issuer != "custom-issuer" {
		t.Errorf("Issuer mismatch: got %s, want custom-issuer", claims.Issuer)
	}
}

func TestTokenTimeClaims(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	config.AccessTokenTTL = 1 * time.Hour
	service := NewTokenService(config)

	// JWT uses second precision, so truncate for comparison
	before := time.Now().Truncate(time.Second)
	token, _ := service.GenerateAccessToken("user-123", []string{"user"})
	after := time.Now().Add(time.Second) // Add buffer for timing

	claims, _ := service.ValidateToken(token)

	// IssuedAt should be between before and after (with second precision)
	iat := claims.IssuedAt.Time
	if iat.Before(before) || iat.After(after) {
		t.Errorf("IssuedAt should be approximately now: got %v, expected between %v and %v", iat, before, after)
	}

	// ExpiresAt should be approximately 1 hour from now
	exp := claims.ExpiresAt.Time
	expectedExp := iat.Add(1 * time.Hour)
	diff := exp.Sub(expectedExp)
	if diff < -time.Second || diff > time.Second {
		t.Error("ExpiresAt should be 1 hour after IssuedAt")
	}
}

func BenchmarkGenerateTokenPair(b *testing.B) {
	config := DefaultJWTConfig("benchmark-secret-key")
	service := NewTokenService(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GenerateTokenPair("user-123", []string{"user", "admin"})
	}
}

func BenchmarkValidateToken(b *testing.B) {
	config := DefaultJWTConfig("benchmark-secret-key")
	service := NewTokenService(config)
	pair, _ := service.GenerateTokenPair("user-123", []string{"user"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.ValidateToken(pair.AccessToken)
	}
}

func TestSeparateRefreshSecret(t *testing.T) {
	config := &JWTConfig{
		Secret:          []byte("access-secret-key"),
		RefreshSecret:   []byte("refresh-secret-key"),
		Issuer:          "tragge",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	service := NewTokenService(config)

	pair, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Access token should validate successfully
	claims, err := service.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %s, want user-123", claims.UserID)
	}

	// Refresh token should validate successfully
	claims, err = service.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %s, want user-123", claims.UserID)
	}
}

func TestSeparateRefreshSecretCrossValidationFails(t *testing.T) {
	config := &JWTConfig{
		Secret:          []byte("access-secret-key"),
		RefreshSecret:   []byte("different-refresh-secret"),
		Issuer:          "tragge",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	service := NewTokenService(config)

	pair, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Validating refresh token with access secret (via ValidateToken) should fail
	_, err = service.ValidateToken(pair.RefreshToken)
	if err == nil {
		t.Error("ValidateToken should fail for refresh token when secrets differ")
	}

	// Validating access token with refresh secret (via ValidateRefreshToken) should fail
	_, err = service.ValidateRefreshToken(pair.AccessToken)
	if err == nil {
		t.Error("ValidateRefreshToken should fail for access token when secrets differ")
	}
}

func TestRefreshSecretFallback(t *testing.T) {
	// When RefreshSecret is not set, both token types should use the same Secret
	config := DefaultJWTConfig("shared-secret-key")
	service := NewTokenService(config)

	pair, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Both should validate (backward compatibility)
	if _, err := service.ValidateAccessToken(pair.AccessToken); err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if _, err := service.ValidateRefreshToken(pair.RefreshToken); err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}
}

func TestAudienceClaimPresent(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	pair, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	claims, err := service.ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if len(claims.Audience) == 0 {
		t.Fatal("Token should have an audience claim")
	}
	if claims.Audience[0] != "tragge-api" {
		t.Errorf("Audience = %v, want [tragge-api]", claims.Audience)
	}
}

func TestAudienceMismatchRejected(t *testing.T) {
	// Generate token with audience "tragge-api"
	genConfig := DefaultJWTConfig("test-secret-key-12345")
	genService := NewTokenService(genConfig)

	pair, err := genService.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Validate with a service expecting a different audience
	valConfig := &JWTConfig{
		Secret:          []byte("test-secret-key-12345"),
		Issuer:          "tragge",
		Audience:        []string{"other-api"},
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	valService := NewTokenService(valConfig)

	_, err = valService.ValidateToken(pair.AccessToken)
	if err == nil {
		t.Error("ValidateToken should fail when audience does not match")
	}
}

func TestTokenHasJTI(t *testing.T) {
	config := DefaultJWTConfig("test-secret-key-12345")
	service := NewTokenService(config)

	pair1, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate first token pair: %v", err)
	}
	pair2, err := service.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate second token pair: %v", err)
	}

	claims1, err := service.ValidateToken(pair1.AccessToken)
	if err != nil {
		t.Fatalf("Failed to validate first token: %v", err)
	}
	claims2, err := service.ValidateToken(pair2.AccessToken)
	if err != nil {
		t.Fatalf("Failed to validate second token: %v", err)
	}

	if claims1.ID == "" {
		t.Error("Token should have a JTI (JWT ID)")
	}
	if claims1.ID == claims2.ID {
		t.Error("Two tokens should have different JTIs")
	}
}
