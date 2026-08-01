package models

import (
	"time"
)

// OAuthProvider represents supported OAuth providers.
type OAuthProvider string

const (
	OAuthProviderGoogle   OAuthProvider = "google"
	OAuthProviderGitHub   OAuthProvider = "github"
	OAuthProviderFacebook OAuthProvider = "facebook"
	OAuthProviderApple    OAuthProvider = "apple"
	OAuthProviderDiscord  OAuthProvider = "discord"
)

// Valid returns true if the provider is a known valid provider.
func (p OAuthProvider) Valid() bool {
	switch p {
	case OAuthProviderGoogle, OAuthProviderGitHub, OAuthProviderFacebook, OAuthProviderApple, OAuthProviderDiscord:
		return true
	default:
		return false
	}
}

// String returns the string representation of the provider.
func (p OAuthProvider) String() string {
	return string(p)
}

// OAuthAccount represents a linked OAuth provider account for a user.
type OAuthAccount struct {
	ID             string        `json:"id"`
	UserID         string        `json:"user_id"`
	Provider       OAuthProvider `json:"provider"`
	ProviderUserID string        `json:"provider_user_id"`
	Email          *string       `json:"email,omitempty"`
	AccessToken    *string       `json:"-"` // Never expose tokens in JSON
	RefreshToken   *string       `json:"-"` // Never expose tokens in JSON
	TokenExpiresAt *time.Time    `json:"token_expires_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// OAuthAccountInfo is a safe representation of an OAuth account for API responses.
type OAuthAccountInfo struct {
	ID        string        `json:"id"`
	Provider  OAuthProvider `json:"provider"`
	Email     *string       `json:"email,omitempty"`
	LinkedAt  time.Time     `json:"linked_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ToInfo converts an OAuthAccount to its safe API representation.
func (o *OAuthAccount) ToInfo() OAuthAccountInfo {
	return OAuthAccountInfo{
		ID:        o.ID,
		Provider:  o.Provider,
		Email:     o.Email,
		LinkedAt:  o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

// User represents a user in the system.
type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	PasswordHash  *string    `json:"-"` // Nullable for OAuth-only users
	EmailVerified bool       `json:"email_verified"`
	DisplayName   *string    `json:"display_name,omitempty"`
	AvatarURL     *string    `json:"avatar_url,omitempty"`
	Username      *string    `json:"username,omitempty"`
	Bio           *string    `json:"bio,omitempty"`
	Country       *string    `json:"country,omitempty"`
	Phone         *string    `json:"phone,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	Roles         []string   `json:"roles,omitempty"`
}

// HasPassword returns true if the user has a password set.
func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

// OAuthUserInfo contains information from an OAuth provider about a user.
type OAuthUserInfo struct {
	ProviderUserID string  `json:"provider_user_id"`
	Email          string  `json:"email"`
	EmailVerified  bool    `json:"email_verified"`
	Name           string  `json:"name,omitempty"`
	GivenName      string  `json:"given_name,omitempty"`
	FamilyName     string  `json:"family_name,omitempty"`
	Picture        string  `json:"picture,omitempty"`
}

// OAuthTokens contains OAuth token information.
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// CreateOAuthUserParams contains parameters for creating a new OAuth user.
type CreateOAuthUserParams struct {
	Email          string
	Name           string
	AvatarURL      string
	Provider       OAuthProvider
	ProviderUserID string
	Tokens         *OAuthTokens
	PreferredLang  string // "fa" or "en"
}

// LinkOAuthAccountParams contains parameters for linking an OAuth account.
type LinkOAuthAccountParams struct {
	UserID         string
	Provider       OAuthProvider
	ProviderUserID string
	Email          string
	Tokens         *OAuthTokens
}

// OAuthResult represents the result of an OAuth operation.
type OAuthResult struct {
	UserID      string `json:"user_id"`
	IsNewUser   bool   `json:"is_new_user"`
	WasLinked   bool   `json:"was_linked"` // True if an existing user had OAuth linked
	HasPassword bool   `json:"has_password"`
}
