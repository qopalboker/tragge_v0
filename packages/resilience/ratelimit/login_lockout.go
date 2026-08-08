package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// LockoutConfig defines distributed progressive login lockout.
type LockoutConfig struct {
	Namespace string
	Threshold int
	LockFor   time.Duration
	Retention time.Duration
}

// LoginLockout stores only digests of normalized IP/account identities.
type LoginLockout struct {
	client redis.UniversalClient
	config LockoutConfig
}

func NewLoginLockout(client redis.UniversalClient, config LockoutConfig) (*LoginLockout, error) {
	if client == nil {
		return nil, ErrRedisUnavailable
	}
	if config.Namespace == "" || config.Threshold < 2 || config.LockFor <= 0 || config.Retention < config.LockFor {
		return nil, errors.New("invalid login lockout configuration")
	}
	return &LoginLockout{client: client, config: config}, nil
}

func (l *LoginLockout) keys(identity string) (string, string) {
	digest := digestKey(identity)
	base := "sec006:lockout:" + l.config.Namespace + ":" + digest
	return base + ":attempts", base + ":locked"
}

var loginFailureScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
if count >= tonumber(ARGV[2]) then
  redis.call('SET', KEYS[2], '1', 'PX', ARGV[3])
  return ARGV[3]
end
return 0
`)

// Check fails closed on storage errors.
func (l *LoginLockout) Check(ctx context.Context, identities ...string) (bool, time.Duration, error) {
	var retry time.Duration
	for _, identity := range identities {
		_, lockKey := l.keys(identity)
		remaining, err := l.client.PTTL(ctx, lockKey).Result()
		if err != nil {
			return false, 0, err
		}
		if remaining > retry {
			retry = remaining
		}
	}
	return retry <= 0, retry, nil
}

// Failure increments every supplied dimension and returns the longest lock.
func (l *LoginLockout) Failure(ctx context.Context, identities ...string) (time.Duration, error) {
	var retry time.Duration
	for _, identity := range identities {
		attemptKey, lockKey := l.keys(identity)
		ms, err := loginFailureScript.Run(ctx, l.client, []string{attemptKey, lockKey},
			l.config.Retention.Milliseconds(), l.config.Threshold, l.config.LockFor.Milliseconds()).Int64()
		if err != nil {
			return 0, err
		}
		if candidate := time.Duration(ms) * time.Millisecond; candidate > retry {
			retry = candidate
		}
	}
	return retry, nil
}

func (l *LoginLockout) Success(ctx context.Context, identities ...string) error {
	keys := make([]string, 0, len(identities)*2)
	for _, identity := range identities {
		attemptKey, lockKey := l.keys(identity)
		keys = append(keys, attemptKey, lockKey)
	}
	if len(keys) == 0 {
		return nil
	}
	return l.client.Del(ctx, keys...).Err()
}
