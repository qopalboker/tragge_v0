package auth

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const blacklistPrefix = "jwt_blacklist:"

// TokenBlacklist manages blacklisted JWT access tokens using Redis.
// Tokens are stored with TTL matching their remaining lifetime, keeping the set small.
type TokenBlacklist struct {
	redis  redis.UniversalClient
	prefix string
}

// NewTokenBlacklist creates a legacy unqualified token blacklist.
func NewTokenBlacklist(redisClient redis.UniversalClient) *TokenBlacklist {
	return NewTokenBlacklistWithPrefix(redisClient, blacklistPrefix)
}

// NewTokenBlacklistWithPrefix creates a revocation store in an explicit
// authentication context namespace.
func NewTokenBlacklistWithPrefix(redisClient redis.UniversalClient, prefix string) *TokenBlacklist {
	if prefix == "" {
		prefix = blacklistPrefix
	}
	return &TokenBlacklist{redis: redisClient, prefix: prefix}
}

// Add blacklists a JWT by its JTI (JWT ID) with TTL matching the remaining token lifetime.
func (b *TokenBlacklist) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return nil // Token already expired, no need to blacklist
	}
	return b.redis.Set(ctx, b.prefix+jti, "1", remaining).Err()
}

// IsBlacklisted checks if a JWT ID is in the blacklist.
// Returns false if Redis is unavailable (graceful degradation — token will expire naturally).
func (b *TokenBlacklist) IsBlacklisted(ctx context.Context, jti string) bool {
	result, err := b.redis.Exists(ctx, b.prefix+jti).Result()
	if err != nil {
		// Graceful degradation: if Redis is unavailable, skip blacklist check.
		// The token will expire naturally within ≤15 minutes.
		log.Printf("[WARN] Redis unavailable, token blacklist check skipped: %v", err)
		return false
	}
	return result > 0
}
