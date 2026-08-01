package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// AuthTestServer wraps an httptest.Server with auth capabilities.
type AuthTestServer struct {
	Server      *httptest.Server
	Auth        *auth.Auth
	Env         *TestEnv
	BaseURL     string
	HTTPClient  *http.Client
}

// NewAuthTestServer creates a test server with auth endpoints.
func NewAuthTestServer(t *testing.T, env *TestEnv) *AuthTestServer {
	t.Helper()

	// Initialize auth service
	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authConfig.AccessTokenTTL = 15 * time.Minute
	authConfig.RefreshTokenTTL = 24 * time.Hour
	authService := auth.New(authConfig)

	ats := &AuthTestServer{
		Auth:       authService,
		Env:        env,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Create test server with auth routes
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/user/auth/register", ats.handleRegister)
	mux.HandleFunc("POST /api/user/auth/login", ats.handleLogin)
	mux.HandleFunc("POST /api/user/auth/refresh", ats.handleRefresh)
	mux.HandleFunc("GET /api/user/me", ats.withAuth(ats.handleMe))

	ats.Server = httptest.NewServer(mux)
	ats.BaseURL = ats.Server.URL

	return ats
}

// Close shuts down the test server.
func (ats *AuthTestServer) Close() {
	ats.Server.Close()
}

// RegisterRequest is the request body for registration.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the request body for login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest is the request body for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse is the response for successful authentication.
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// UserResponse is the response for /me endpoint.
type UserResponse struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
}

// ErrorResponse is the response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

func (ats *AuthTestServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "email and password are required"})
		return
	}

	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "password must be at least 8 characters"})
		return
	}

	// Hash password
	passwordHash, err := ats.Auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	ctx := r.Context()

	// Begin transaction
	tx, err := ats.Env.DB.BeginTx(ctx, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	defer tx.Rollback()

	// Insert user
	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		req.Email, passwordHash,
	).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "email already registered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Assign user role
	var roleID int
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = 'user'`).Scan(&roleID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
		userID, roleID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Generate tokens
	roles := []string{"user"}
	tokenPair, err := ats.Auth.Token.GenerateTokenPair(userID, roles)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
	})
}

func (ats *AuthTestServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "email and password are required"})
		return
	}

	ctx := r.Context()

	// Get user by email
	var userID, passwordHash string
	err := ats.Env.DB.QueryRowContext(ctx,
		`SELECT id, password_hash FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &passwordHash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
		return
	}

	// Verify password
	if err := ats.Auth.VerifyPassword(req.Password, passwordHash); err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
		return
	}

	// Get user roles
	roles, err := getUserRoles(ctx, ats.Env.DB, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Generate tokens
	tokenPair, err := ats.Auth.Token.GenerateTokenPair(userID, roles)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
	})
}

func (ats *AuthTestServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "refresh_token is required"})
		return
	}

	// Validate refresh token
	claims, err := ats.Auth.Token.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid or expired refresh token"})
		return
	}

	ctx := r.Context()

	// Get fresh roles from database
	roles, err := getUserRoles(ctx, ats.Env.DB, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Generate new token pair
	tokenPair, err := ats.Auth.Token.GenerateTokenPair(claims.UserID, roles)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
	})
}

func (ats *AuthTestServer) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	roles := r.Context().Value(contextKeyRoles).([]string)

	ctx := r.Context()

	// Get user email from database
	var email string
	err := ats.Env.DB.QueryRowContext(ctx,
		`SELECT email FROM users WHERE id = $1`,
		userID,
	).Scan(&email)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, UserResponse{
		UserID: userID,
		Email:  email,
		Roles:  roles,
	})
}

// Context keys for auth middleware
type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
	contextKeyRoles  contextKey = "roles"
)

// withAuth is middleware that validates JWT tokens.
func (ats *AuthTestServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "authorization header required"})
			return
		}

		const bearerPrefix = "Bearer "
		if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid authorization header format"})
			return
		}

		token := authHeader[len(bearerPrefix):]

		claims, err := ats.Auth.Token.ValidateAccessToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid or expired token"})
			return
		}

		ctx := context.WithValue(r.Context(), contextKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, contextKeyRoles, claims.Roles)
		next(w, r.WithContext(ctx))
	}
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "duplicate key") || contains(errStr, "unique constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func getUserRoles(ctx context.Context, db *sql.DB, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT r.name FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	if len(roles) == 0 {
		roles = []string{"user"}
	}

	return roles, rows.Err()
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestAuthFlow_RegisterLoginRefresh(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create test server
	server := NewAuthTestServer(t, env)
	defer server.Close()

	t.Run("Register_Success", func(t *testing.T) {
		reqBody := RegisterRequest{
			Email:    "test@example.com",
			Password: "securepassword123",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/register",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var authResp AuthResponse
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if authResp.AccessToken == "" {
			t.Error("Expected access token, got empty string")
		}
		if authResp.RefreshToken == "" {
			t.Error("Expected refresh token, got empty string")
		}
		if authResp.ExpiresAt.IsZero() {
			t.Error("Expected expires_at to be set")
		}
		if authResp.ExpiresAt.Before(time.Now()) {
			t.Error("Expected expires_at to be in the future")
		}
	})

	t.Run("Register_DuplicateEmail", func(t *testing.T) {
		reqBody := RegisterRequest{
			Email:    "test@example.com", // Same email as above
			Password: "anotherpassword123",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/register",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected status 409, got %d", resp.StatusCode)
		}
	})

	t.Run("Register_InvalidPassword", func(t *testing.T) {
		reqBody := RegisterRequest{
			Email:    "newuser@example.com",
			Password: "short", // Too short
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/register",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	var accessToken, refreshToken string

	t.Run("Login_Success", func(t *testing.T) {
		reqBody := LoginRequest{
			Email:    "test@example.com",
			Password: "securepassword123",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/login",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var authResp AuthResponse
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if authResp.AccessToken == "" {
			t.Error("Expected access token, got empty string")
		}
		if authResp.RefreshToken == "" {
			t.Error("Expected refresh token, got empty string")
		}

		accessToken = authResp.AccessToken
		refreshToken = authResp.RefreshToken
	})

	t.Run("Login_InvalidCredentials", func(t *testing.T) {
		reqBody := LoginRequest{
			Email:    "test@example.com",
			Password: "wrongpassword",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/login",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Login_NonexistentUser", func(t *testing.T) {
		reqBody := LoginRequest{
			Email:    "nonexistent@example.com",
			Password: "somepassword123",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/login",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Me_Success", func(t *testing.T) {
		if accessToken == "" {
			t.Skip("skipping: Login_Success did not populate token")
		}
		req, _ := http.NewRequest("GET", server.BaseURL+"/api/user/me", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := server.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var userResp UserResponse
		if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if userResp.UserID == "" {
			t.Error("Expected user_id, got empty string")
		}
		if userResp.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got '%s'", userResp.Email)
		}
		if len(userResp.Roles) == 0 {
			t.Error("Expected at least one role")
		}
	})

	t.Run("Me_Unauthorized", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.BaseURL+"/api/user/me", nil)
		// No Authorization header

		resp, err := server.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Me_InvalidToken", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.BaseURL+"/api/user/me", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")

		resp, err := server.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Refresh_Success", func(t *testing.T) {
		if refreshToken == "" {
			t.Skip("skipping: Login_Success did not populate token")
		}
		reqBody := RefreshRequest{
			RefreshToken: refreshToken,
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/refresh",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var authResp AuthResponse
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if authResp.AccessToken == "" {
			t.Error("Expected new access token, got empty string")
		}
		if authResp.RefreshToken == "" {
			t.Error("Expected new refresh token, got empty string")
		}

		// Verify the new access token works
		req, _ := http.NewRequest("GET", server.BaseURL+"/api/user/me", nil)
		req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)

		resp2, err := server.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			t.Errorf("New access token should be valid, got status %d", resp2.StatusCode)
		}
	})

	t.Run("Refresh_InvalidToken", func(t *testing.T) {
		reqBody := RefreshRequest{
			RefreshToken: "invalid-refresh-token",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/refresh",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Refresh_UsingAccessToken", func(t *testing.T) {
		if accessToken == "" {
			t.Skip("skipping: Login_Success did not populate token")
		}
		// Access tokens should not work as refresh tokens
		reqBody := RefreshRequest{
			RefreshToken: accessToken, // Using access token instead of refresh token
		}
		body, _ := json.Marshal(reqBody)

		resp, err := server.HTTPClient.Post(
			server.BaseURL+"/api/user/auth/refresh",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401 when using access token as refresh token, got %d", resp.StatusCode)
		}
	})
}

func TestAuthFlow_TokenValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create auth service with short token TTL for testing expiration
	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authConfig.AccessTokenTTL = 1 * time.Second  // Very short TTL
	authConfig.RefreshTokenTTL = 2 * time.Second // Very short TTL
	authService := auth.New(authConfig)

	// Create test user
	passwordHash, _ := authService.HashPassword("testpassword123")
	userID := env.CreateTestUser(ctx, t, "tokentest@example.com", passwordHash)

	t.Run("AccessToken_Expiration", func(t *testing.T) {
		// Generate token pair
		tokenPair, err := authService.Token.GenerateTokenPair(userID, []string{"user"})
		if err != nil {
			t.Fatalf("Failed to generate token pair: %v", err)
		}

		// Verify token works initially
		claims, err := authService.Token.ValidateAccessToken(tokenPair.AccessToken)
		if err != nil {
			t.Fatalf("Token should be valid initially: %v", err)
		}
		if claims.UserID != userID {
			t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
		}

		// Wait for token to expire
		time.Sleep(2 * time.Second)

		// Verify token is now expired
		_, err = authService.Token.ValidateAccessToken(tokenPair.AccessToken)
		if err == nil {
			t.Error("Token should be expired")
		}
	})

	t.Run("RefreshToken_Expiration", func(t *testing.T) {
		// Generate token pair
		tokenPair, err := authService.Token.GenerateTokenPair(userID, []string{"user"})
		if err != nil {
			t.Fatalf("Failed to generate token pair: %v", err)
		}

		// Verify refresh token works initially
		claims, err := authService.Token.ValidateRefreshToken(tokenPair.RefreshToken)
		if err != nil {
			t.Fatalf("Refresh token should be valid initially: %v", err)
		}
		if claims.UserID != userID {
			t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
		}

		// Wait for refresh token to expire
		time.Sleep(3 * time.Second)

		// Verify refresh token is now expired
		_, err = authService.Token.ValidateRefreshToken(tokenPair.RefreshToken)
		if err == nil {
			t.Error("Refresh token should be expired")
		}
	})
}

func TestAuthFlow_RoleBasedAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authService := auth.New(authConfig)

	t.Run("Claims_ContainRoles", func(t *testing.T) {
		roles := []string{"user", "admin"}
		tokenPair, err := authService.Token.GenerateTokenPair("test-user-id", roles)
		if err != nil {
			t.Fatalf("Failed to generate token pair: %v", err)
		}

		claims, err := authService.Token.ValidateAccessToken(tokenPair.AccessToken)
		if err != nil {
			t.Fatalf("Failed to validate token: %v", err)
		}

		if len(claims.Roles) != 2 {
			t.Errorf("Expected 2 roles, got %d", len(claims.Roles))
		}

		// Verify role checking methods
		if !claims.HasRole("user") {
			t.Error("Expected claims to have 'user' role")
		}
		if !claims.HasRole("admin") {
			t.Error("Expected claims to have 'admin' role")
		}
		if claims.HasRole("moderator") {
			t.Error("Expected claims to not have 'moderator' role")
		}
		if !claims.HasAnyRole("admin", "superuser") {
			t.Error("Expected claims to have at least one of 'admin' or 'superuser'")
		}
		if !claims.IsAdmin() {
			t.Error("Expected claims.IsAdmin() to be true")
		}
	})
}
