package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const hashPrefix = "sha256:"

// Session represents a user session stored in Redis.
type Session struct {
	ID           string       `json:"id"`
	UserID       string       `json:"user_id"`
	RefreshToken string       `json:"refresh_token"`
	Roles        []string     `json:"roles"`
	Permissions  []string     `json:"permissions,omitempty"`
	MFAAssurance MFAAssurance `json:"mfa_assurance,omitempty"`
	DeviceInfo   string       `json:"device_info"`
	IPAddress    string       `json:"ip_address"`
	CreatedAt    time.Time    `json:"created_at"`
	LastSeenAt   time.Time    `json:"last_seen_at"`
	ExpiresAt    time.Time    `json:"expires_at"`
}

var (
	// ErrSessionNotFound indicates the session does not exist or has expired.
	ErrSessionNotFound = errors.New("auth: session not found")
	// ErrSessionRevoked indicates the session has been explicitly revoked.
	ErrSessionRevoked = errors.New("auth: session has been revoked")
	// ErrRefreshTokenReuse indicates a previously used refresh token was presented,
	// suggesting a token theft/replay attack. The session is invalidated.
	ErrRefreshTokenReuse = errors.New("auth: refresh token reuse detected")
)

// SessionStore manages user sessions in Redis.
type SessionStore struct {
	redis              redis.UniversalClient
	prefix             string
	ttl                time.Duration
	maxSessionsPerUser int
}

// SessionStoreConfig holds configuration for the session store.
type SessionStoreConfig struct {
	// Redis client instance (supports standalone, sentinel, and cluster modes)
	Redis redis.UniversalClient
	// KeyPrefix for session keys. Default: "session:"
	KeyPrefix string
	// TTL for sessions. Default: 7 days (matches refresh token TTL)
	TTL time.Duration
	// MaxSessionsPerUser limits concurrent sessions. Default: 10
	MaxSessionsPerUser int
}

// NewSessionStore creates a new session store with the given configuration.
func NewSessionStore(config *SessionStoreConfig) *SessionStore {
	prefix := "session:"
	if config.KeyPrefix != "" {
		prefix = config.KeyPrefix
	}

	ttl := 7 * 24 * time.Hour
	if config.TTL > 0 {
		ttl = config.TTL
	}

	maxSessionsPerUser := 10
	if config.MaxSessionsPerUser > 0 {
		maxSessionsPerUser = config.MaxSessionsPerUser
	}

	return &SessionStore{
		redis:              config.Redis,
		prefix:             prefix,
		ttl:                ttl,
		maxSessionsPerUser: maxSessionsPerUser,
	}
}

// GenerateSessionID creates a cryptographically secure session ID.
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Create stores a new session and returns the session ID.
func (s *SessionStore) Create(ctx context.Context, session *Session) (string, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return "", err
	}

	now := time.Now()
	session.ID = sessionID
	session.CreatedAt = now
	session.LastSeenAt = now
	session.ExpiresAt = now.Add(s.ttl)

	key := s.sessionKey(sessionID)
	userKey := s.userSessionsKey(session.UserID)

	// Enforce max sessions per user with distributed lock to prevent TOCTOU race.
	if s.maxSessionsPerUser > 0 {
		lockKey := s.prefix + "lock:user:" + session.UserID
		var acquired bool
		var lockErr error

		// Try to acquire lock with exponential backoff (50ms, 100ms, 200ms)
		for attempt := 0; attempt < 3; attempt++ {
			acquired, lockErr = s.redis.SetNX(ctx, lockKey, "1", 5*time.Second).Result()
			if lockErr != nil {
				return "", fmt.Errorf("failed to acquire session creation lock: %w", lockErr)
			}
			if acquired {
				break
			}
			// Wait with exponential backoff before retry
			backoff := time.Duration(50<<uint(attempt)) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		if acquired {
			// Use background context for cleanup so lock is released even if request ctx is cancelled
			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				s.redis.Del(cleanupCtx, lockKey)
			}()
		}
		// If lock not acquired after retries, proceed without lock (graceful degradation).
		// Worst case: one extra session beyond the limit, which is not a security issue.

		sessionIDs, err := s.redis.SMembers(ctx, userKey).Result()
		if err != nil {
			return "", fmt.Errorf("failed to list user sessions for enforcement: %w", err)
		}
		if len(sessionIDs) >= s.maxSessionsPerUser {
			// Batch fetch created_at via pipeline
			type sessionWithTime struct {
				id        string
				createdAt time.Time
			}
			pipe := s.redis.Pipeline()
			getCmds := make([]*redis.StringCmd, len(sessionIDs))
			for i, sid := range sessionIDs {
				getCmds[i] = pipe.HGet(ctx, s.sessionKey(sid), "created_at")
			}
			if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
				return "", fmt.Errorf("failed to fetch session creation times: %w", err)
			}

			sessions := make([]sessionWithTime, 0, len(sessionIDs))
			for i, sid := range sessionIDs {
				createdAtStr, err := getCmds[i].Result()
				if err == nil {
					if createdAt, err := parseUnixTime(createdAtStr); err == nil {
						sessions = append(sessions, sessionWithTime{sid, createdAt})
					}
				}
			}
			// Sort by created_at (oldest first)
			sort.Slice(sessions, func(i, j int) bool {
				return sessions[i].createdAt.Before(sessions[j].createdAt)
			})
			// Delete oldest sessions to make room for new one
			toDelete := len(sessions) - s.maxSessionsPerUser + 1
			if toDelete > 0 {
				delPipe := s.redis.Pipeline()
				for i := 0; i < toDelete && i < len(sessions); i++ {
					delPipe.Del(ctx, s.sessionKey(sessions[i].id))
					delPipe.SRem(ctx, userKey, sessions[i].id)
				}
				if _, err := delPipe.Exec(ctx); err != nil {
					return "", fmt.Errorf("failed to evict oldest sessions: %w", err)
				}
			}
		}
	}

	// Store session data as hash — always hash the refresh token for security.
	refreshTokenValue := session.RefreshToken
	if refreshTokenValue != "" {
		refreshTokenValue = hashRefreshToken(refreshTokenValue)
	}

	// Use pipeline for atomic session creation
	pipe := s.redis.Pipeline()
	pipe.HSet(ctx, key, map[string]interface{}{
		"user_id":       session.UserID,
		"refresh_token": refreshTokenValue,
		"roles":         strings.Join(session.Roles, ","),
		"permissions":   strings.Join(session.Permissions, ","),
		"mfa_assurance": string(session.MFAAssurance),
		"device_info":   session.DeviceInfo,
		"ip_address":    session.IPAddress,
		"created_at":    session.CreatedAt.Unix(),
		"last_seen_at":  session.LastSeenAt.Unix(),
		"expires_at":    session.ExpiresAt.Unix(),
	})
	pipe.Expire(ctx, key, s.ttl)
	pipe.SAdd(ctx, userKey, sessionID)
	pipe.Expire(ctx, userKey, s.ttl+24*time.Hour)

	if _, err = pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	key := s.sessionKey(sessionID)

	data, err := s.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if len(data) == 0 {
		return nil, ErrSessionNotFound
	}

	session := &Session{
		ID:           sessionID,
		UserID:       data["user_id"],
		RefreshToken: data["refresh_token"],
		Roles:        splitRoles(data["roles"]),
		Permissions:  splitRoles(data["permissions"]),
		MFAAssurance: MFAAssurance(data["mfa_assurance"]),
		DeviceInfo:   data["device_info"],
		IPAddress:    data["ip_address"],
	}

	if createdAt, err := parseUnixTime(data["created_at"]); err == nil {
		session.CreatedAt = createdAt
	}
	if lastSeenAt, err := parseUnixTime(data["last_seen_at"]); err == nil {
		session.LastSeenAt = lastSeenAt
	}
	if expiresAt, err := parseUnixTime(data["expires_at"]); err == nil {
		session.ExpiresAt = expiresAt
	}

	return session, nil
}

// ValidateRefreshToken checks if a refresh token is valid for a session.
// On token mismatch the entire session is invalidated (refresh token reuse detection).
func (s *SessionStore) ValidateRefreshToken(ctx context.Context, sessionID, refreshToken string) (*Session, error) {
	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Compare tokens — support three formats for backward compatibility:
	//  1. New prefixed: "sha256:<hex>" — compare with hashRefreshToken (prefixed).
	//  2. Legacy hash: 64 lowercase hex chars — compare with hashRefreshTokenLegacy.
	//  3. Legacy plain-text: raw JWT — direct string comparison.
	match := false
	if strings.HasPrefix(session.RefreshToken, hashPrefix) {
		match = session.RefreshToken == hashRefreshToken(refreshToken)
	} else if isHashedToken(session.RefreshToken) {
		// Legacy hash stored without prefix
		match = session.RefreshToken == hashRefreshTokenLegacy(refreshToken)
	} else {
		// Backward compatibility: plain-text comparison for pre-existing sessions
		// Use constant-time comparison to prevent timing attacks
		match = subtle.ConstantTimeCompare([]byte(session.RefreshToken), []byte(refreshToken)) == 1
	}

	if !match {
		// Potential token reuse attack — invalidate the entire session
		_ = s.Delete(ctx, sessionID)
		return nil, ErrRefreshTokenReuse
	}

	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = s.Delete(ctx, sessionID)
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// Refresh updates the refresh token and extends the session.
func (s *SessionStore) Refresh(ctx context.Context, sessionID, newRefreshToken string) error {
	key := s.sessionKey(sessionID)

	exists, err := s.redis.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	newExpiresAt := time.Now().Add(s.ttl)

	err = s.redis.HSet(ctx, key, map[string]interface{}{
		"refresh_token": hashRefreshToken(newRefreshToken),
		"expires_at":    newExpiresAt.Unix(),
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if err := s.redis.Expire(ctx, key, s.ttl).Err(); err != nil {
		return fmt.Errorf("failed to extend session expiration: %w", err)
	}

	return nil
}

// Delete removes a session.
func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	key := s.sessionKey(sessionID)

	// Get user ID to remove from user's session set
	userID, err := s.redis.HGet(ctx, key, "user_id").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to get session user: %w", err)
	}

	if err := s.redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from user's session set
	if userID != "" {
		s.redis.SRem(ctx, s.userSessionsKey(userID), sessionID)
	}

	return nil
}

// DeleteAllForUser removes all sessions for a user (logout from all devices).
func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID string) error {
	userKey := s.userSessionsKey(userID)

	sessionIDs, err := s.redis.SMembers(ctx, userKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	// Batch delete all session keys + user set in one pipeline
	pipe := s.redis.Pipeline()
	for _, sessionID := range sessionIDs {
		pipe.Del(ctx, s.sessionKey(sessionID))
	}
	pipe.Del(ctx, userKey)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

// ListUserSessions returns all active session IDs for a user.
func (s *SessionStore) ListUserSessions(ctx context.Context, userID string) ([]string, error) {
	userKey := s.userSessionsKey(userID)

	sessionIDs, err := s.redis.SMembers(ctx, userKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return nil, nil
	}

	// Batch EXISTS checks via pipeline (avoids N round-trips)
	pipe := s.redis.Pipeline()
	existsCmds := make([]*redis.IntCmd, len(sessionIDs))
	for i, sessionID := range sessionIDs {
		existsCmds[i] = pipe.Exists(ctx, s.sessionKey(sessionID))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to check session existence: %w", err)
	}

	// Collect valid sessions and stale references
	validSessions := make([]string, 0, len(sessionIDs))
	var stale []interface{}
	for i, sessionID := range sessionIDs {
		if existsCmds[i].Val() > 0 {
			validSessions = append(validSessions, sessionID)
		} else {
			stale = append(stale, sessionID)
		}
	}

	// Clean up stale references in one call
	if len(stale) > 0 {
		s.redis.SRem(ctx, userKey, stale...)
	}

	return validSessions, nil
}

// GetUserSessions returns all active sessions with full details for a user.
func (s *SessionStore) GetUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	sessionIDs, err := s.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(sessionIDs) == 0 {
		return nil, nil
	}

	// Batch HGetAll via pipeline (avoids N round-trips)
	pipe := s.redis.Pipeline()
	getCmds := make([]*redis.MapStringStringCmd, len(sessionIDs))
	for i, sessionID := range sessionIDs {
		getCmds[i] = pipe.HGetAll(ctx, s.sessionKey(sessionID))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for i, sessionID := range sessionIDs {
		data := getCmds[i].Val()
		if len(data) == 0 {
			continue
		}

		session := &Session{
			ID:           sessionID,
			UserID:       data["user_id"],
			RefreshToken: data["refresh_token"],
			Roles:        splitRoles(data["roles"]),
			Permissions:  splitRoles(data["permissions"]),
			MFAAssurance: MFAAssurance(data["mfa_assurance"]),
			DeviceInfo:   data["device_info"],
			IPAddress:    data["ip_address"],
		}

		if createdAt, err := parseUnixTime(data["created_at"]); err == nil {
			session.CreatedAt = createdAt
		}
		if lastSeenAt, err := parseUnixTime(data["last_seen_at"]); err == nil {
			session.LastSeenAt = lastSeenAt
		}
		if expiresAt, err := parseUnixTime(data["expires_at"]); err == nil {
			session.ExpiresAt = expiresAt
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// UpdateLastSeen updates the last_seen_at timestamp for a session.
func (s *SessionStore) UpdateLastSeen(ctx context.Context, sessionID string) error {
	key := s.sessionKey(sessionID)

	exists, err := s.redis.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	now := time.Now()
	err = s.redis.HSet(ctx, key, "last_seen_at", now.Unix()).Err()
	if err != nil {
		return fmt.Errorf("failed to update last_seen_at: %w", err)
	}

	return nil
}

// DeleteAllExcept removes all sessions for a user except the specified session ID.
func (s *SessionStore) DeleteAllExcept(ctx context.Context, userID, exceptSessionID string) error {
	userKey := s.userSessionsKey(userID)

	sessionIDs, err := s.redis.SMembers(ctx, userKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Batch delete + remove from set via pipeline
	pipe := s.redis.Pipeline()
	for _, sessionID := range sessionIDs {
		if sessionID != exceptSessionID {
			pipe.Del(ctx, s.sessionKey(sessionID))
			pipe.SRem(ctx, userKey, sessionID)
		}
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete other sessions: %w", err)
	}

	return nil
}

func (s *SessionStore) sessionKey(sessionID string) string {
	return s.prefix + sessionID
}

func (s *SessionStore) userSessionsKey(userID string) string {
	return s.prefix + "user:" + userID
}

func joinRoles(roles []string) string {
	return strings.Join(roles, ",")
}

func splitRoles(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// hashRefreshToken returns a prefixed SHA-256 hex digest of a refresh token.
// The "sha256:" prefix makes hashed tokens unambiguously distinguishable from
// plain-text JWTs, avoiding false positives when a JWT happens to be exactly
// 64 lowercase hex characters.
func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hashPrefix + hex.EncodeToString(h[:])
}

// hashRefreshTokenLegacy returns the raw hex-encoded SHA-256 digest without prefix.
// Used only for comparing against sessions stored before the prefix migration.
func hashRefreshTokenLegacy(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// isHashedToken returns true if s looks like a hashed refresh token.
// Recognises two formats:
//  1. New: "sha256:" prefix followed by 64 hex chars (unambiguous).
//  2. Legacy: exactly 64 lowercase hex chars (backward compat for existing sessions).
func isHashedToken(s string) bool {
	if strings.HasPrefix(s, hashPrefix) {
		return true
	}
	// Legacy format: 64-char lowercase hex
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func parseUnixTime(s string) (time.Time, error) {
	var unix int64
	_, err := fmt.Sscanf(s, "%d", &unix)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(unix, 0), nil
}
