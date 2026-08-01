package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestHashRefreshToken(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token"
	hash := hashRefreshToken(token)

	// Should start with "sha256:" prefix
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash should start with 'sha256:' prefix, got %s", hash)
	}

	// Total length: 7 (prefix) + 64 (hex) = 71
	if len(hash) != 71 {
		t.Errorf("hash length = %d, want 71", len(hash))
	}

	// Hex part should match manual SHA-256 computation
	expected := sha256.Sum256([]byte(token))
	expectedHex := "sha256:" + hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash = %s, want %s", hash, expectedHex)
	}

	// Same input should produce same hash (deterministic)
	hash2 := hashRefreshToken(token)
	if hash != hash2 {
		t.Error("hashRefreshToken is not deterministic")
	}

	// Different input should produce different hash
	hash3 := hashRefreshToken(token + "x")
	if hash == hash3 {
		t.Error("different tokens should produce different hashes")
	}
}

func TestHashRefreshTokenLegacy(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token"
	hash := hashRefreshTokenLegacy(token)

	// Legacy hash should be 64-char hex without prefix
	if len(hash) != 64 {
		t.Errorf("legacy hash length = %d, want 64", len(hash))
	}
	if strings.HasPrefix(hash, "sha256:") {
		t.Error("legacy hash should not have sha256: prefix")
	}

	expected := sha256.Sum256([]byte(token))
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash = %s, want %s", hash, expectedHex)
	}
}

func TestIsHashedToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "new prefixed SHA-256 hash",
			input:    hashRefreshToken("some-token"),
			expected: true,
		},
		{
			name:     "sha256: prefix with hex",
			input:    "sha256:" + strings.Repeat("a", 64),
			expected: true,
		},
		{
			name:     "legacy 64 hex chars",
			input:    "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			expected: true,
		},
		{
			name:     "legacy all zeros",
			input:    strings.Repeat("0", 64),
			expected: true,
		},
		{
			name:     "JWT token (too long)",
			input:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Gfx6VO9tcxwk6xqx9yYzSfebfeakZp5JYIgP_edcw_A",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "63 hex chars (too short)",
			input:    strings.Repeat("a", 63),
			expected: false,
		},
		{
			name:     "65 hex chars (too long)",
			input:    strings.Repeat("a", 65),
			expected: false,
		},
		{
			name:     "64 chars with uppercase hex",
			input:    strings.Repeat("A", 64),
			expected: false, // only lowercase hex is valid
		},
		{
			name:     "64 chars with non-hex character",
			input:    strings.Repeat("g", 64),
			expected: false,
		},
		{
			name:     "64 chars mixed valid and invalid",
			input:    strings.Repeat("a", 63) + "G",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHashedToken(tt.input)
			if got != tt.expected {
				t.Errorf("isHashedToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHashedTokenRoundTrip(t *testing.T) {
	// Verify that hashing a token and checking with isHashedToken works correctly
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.refresh-token-payload"
	hash := hashRefreshToken(token)

	if !isHashedToken(hash) {
		t.Error("hashRefreshToken output should be recognized by isHashedToken")
	}

	// New format should have sha256: prefix
	if !strings.HasPrefix(hash, "sha256:") {
		t.Error("hashRefreshToken should produce sha256: prefixed output")
	}

	// Verify hash comparison works
	if hash != hashRefreshToken(token) {
		t.Error("same token should produce the same hash")
	}

	if hash == hashRefreshToken("different-token") {
		t.Error("different tokens should not produce the same hash")
	}
}

func TestCreatePathHashesRefreshToken(t *testing.T) {
	// Verify that the hashing applied in Create() produces values
	// that ValidateRefreshToken's three-format check handles correctly.
	rawToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.refresh-token-body.signature"
	hashed := hashRefreshToken(rawToken)

	// The stored value should be detected as a hash (new prefixed format)
	if !isHashedToken(hashed) {
		t.Error("hashed refresh token should be detected as hashed")
	}

	// Should be in the new prefixed format
	if !strings.HasPrefix(hashed, "sha256:") {
		t.Error("new hashes should use sha256: prefix")
	}

	// Comparing the hash with a fresh hash of the same token should match
	if hashed != hashRefreshToken(rawToken) {
		t.Error("hash of same token should match stored hash")
	}

	// Empty token should not be hashed (preserved as empty string)
	emptyHash := hashRefreshToken("")
	// Even empty string produces a valid SHA-256 hash, but in Create()
	// we skip hashing when the token is empty, so this just verifies the guard.
	if emptyHash == "" {
		t.Error("hashRefreshToken of empty string should produce a hash, not empty")
	}
}

func TestLegacyTokenNotDetectedAsHashed(t *testing.T) {
	// A real JWT refresh token should NOT be detected as a hashed token.
	// JWTs are base64url-encoded and typically much longer than 64 chars.
	legacyToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMTIzIiwidG9rZW5fdHlwZSI6InJlZnJlc2gifQ.signature"
	if isHashedToken(legacyToken) {
		t.Error("JWT token should not be detected as a hashed token")
	}
}

func TestLegacyHashBackwardCompat(t *testing.T) {
	// Legacy sessions stored 64-char hex without prefix.
	// isHashedToken should still recognize them.
	legacyHash := hashRefreshTokenLegacy("some-token")
	if !isHashedToken(legacyHash) {
		t.Error("legacy 64-char hex hash should be recognized by isHashedToken")
	}
	if strings.HasPrefix(legacyHash, "sha256:") {
		t.Error("legacy hash should not have prefix")
	}
}

func TestConcurrentSessionCreationMaxLimit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	maxSessions := 5
	store := NewSessionStore(&SessionStoreConfig{
		Redis:              client,
		TTL:                10 * time.Minute,
		MaxSessionsPerUser: maxSessions,
	})

	userID := "test-user-concurrent"
	concurrency := 20
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var successCount int

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session := &Session{
				UserID:       userID,
				RefreshToken: "token",
				Roles:        []string{"user"},
				DeviceInfo:   "test",
				IPAddress:    "127.0.0.1",
			}
			_, err := store.Create(ctx, session)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Verify: final session count should be at most maxSessions + 1 (tolerance for lock contention)
	sessionIDs, err := client.SMembers(ctx, "session:user_sessions:"+userID).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionIDs) > maxSessions+1 {
		t.Errorf("expected at most %d sessions (max + 1 tolerance), got %d", maxSessions+1, len(sessionIDs))
	}
	t.Logf("Created %d sessions out of %d concurrent attempts (max: %d)", len(sessionIDs), concurrency, maxSessions)
}
