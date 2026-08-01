package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/models"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"go.uber.org/zap"
)

// TestFindUserByOAuthProvider_ExistingUser tests finding an existing user by OAuth provider.
func TestFindUserByOAuthProvider_ExistingUser(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	// Create a test pool using the mock
	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	// Expected data
	userID := "user-123"
	email := "test@example.com"
	passwordHash := "hashed_password"
	oauthID := "oauth-456"
	providerUserID := "google-789"
	oauthEmail := "test@example.com"
	accessToken := "access_token_123"
	refreshToken := "refresh_token_456"
	now := time.Now()

	// Set up expectations
	rows := sqlmock.NewRows([]string{
		"id", "email", "password_hash", "email_verified",
		"display_name", "avatar_url", "username", "bio", "country", "phone",
		"created_at", "updated_at",
		"oauth_id", "oauth_user_id", "provider", "provider_user_id", "oauth_email",
		"access_token", "refresh_token", "token_expires_at",
		"oauth_created_at", "oauth_updated_at",
	}).AddRow(
		userID, email, passwordHash, true,
		"Test User", "https://example.com/avatar.png", "testuser", "bio", "US", "+1234567890",
		now, now,
		oauthID, userID, "google", providerUserID, oauthEmail,
		accessToken, refreshToken, now.Add(time.Hour),
		now, now,
	)

	mock.ExpectQuery(`SELECT.*FROM oauth_accounts.*INNER JOIN users`).
		WithArgs("google", providerUserID).
		WillReturnRows(rows)

	// Execute test
	ctx := context.Background()
	user, oauth, err := service.FindUserByOAuthProvider(ctx, models.OAuthProviderGoogle, providerUserID)

	// Verify results
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if user == nil {
		t.Fatal("Expected user to be returned, got nil")
	}
	if oauth == nil {
		t.Fatal("Expected oauth account to be returned, got nil")
	}

	// Verify user fields
	if user.ID != userID {
		t.Errorf("User ID = %v, want %v", user.ID, userID)
	}
	if user.Email != email {
		t.Errorf("User Email = %v, want %v", user.Email, email)
	}
	if !user.HasPassword() {
		t.Error("Expected user to have password")
	}
	if !user.EmailVerified {
		t.Error("Expected email to be verified")
	}

	// Verify oauth account fields
	if oauth.ID != oauthID {
		t.Errorf("OAuth ID = %v, want %v", oauth.ID, oauthID)
	}
	if oauth.Provider != models.OAuthProviderGoogle {
		t.Errorf("OAuth Provider = %v, want %v", oauth.Provider, models.OAuthProviderGoogle)
	}
	if oauth.ProviderUserID != providerUserID {
		t.Errorf("OAuth ProviderUserID = %v, want %v", oauth.ProviderUserID, providerUserID)
	}

	// Verify all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestFindUserByOAuthProvider_UserNotFound tests the case when no user is found.
func TestFindUserByOAuthProvider_UserNotFound(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	// Create a test pool using the mock
	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	providerUserID := "nonexistent-google-id"

	// Set up expectations - return no rows
	mock.ExpectQuery(`SELECT.*FROM oauth_accounts.*INNER JOIN users`).
		WithArgs("google", providerUserID).
		WillReturnError(sql.ErrNoRows)

	// Execute test
	ctx := context.Background()
	user, oauth, err := service.FindUserByOAuthProvider(ctx, models.OAuthProviderGoogle, providerUserID)

	// Verify results
	if !errors.Is(err, ErrOAuthAccountNotFound) {
		t.Errorf("Expected ErrOAuthAccountNotFound, got %v", err)
	}
	if user != nil {
		t.Errorf("Expected nil user, got %v", user)
	}
	if oauth != nil {
		t.Errorf("Expected nil oauth, got %v", oauth)
	}

	// Verify all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestFindUserByOAuthProvider_DatabaseError tests handling of database errors.
func TestFindUserByOAuthProvider_DatabaseError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	providerUserID := "google-123"
	dbErr := errors.New("connection refused")

	mock.ExpectQuery(`SELECT.*FROM oauth_accounts.*INNER JOIN users`).
		WithArgs("google", providerUserID).
		WillReturnError(dbErr)

	ctx := context.Background()
	user, oauth, err := service.FindUserByOAuthProvider(ctx, models.OAuthProviderGoogle, providerUserID)

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if user != nil {
		t.Error("Expected nil user")
	}
	if oauth != nil {
		t.Error("Expected nil oauth")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestCreateOAuthUser_Success tests successful creation of a new OAuth user.
func TestCreateOAuthUser_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	params := models.CreateOAuthUserParams{
		Email:          "newuser@example.com",
		Name:           "New User",
		AvatarURL:      "https://example.com/avatar.png",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-new-123",
		Tokens: &models.OAuthTokens{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresAt:    time.Now().Add(time.Hour),
		},
	}

	newUserID := "new-user-uuid-123"
	roleID := 1

	// Set up expectations
	mock.ExpectBegin()

	// Insert user
	mock.ExpectQuery(`INSERT INTO users.*RETURNING id`).
		WithArgs(params.Email, params.Name, params.AvatarURL, "fa").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(newUserID))

	// Get role ID
	mock.ExpectQuery(`SELECT id FROM roles WHERE name = 'user'`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(roleID))

	// Assign role
	mock.ExpectExec(`INSERT INTO user_roles`).
		WithArgs(newUserID, roleID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Create OAuth account
	mock.ExpectExec(`INSERT INTO oauth_accounts`).
		WithArgs(newUserID, "google", params.ProviderUserID, params.Email,
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	// Execute test
	ctx := context.Background()
	result, err := service.CreateOAuthUser(ctx, params)

	// Verify results
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result to be returned, got nil")
	}
	if result.UserID != newUserID {
		t.Errorf("UserID = %v, want %v", result.UserID, newUserID)
	}
	if !result.IsNewUser {
		t.Error("Expected IsNewUser to be true")
	}
	if result.WasLinked {
		t.Error("Expected WasLinked to be false for new user")
	}
	if result.HasPassword {
		t.Error("Expected HasPassword to be false for OAuth-only user")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestCreateOAuthUser_WithoutTokens tests creation without OAuth tokens.
func TestCreateOAuthUser_WithoutTokens(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	params := models.CreateOAuthUserParams{
		Email:          "newuser@example.com",
		Name:           "New User",
		AvatarURL:      "",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-new-123",
		Tokens:         nil, // No tokens
	}

	newUserID := "new-user-uuid-123"
	roleID := 1

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO users.*RETURNING id`).
		WithArgs(params.Email, params.Name, params.AvatarURL, "fa").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(newUserID))
	mock.ExpectQuery(`SELECT id FROM roles WHERE name = 'user'`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(roleID))
	mock.ExpectExec(`INSERT INTO user_roles`).
		WithArgs(newUserID, roleID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO oauth_accounts`).
		WithArgs(newUserID, "google", params.ProviderUserID, params.Email,
			nil, nil, nil). // nil tokens
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctx := context.Background()
	result, err := service.CreateOAuthUser(ctx, params)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.UserID != newUserID {
		t.Errorf("UserID = %v, want %v", result.UserID, newUserID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestCreateOAuthUser_TransactionRollback tests transaction rollback on error.
func TestCreateOAuthUser_TransactionRollback(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	params := models.CreateOAuthUserParams{
		Email:          "newuser@example.com",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-123",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO users.*RETURNING id`).
		WithArgs(params.Email, params.Name, params.AvatarURL, "fa").
		WillReturnError(errors.New("duplicate key"))
	mock.ExpectRollback()

	ctx := context.Background()
	result, err := service.CreateOAuthUser(ctx, params)

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if result != nil {
		t.Error("Expected nil result")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestLinkOAuthAccount_Success tests successfully linking an OAuth account to existing user.
func TestLinkOAuthAccount_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	params := models.LinkOAuthAccountParams{
		UserID:         "existing-user-123",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-456",
		Email:          "existing@example.com",
		Tokens: &models.OAuthTokens{
			AccessToken:  "new_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresAt:    time.Now().Add(time.Hour),
		},
	}

	mock.ExpectBegin()

	// Check if OAuth account exists - should not find one
	mock.ExpectQuery(`SELECT user_id FROM oauth_accounts WHERE provider = .* AND provider_user_id = .*`).
		WithArgs("google", params.ProviderUserID).
		WillReturnError(sql.ErrNoRows)

	// Insert OAuth account
	mock.ExpectExec(`INSERT INTO oauth_accounts`).
		WithArgs(params.UserID, "google", params.ProviderUserID, params.Email,
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Update email verification
	mock.ExpectExec(`UPDATE users SET email_verified = TRUE`).
		WithArgs(params.UserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	ctx := context.Background()
	err = service.LinkOAuthAccount(ctx, params)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestLinkOAuthAccount_AlreadyLinkedToSameUser tests linking when OAuth is already linked to the same user.
func TestLinkOAuthAccount_AlreadyLinkedToSameUser(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "existing-user-123"
	params := models.LinkOAuthAccountParams{
		UserID:         userID,
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-456",
		Email:          "existing@example.com",
		Tokens: &models.OAuthTokens{
			AccessToken:  "new_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresAt:    time.Now().Add(time.Hour),
		},
	}

	mock.ExpectBegin()

	// Check if OAuth account exists - found and linked to same user
	mock.ExpectQuery(`SELECT user_id FROM oauth_accounts WHERE provider = .* AND provider_user_id = .*`).
		WithArgs("google", params.ProviderUserID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(userID))

	// Should update tokens instead of creating new link
	mock.ExpectExec(`UPDATE oauth_accounts.*SET access_token.*refresh_token.*token_expires_at.*email`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), params.Email,
			"google", params.ProviderUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	ctx := context.Background()
	err = service.LinkOAuthAccount(ctx, params)

	// Should succeed (updating tokens)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestLinkOAuthAccount_AlreadyLinkedToDifferentUser tests linking when OAuth is already linked to a different user.
func TestLinkOAuthAccount_AlreadyLinkedToDifferentUser(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	params := models.LinkOAuthAccountParams{
		UserID:         "user-trying-to-link",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-456",
		Email:          "test@example.com",
	}

	differentUserID := "different-user-789"

	mock.ExpectBegin()

	// Check if OAuth account exists - found but linked to different user
	mock.ExpectQuery(`SELECT user_id FROM oauth_accounts WHERE provider = .* AND provider_user_id = .*`).
		WithArgs("google", params.ProviderUserID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(differentUserID))

	mock.ExpectRollback()

	ctx := context.Background()
	err = service.LinkOAuthAccount(ctx, params)

	if !errors.Is(err, ErrOAuthAccountExists) {
		t.Errorf("Expected ErrOAuthAccountExists, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestProcessOAuthLogin_ExistingOAuthAccount tests login with existing OAuth account.
func TestProcessOAuthLogin_ExistingOAuthAccount(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "existing-user-123"
	providerUserID := "google-456"
	email := "test@example.com"
	passwordHash := "hashed_password"
	now := time.Now()

	userInfo := models.OAuthUserInfo{
		ProviderUserID: providerUserID,
		Email:          email,
		EmailVerified:  true,
		Name:           "Test User",
	}

	tokens := &models.OAuthTokens{
		AccessToken:  "new_access_token",
		RefreshToken: "new_refresh_token",
		ExpiresAt:    now.Add(time.Hour),
	}

	// FindUserByOAuthProvider - existing user found
	rows := sqlmock.NewRows([]string{
		"id", "email", "password_hash", "email_verified",
		"display_name", "avatar_url", "username", "bio", "country", "phone",
		"created_at", "updated_at",
		"oauth_id", "oauth_user_id", "provider", "provider_user_id", "oauth_email",
		"access_token", "refresh_token", "token_expires_at",
		"oauth_created_at", "oauth_updated_at",
	}).AddRow(
		userID, email, passwordHash, true,
		"Test User", nil, nil, nil, nil, nil,
		now, now,
		"oauth-123", userID, "google", providerUserID, email,
		"old_access_token", "old_refresh_token", now,
		now, now,
	)

	mock.ExpectQuery(`SELECT.*FROM oauth_accounts.*INNER JOIN users`).
		WithArgs("google", providerUserID).
		WillReturnRows(rows)

	// UpdateOAuthTokens
	mock.ExpectExec(`UPDATE oauth_accounts.*SET access_token.*refresh_token.*token_expires_at.*email`).
		WithArgs(tokens.AccessToken, tokens.RefreshToken, tokens.ExpiresAt, email, "google", providerUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	result, err := service.ProcessOAuthLogin(ctx, models.OAuthProviderGoogle, userInfo, tokens, "fa")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.UserID != userID {
		t.Errorf("UserID = %v, want %v", result.UserID, userID)
	}
	if result.IsNewUser {
		t.Error("Expected IsNewUser to be false")
	}
	if result.WasLinked {
		t.Error("Expected WasLinked to be false")
	}
	if !result.HasPassword {
		t.Error("Expected HasPassword to be true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestProcessOAuthLogin_LinkToExistingUser tests OAuth login that links to existing user with same email.
func TestProcessOAuthLogin_LinkToExistingUser(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "existing-user-123"
	providerUserID := "google-new-456"
	email := "test@example.com"
	passwordHash := "hashed_password"
	now := time.Now()

	userInfo := models.OAuthUserInfo{
		ProviderUserID: providerUserID,
		Email:          email,
		EmailVerified:  true,
		Name:           "Test User",
	}

	tokens := &models.OAuthTokens{
		AccessToken:  "new_access_token",
		RefreshToken: "new_refresh_token",
		ExpiresAt:    now.Add(time.Hour),
	}

	// Step 1: FindUserByOAuthProvider - not found
	mock.ExpectQuery(`SELECT.*FROM oauth_accounts.*INNER JOIN users`).
		WithArgs("google", providerUserID).
		WillReturnError(sql.ErrNoRows)

	// Step 2: FindUserByEmail - found
	userRows := sqlmock.NewRows([]string{
		"id", "email", "password_hash", "email_verified",
		"display_name", "avatar_url", "username", "bio", "country", "phone",
		"created_at", "updated_at",
	}).AddRow(
		userID, email, passwordHash, true,
		"Test User", nil, nil, nil, nil, nil,
		now, now,
	)
	mock.ExpectQuery(`SELECT.*FROM users.*WHERE email = .*`).
		WithArgs(email).
		WillReturnRows(userRows)

	// Step 3: LinkOAuthAccount
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM oauth_accounts WHERE provider = .* AND provider_user_id = .*`).
		WithArgs("google", providerUserID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO oauth_accounts`).
		WithArgs(userID, "google", providerUserID, email,
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET email_verified = TRUE`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ctx := context.Background()
	result, err := service.ProcessOAuthLogin(ctx, models.OAuthProviderGoogle, userInfo, tokens, "fa")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.UserID != userID {
		t.Errorf("UserID = %v, want %v", result.UserID, userID)
	}
	if result.IsNewUser {
		t.Error("Expected IsNewUser to be false")
	}
	if !result.WasLinked {
		t.Error("Expected WasLinked to be true")
	}
	if !result.HasPassword {
		t.Error("Expected HasPassword to be true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestFindUserByEmail_Found tests finding user by email.
func TestFindUserByEmail_Found(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"
	email := "test@example.com"
	passwordHash := "hashed_password"
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "email", "password_hash", "email_verified",
		"display_name", "avatar_url", "username", "bio", "country", "phone",
		"created_at", "updated_at",
	}).AddRow(
		userID, email, passwordHash, true,
		"Test User", nil, nil, nil, nil, nil,
		now, now,
	)

	mock.ExpectQuery(`SELECT.*FROM users.*WHERE email = .*`).
		WithArgs(email).
		WillReturnRows(rows)

	ctx := context.Background()
	user, err := service.FindUserByEmail(ctx, email)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if user == nil {
		t.Fatal("Expected user, got nil")
	}
	if user.ID != userID {
		t.Errorf("User ID = %v, want %v", user.ID, userID)
	}
	if user.Email != email {
		t.Errorf("User Email = %v, want %v", user.Email, email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestFindUserByEmail_NotFound tests finding user by email when not found.
func TestFindUserByEmail_NotFound(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	email := "nonexistent@example.com"

	mock.ExpectQuery(`SELECT.*FROM users.*WHERE email = .*`).
		WithArgs(email).
		WillReturnError(sql.ErrNoRows)

	ctx := context.Background()
	user, err := service.FindUserByEmail(ctx, email)

	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
	if user != nil {
		t.Errorf("Expected nil user, got %v", user)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestUpdateOAuthTokens_Success tests successful token update.
func TestUpdateOAuthTokens_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	providerUserID := "google-123"
	email := "test@example.com"
	tokens := &models.OAuthTokens{
		AccessToken:  "new_access",
		RefreshToken: "new_refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mock.ExpectExec(`UPDATE oauth_accounts.*SET access_token.*refresh_token.*token_expires_at.*email`).
		WithArgs(tokens.AccessToken, tokens.RefreshToken, tokens.ExpiresAt, email, "google", providerUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err = service.UpdateOAuthTokens(ctx, models.OAuthProviderGoogle, providerUserID, tokens, email)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestUpdateOAuthTokens_NotFound tests token update when account not found.
func TestUpdateOAuthTokens_NotFound(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	providerUserID := "nonexistent-google-id"
	tokens := &models.OAuthTokens{
		AccessToken:  "new_access",
		RefreshToken: "new_refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mock.ExpectExec(`UPDATE oauth_accounts.*SET access_token.*refresh_token.*token_expires_at.*email`).
		WithArgs(tokens.AccessToken, tokens.RefreshToken, tokens.ExpiresAt, "", "google", providerUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	ctx := context.Background()
	err = service.UpdateOAuthTokens(ctx, models.OAuthProviderGoogle, providerUserID, tokens, "")

	if !errors.Is(err, ErrOAuthAccountNotFound) {
		t.Errorf("Expected ErrOAuthAccountNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestUnlinkOAuthAccount_Success tests successfully unlinking OAuth account.
func TestUnlinkOAuthAccount_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"
	passwordHash := "hashed_password"

	mock.ExpectBegin()

	// Check if user has password
	mock.ExpectQuery(`SELECT password_hash FROM users WHERE id = .*`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(passwordHash))

	// Count OAuth accounts
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_accounts WHERE user_id = .*`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Delete OAuth account
	mock.ExpectExec(`DELETE FROM oauth_accounts WHERE user_id = .* AND provider = .*`).
		WithArgs(userID, "google").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	ctx := context.Background()
	err = service.UnlinkOAuthAccount(ctx, userID, models.OAuthProviderGoogle)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestUnlinkOAuthAccount_NoPasswordSingleOAuth tests preventing unlink when user has no password and single OAuth.
func TestUnlinkOAuthAccount_NoPasswordSingleOAuth(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"

	mock.ExpectBegin()

	// Check if user has password - no password
	mock.ExpectQuery(`SELECT password_hash FROM users WHERE id = .*`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(nil))

	// Count OAuth accounts - only one
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_accounts WHERE user_id = .*`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectRollback()

	ctx := context.Background()
	err = service.UnlinkOAuthAccount(ctx, userID, models.OAuthProviderGoogle)

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != "cannot unlink: no password set and this is the only login method" {
		t.Errorf("Unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestHasOAuthAccount_True tests checking OAuth account exists.
func TestHasOAuthAccount_True(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"

	mock.ExpectQuery(`SELECT EXISTS.*FROM oauth_accounts WHERE user_id = .* AND provider = .*`).
		WithArgs(userID, "google").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	ctx := context.Background()
	exists, err := service.HasOAuthAccount(ctx, userID, models.OAuthProviderGoogle)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !exists {
		t.Error("Expected exists to be true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestHasOAuthAccount_False tests checking OAuth account doesn't exist.
func TestHasOAuthAccount_False(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"

	mock.ExpectQuery(`SELECT EXISTS.*FROM oauth_accounts WHERE user_id = .* AND provider = .*`).
		WithArgs(userID, "google").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	ctx := context.Background()
	exists, err := service.HasOAuthAccount(ctx, userID, models.OAuthProviderGoogle)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if exists {
		t.Error("Expected exists to be false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestGetUserOAuthAccounts_Success tests getting all OAuth accounts for a user.
func TestGetUserOAuthAccounts_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"
	now := time.Now()
	email1 := "google@example.com"
	email2 := "github@example.com"

	rows := sqlmock.NewRows([]string{"id", "provider", "email", "created_at", "updated_at"}).
		AddRow("oauth-1", "google", email1, now, now).
		AddRow("oauth-2", "github", email2, now.Add(-time.Hour), now)

	mock.ExpectQuery(`SELECT id, provider, email, created_at, updated_at.*FROM oauth_accounts.*WHERE user_id = .*`).
		WithArgs(userID).
		WillReturnRows(rows)

	ctx := context.Background()
	accounts, err := service.GetUserOAuthAccounts(ctx, userID)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].Provider != models.OAuthProviderGoogle {
		t.Errorf("First account provider = %v, want %v", accounts[0].Provider, models.OAuthProviderGoogle)
	}
	if accounts[1].Provider != models.OAuthProviderGitHub {
		t.Errorf("Second account provider = %v, want %v", accounts[1].Provider, models.OAuthProviderGitHub)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestGetUserOAuthAccounts_Empty tests getting OAuth accounts when none exist.
func TestGetUserOAuthAccounts_Empty(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer mockDB.Close()

	pool := db.NewPoolFromDB(mockDB)
	service := NewOAuthService(pool, zap.NewNop())

	userID := "user-123"

	rows := sqlmock.NewRows([]string{"id", "provider", "email", "created_at", "updated_at"})

	mock.ExpectQuery(`SELECT id, provider, email, created_at, updated_at.*FROM oauth_accounts.*WHERE user_id = .*`).
		WithArgs(userID).
		WillReturnRows(rows)

	ctx := context.Background()
	accounts, err := service.GetUserOAuthAccounts(ctx, userID)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if accounts == nil {
		// nil is acceptable for empty results
		accounts = []models.OAuthAccountInfo{}
	}
	if len(accounts) != 0 {
		t.Errorf("Expected 0 accounts, got %d", len(accounts))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}
