package service

import (
	"testing"

	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/models"
)

func TestOAuthProvider_Valid(t *testing.T) {
	tests := []struct {
		name     string
		provider models.OAuthProvider
		want     bool
	}{
		{
			name:     "Google is valid",
			provider: models.OAuthProviderGoogle,
			want:     true,
		},
		{
			name:     "GitHub is valid",
			provider: models.OAuthProviderGitHub,
			want:     true,
		},
		{
			name:     "Facebook is valid",
			provider: models.OAuthProviderFacebook,
			want:     true,
		},
		{
			name:     "Apple is valid",
			provider: models.OAuthProviderApple,
			want:     true,
		},
		{
			name:     "Discord is valid",
			provider: models.OAuthProviderDiscord,
			want:     true,
		},
		{
			name:     "Empty provider is invalid",
			provider: models.OAuthProvider(""),
			want:     false,
		},
		{
			name:     "Unknown provider is invalid",
			provider: models.OAuthProvider("twitter"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.Valid(); got != tt.want {
				t.Errorf("OAuthProvider.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthProvider_String(t *testing.T) {
	tests := []struct {
		name     string
		provider models.OAuthProvider
		want     string
	}{
		{
			name:     "Google",
			provider: models.OAuthProviderGoogle,
			want:     "google",
		},
		{
			name:     "GitHub",
			provider: models.OAuthProviderGitHub,
			want:     "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.String(); got != tt.want {
				t.Errorf("OAuthProvider.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUser_HasPassword(t *testing.T) {
	password := "hashed_password"
	empty := ""

	tests := []struct {
		name string
		user models.User
		want bool
	}{
		{
			name: "User with password",
			user: models.User{
				ID:           "user-1",
				Email:        "test@example.com",
				PasswordHash: &password,
			},
			want: true,
		},
		{
			name: "User without password (nil)",
			user: models.User{
				ID:           "user-2",
				Email:        "oauth@example.com",
				PasswordHash: nil,
			},
			want: false,
		},
		{
			name: "User with empty password",
			user: models.User{
				ID:           "user-3",
				Email:        "empty@example.com",
				PasswordHash: &empty,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.HasPassword(); got != tt.want {
				t.Errorf("User.HasPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthAccount_ToInfo(t *testing.T) {
	email := "test@example.com"

	account := models.OAuthAccount{
		ID:             "oauth-1",
		UserID:         "user-1",
		Provider:       models.OAuthProviderGoogle,
		ProviderUserID: "google-123",
		Email:          &email,
	}

	info := account.ToInfo()

	if info.ID != account.ID {
		t.Errorf("ToInfo().ID = %v, want %v", info.ID, account.ID)
	}
	if info.Provider != account.Provider {
		t.Errorf("ToInfo().Provider = %v, want %v", info.Provider, account.Provider)
	}
	if info.Email == nil || *info.Email != email {
		t.Errorf("ToInfo().Email = %v, want %v", info.Email, &email)
	}
	if info.LinkedAt != account.CreatedAt {
		t.Errorf("ToInfo().LinkedAt = %v, want %v", info.LinkedAt, account.CreatedAt)
	}
}

func TestNewOAuthService_NilLogger(t *testing.T) {
	// Should not panic with nil logger
	svc := NewOAuthService(nil, nil)
	if svc == nil {
		t.Error("NewOAuthService returned nil")
	}
	if svc.logger == nil {
		t.Error("NewOAuthService did not set a default logger")
	}
}

func TestOAuthService_InvalidProvider(t *testing.T) {
	svc := NewOAuthService(nil, nil)

	// FindUserByOAuthProvider should return error for invalid provider
	_, _, err := svc.FindUserByOAuthProvider(nil, models.OAuthProvider("invalid"), "123")
	if err != ErrInvalidProvider {
		t.Errorf("FindUserByOAuthProvider() error = %v, want %v", err, ErrInvalidProvider)
	}

	// CreateOAuthUser should return error for invalid provider
	_, err = svc.CreateOAuthUser(nil, models.CreateOAuthUserParams{
		Provider: models.OAuthProvider("invalid"),
	})
	if err != ErrInvalidProvider {
		t.Errorf("CreateOAuthUser() error = %v, want %v", err, ErrInvalidProvider)
	}

	// LinkOAuthAccount should return error for invalid provider
	err = svc.LinkOAuthAccount(nil, models.LinkOAuthAccountParams{
		Provider: models.OAuthProvider("invalid"),
	})
	if err != ErrInvalidProvider {
		t.Errorf("LinkOAuthAccount() error = %v, want %v", err, ErrInvalidProvider)
	}

	// UpdateOAuthTokens should return error for invalid provider
	err = svc.UpdateOAuthTokens(nil, models.OAuthProvider("invalid"), "123", nil, "")
	if err != ErrInvalidProvider {
		t.Errorf("UpdateOAuthTokens() error = %v, want %v", err, ErrInvalidProvider)
	}

	// ProcessOAuthLogin should return error for invalid provider
	_, err = svc.ProcessOAuthLogin(nil, models.OAuthProvider("invalid"), models.OAuthUserInfo{}, nil, "fa")
	if err != ErrInvalidProvider {
		t.Errorf("ProcessOAuthLogin() error = %v, want %v", err, ErrInvalidProvider)
	}

	// UnlinkOAuthAccount should return error for invalid provider
	err = svc.UnlinkOAuthAccount(nil, "user-1", models.OAuthProvider("invalid"))
	if err != ErrInvalidProvider {
		t.Errorf("UnlinkOAuthAccount() error = %v, want %v", err, ErrInvalidProvider)
	}

	// HasOAuthAccount should return error for invalid provider
	_, err = svc.HasOAuthAccount(nil, "user-1", models.OAuthProvider("invalid"))
	if err != ErrInvalidProvider {
		t.Errorf("HasOAuthAccount() error = %v, want %v", err, ErrInvalidProvider)
	}
}
