package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/models"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"go.uber.org/zap"
)

// Executor interface for executing queries (works with *sql.DB, *sql.Tx, and db.Transaction)
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Common errors for OAuth operations.
var (
	// ErrUserNotFound indicates no user was found with the given criteria.
	ErrUserNotFound = errors.New("user not found")

	// ErrOAuthAccountNotFound indicates no OAuth account was found.
	ErrOAuthAccountNotFound = errors.New("oauth account not found")

	// ErrOAuthAccountExists indicates the OAuth account is already linked.
	ErrOAuthAccountExists = errors.New("oauth account already exists")

	// ErrEmailAlreadyLinked indicates the email is already linked to another OAuth account.
	ErrEmailAlreadyLinked = errors.New("email already linked to another oauth account")

	// ErrInvalidProvider indicates an invalid OAuth provider was specified.
	ErrInvalidProvider = errors.New("invalid oauth provider")
)

// OAuthService handles OAuth-related operations.
type OAuthService struct {
	pool   *db.Pool
	logger *zap.Logger
}

// NewOAuthService creates a new OAuthService instance.
func NewOAuthService(pool *db.Pool, logger *zap.Logger) *OAuthService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &OAuthService{
		pool:   pool,
		logger: logger,
	}
}

// FindUserByOAuthProvider finds a user by their OAuth provider and provider user ID.
// Returns the user and OAuth account if found, ErrOAuthAccountNotFound if not found.
func (s *OAuthService) FindUserByOAuthProvider(ctx context.Context, provider models.OAuthProvider, providerUserID string) (*models.User, *models.OAuthAccount, error) {
	if !provider.Valid() {
		return nil, nil, ErrInvalidProvider
	}

	query := `
		SELECT
			u.id, u.email, u.password_hash, u.email_verified,
			u.display_name, u.avatar_url, u.username, u.bio, u.country, u.phone,
			u.created_at, u.updated_at,
			oa.id, oa.user_id, oa.provider, oa.provider_user_id, oa.email,
			oa.access_token, oa.refresh_token, oa.token_expires_at,
			oa.created_at, oa.updated_at
		FROM oauth_accounts oa
		INNER JOIN users u ON u.id = oa.user_id
		WHERE oa.provider = $1 AND oa.provider_user_id = $2
	`

	var user models.User
	var oauth models.OAuthAccount
	var passwordHash, displayName, avatarURL, username, bio, country, phone sql.NullString
	var userUpdatedAt sql.NullTime
	var oauthEmail, accessToken, refreshToken sql.NullString
	var tokenExpiresAt sql.NullTime

	err := s.pool.Primary().QueryRowContext(ctx, query, provider.String(), providerUserID).Scan(
		&user.ID, &user.Email, &passwordHash, &user.EmailVerified,
		&displayName, &avatarURL, &username, &bio, &country, &phone,
		&user.CreatedAt, &userUpdatedAt,
		&oauth.ID, &oauth.UserID, &oauth.Provider, &oauth.ProviderUserID, &oauthEmail,
		&accessToken, &refreshToken, &tokenExpiresAt,
		&oauth.CreatedAt, &oauth.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrOAuthAccountNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query oauth account: %w", err)
	}

	// Map nullable fields
	if passwordHash.Valid {
		user.PasswordHash = &passwordHash.String
	}
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	if country.Valid {
		user.Country = &country.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if userUpdatedAt.Valid {
		user.UpdatedAt = &userUpdatedAt.Time
	}
	if oauthEmail.Valid {
		oauth.Email = &oauthEmail.String
	}
	if accessToken.Valid {
		oauth.AccessToken = &accessToken.String
	}
	if refreshToken.Valid {
		oauth.RefreshToken = &refreshToken.String
	}
	if tokenExpiresAt.Valid {
		oauth.TokenExpiresAt = &tokenExpiresAt.Time
	}

	return &user, &oauth, nil
}

// FindUserByEmail finds a user by their email address.
// Returns the user if found, ErrUserNotFound if not found.
func (s *OAuthService) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT
			id, email, password_hash, email_verified,
			display_name, avatar_url, username, bio, country, phone,
			created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user models.User
	var passwordHash, displayName, avatarURL, username, bio, country, phone sql.NullString
	var updatedAt sql.NullTime

	err := s.pool.Primary().QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &passwordHash, &user.EmailVerified,
		&displayName, &avatarURL, &username, &bio, &country, &phone,
		&user.CreatedAt, &updatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	// Map nullable fields
	if passwordHash.Valid {
		user.PasswordHash = &passwordHash.String
	}
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	if country.Valid {
		user.Country = &country.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	return &user, nil
}

// CreateOAuthUser creates a new user from OAuth provider information.
// This creates both the user record and the OAuth account link.
// Returns the OAuth result with the new user ID.
func (s *OAuthService) CreateOAuthUser(ctx context.Context, params models.CreateOAuthUserParams) (*models.OAuthResult, error) {
	if !params.Provider.Valid() {
		return nil, ErrInvalidProvider
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create user with email verified (OAuth provider verified it)
	preferredLang := params.PreferredLang
	if preferredLang == "" {
		preferredLang = "fa"
	}
	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (email, email_verified, display_name, avatar_url, preferred_lang)
		 VALUES ($1, TRUE, NULLIF($2, ''), NULLIF($3, ''), $4)
		 RETURNING id`,
		params.Email, params.Name, params.AvatarURL, preferredLang,
	).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Assign default "user" role
	var roleID int
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = 'user'`).Scan(&roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user role: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
		userID, roleID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to assign role: %w", err)
	}

	// Create OAuth account link
	oauthQuery := `
		INSERT INTO oauth_accounts (user_id, provider, provider_user_id, email, access_token, refresh_token, token_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	var accessToken, refreshToken *string
	var expiresAt interface{}

	if params.Tokens != nil {
		accessToken = &params.Tokens.AccessToken
		refreshToken = &params.Tokens.RefreshToken
		expiresAt = params.Tokens.ExpiresAt
	}

	_, err = tx.ExecContext(ctx, oauthQuery,
		userID, params.Provider.String(), params.ProviderUserID, params.Email,
		accessToken, refreshToken, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Created new user via OAuth",
		zap.String("user_id", userID),
		zap.String("provider", params.Provider.String()),
		zap.String("provider_user_id", params.ProviderUserID),
		zap.String("email", params.Email))

	return &models.OAuthResult{
		UserID:      userID,
		IsNewUser:   true,
		WasLinked:   false,
		HasPassword: false,
	}, nil
}

// LinkOAuthAccount links an OAuth account to an existing user.
// This is used when a user with an existing account signs in via OAuth.
// Also marks the user's email as verified since OAuth provider verified it.
// Returns ErrOAuthAccountExists if the OAuth account is already linked to any user.
func (s *OAuthService) LinkOAuthAccount(ctx context.Context, params models.LinkOAuthAccountParams) error {
	if !params.Provider.Valid() {
		return ErrInvalidProvider
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if OAuth account already exists
	var existingUserID string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM oauth_accounts WHERE provider = $1 AND provider_user_id = $2`,
		params.Provider.String(), params.ProviderUserID,
	).Scan(&existingUserID)

	if err == nil {
		// OAuth account exists
		if existingUserID == params.UserID {
			// Already linked to this user, just update tokens
			return s.updateOAuthTokens(ctx, tx, params)
		}
		return ErrOAuthAccountExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check existing oauth account: %w", err)
	}

	// Create the OAuth account link
	oauthQuery := `
		INSERT INTO oauth_accounts (user_id, provider, provider_user_id, email, access_token, refresh_token, token_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	var accessToken, refreshToken *string
	var expiresAt interface{}

	if params.Tokens != nil {
		accessToken = &params.Tokens.AccessToken
		refreshToken = &params.Tokens.RefreshToken
		expiresAt = params.Tokens.ExpiresAt
	}

	_, err = tx.ExecContext(ctx, oauthQuery,
		params.UserID, params.Provider.String(), params.ProviderUserID, params.Email,
		accessToken, refreshToken, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to link oauth account: %w", err)
	}

	// Mark email as verified (OAuth provider verified it)
	_, err = tx.ExecContext(ctx,
		`UPDATE users SET email_verified = TRUE WHERE id = $1 AND email_verified = FALSE`,
		params.UserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update email verification: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Linked OAuth account to existing user",
		zap.String("user_id", params.UserID),
		zap.String("provider", params.Provider.String()),
		zap.String("provider_user_id", params.ProviderUserID))

	return nil
}

// updateOAuthTokens updates the tokens for an existing OAuth account.
func (s *OAuthService) updateOAuthTokens(ctx context.Context, tx *db.Transaction, params models.LinkOAuthAccountParams) error {
	query := `
		UPDATE oauth_accounts
		SET access_token = $1, refresh_token = $2, token_expires_at = $3, email = $4
		WHERE provider = $5 AND provider_user_id = $6
	`
	var accessToken, refreshToken *string
	var expiresAt interface{}

	if params.Tokens != nil {
		accessToken = &params.Tokens.AccessToken
		refreshToken = &params.Tokens.RefreshToken
		expiresAt = params.Tokens.ExpiresAt
	}

	_, err := tx.ExecContext(ctx, query,
		accessToken, refreshToken, expiresAt, params.Email,
		params.Provider.String(), params.ProviderUserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update oauth tokens: %w", err)
	}

	return tx.Commit()
}

// GetUserOAuthAccounts returns all OAuth accounts linked to a user.
func (s *OAuthService) GetUserOAuthAccounts(ctx context.Context, userID string) ([]models.OAuthAccountInfo, error) {
	query := `
		SELECT id, provider, email, created_at, updated_at
		FROM oauth_accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Primary().QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query oauth accounts: %w", err)
	}
	defer rows.Close()

	var accounts []models.OAuthAccountInfo
	for rows.Next() {
		var account models.OAuthAccountInfo
		var email sql.NullString

		err := rows.Scan(&account.ID, &account.Provider, &email, &account.LinkedAt, &account.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan oauth account: %w", err)
		}

		if email.Valid {
			account.Email = &email.String
		}
		accounts = append(accounts, account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating oauth accounts: %w", err)
	}

	return accounts, nil
}

// UpdateOAuthTokens updates the OAuth tokens for an existing account.
// This is called when tokens are refreshed during OAuth flow.
func (s *OAuthService) UpdateOAuthTokens(ctx context.Context, provider models.OAuthProvider, providerUserID string, tokens *models.OAuthTokens, email string) error {
	if !provider.Valid() {
		return ErrInvalidProvider
	}

	query := `
		UPDATE oauth_accounts
		SET access_token = $1, refresh_token = $2, token_expires_at = $3, email = $4
		WHERE provider = $5 AND provider_user_id = $6
	`

	result, err := s.pool.Primary().ExecContext(ctx, query,
		tokens.AccessToken, tokens.RefreshToken, tokens.ExpiresAt, email,
		provider.String(), providerUserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update oauth tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrOAuthAccountNotFound
	}

	return nil
}

// ProcessOAuthLogin handles the complete OAuth login flow.
// This method handles all edge cases:
// - 5.6: User registered with email, now signs in with OAuth (same email) - links account
// - 5.7: User has password, adds OAuth login - links account
// Returns the result indicating user ID, whether new user, and whether account was linked.
func (s *OAuthService) ProcessOAuthLogin(ctx context.Context, provider models.OAuthProvider, userInfo models.OAuthUserInfo, tokens *models.OAuthTokens, preferredLang string) (*models.OAuthResult, error) {
	if !provider.Valid() {
		return nil, ErrInvalidProvider
	}

	// Step 1: Check if OAuth account already exists
	user, oauthAccount, err := s.FindUserByOAuthProvider(ctx, provider, userInfo.ProviderUserID)
	if err == nil {
		// OAuth account exists - update tokens and return existing user
		currentEmail := ""
		if oauthAccount.Email != nil {
			currentEmail = *oauthAccount.Email
		}

		// Log if email changed
		if currentEmail != "" && currentEmail != userInfo.Email {
			s.logger.Warn("OAuth email changed",
				zap.String("provider", provider.String()),
				zap.String("provider_user_id", userInfo.ProviderUserID),
				zap.String("old_email", currentEmail),
				zap.String("new_email", userInfo.Email))
		}

		// Update tokens
		err = s.UpdateOAuthTokens(ctx, provider, userInfo.ProviderUserID, tokens, userInfo.Email)
		if err != nil {
			return nil, fmt.Errorf("failed to update oauth tokens: %w", err)
		}

		return &models.OAuthResult{
			UserID:      user.ID,
			IsNewUser:   false,
			WasLinked:   false,
			HasPassword: user.HasPassword(),
		}, nil
	}

	if !errors.Is(err, ErrOAuthAccountNotFound) {
		return nil, err
	}

	// Step 2: OAuth account doesn't exist - check if user with this email exists
	// This handles edge case 5.6: User registered with email, now signs in with OAuth
	existingUser, err := s.FindUserByEmail(ctx, userInfo.Email)
	if err == nil {
		// User exists with this email - link the OAuth account
		// This also handles edge case 5.7: User has password, adds OAuth login
		linkParams := models.LinkOAuthAccountParams{
			UserID:         existingUser.ID,
			Provider:       provider,
			ProviderUserID: userInfo.ProviderUserID,
			Email:          userInfo.Email,
			Tokens:         tokens,
		}

		if err := s.LinkOAuthAccount(ctx, linkParams); err != nil {
			return nil, fmt.Errorf("failed to link oauth account: %w", err)
		}

		s.logger.Info("Linked OAuth to existing user with matching email",
			zap.String("user_id", existingUser.ID),
			zap.String("provider", provider.String()),
			zap.Bool("has_password", existingUser.HasPassword()))

		return &models.OAuthResult{
			UserID:      existingUser.ID,
			IsNewUser:   false,
			WasLinked:   true,
			HasPassword: existingUser.HasPassword(),
		}, nil
	}

	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	// Step 3: No user exists - create new user
	createParams := models.CreateOAuthUserParams{
		Email:          userInfo.Email,
		Name:           userInfo.Name,
		AvatarURL:      userInfo.Picture,
		Provider:       provider,
		ProviderUserID: userInfo.ProviderUserID,
		Tokens:         tokens,
		PreferredLang:  preferredLang,
	}

	return s.CreateOAuthUser(ctx, createParams)
}

// UnlinkOAuthAccount removes an OAuth account link from a user.
// This is used when a user wants to disconnect a social login.
// Returns an error if the user has no password and this is their only OAuth account.
func (s *OAuthService) UnlinkOAuthAccount(ctx context.Context, userID string, provider models.OAuthProvider) error {
	if !provider.Valid() {
		return ErrInvalidProvider
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if user has a password
	var passwordHash sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to query user: %w", err)
	}

	hasPassword := passwordHash.Valid && passwordHash.String != ""

	// Count OAuth accounts
	var oauthCount int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM oauth_accounts WHERE user_id = $1`,
		userID,
	).Scan(&oauthCount)
	if err != nil {
		return fmt.Errorf("failed to count oauth accounts: %w", err)
	}

	// If no password and only one OAuth account, don't allow unlinking
	if !hasPassword && oauthCount <= 1 {
		return errors.New("cannot unlink: no password set and this is the only login method")
	}

	// Delete the OAuth account
	result, err := tx.ExecContext(ctx,
		`DELETE FROM oauth_accounts WHERE user_id = $1 AND provider = $2`,
		userID, provider.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete oauth account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrOAuthAccountNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Unlinked OAuth account",
		zap.String("user_id", userID),
		zap.String("provider", provider.String()))

	return nil
}

// HasOAuthAccount checks if a user has an OAuth account with the specified provider.
func (s *OAuthService) HasOAuthAccount(ctx context.Context, userID string, provider models.OAuthProvider) (bool, error) {
	if !provider.Valid() {
		return false, ErrInvalidProvider
	}

	var exists bool
	err := s.pool.Primary().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM oauth_accounts WHERE user_id = $1 AND provider = $2)`,
		userID, provider.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check oauth account: %w", err)
	}

	return exists, nil
}
