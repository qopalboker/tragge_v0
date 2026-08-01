package server

import (
	"context"
	"strconv"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// Step 6 split user and admin sessions into two Redis namespaces. An
// account that holds both roles can be logged into both panels at once,
// so anything admin-bff does to "a user's sessions" (list, invalidate,
// terminate) must cover both namespaces — missing one leaves a dangling
// session the admin thought they killed.
const (
	sessionPrefixUser  = auth.UserSessionPrefix
	sessionPrefixAdmin = auth.AdminSessionPrefix
)

// sessionNamespaces is the authoritative list of Redis key prefixes any
// cross-panel session sweep must iterate over. Keep in sync with the
// SessionPrefix values in user-bff and admin-bff's Auth config.
var sessionNamespaces = []string{sessionPrefixUser, sessionPrefixAdmin}

// userSessionInfo mirrors handlers_user_management.UserSessionInfo but
// lives here so this file doesn't take a dep on the handler type. The
// handler converts.
type adminSessionSnapshot struct {
	Prefix     string
	SessionID  string
	DeviceInfo string
	IPAddress  string
	LastSeenAt time.Time
}

// listUserSessionsAllNamespaces returns every active session Redis knows
// about for a given userID across both panel namespaces. Errors on
// individual reads are swallowed on purpose — this feeds a display, not
// an auth decision, and a partial list beats failing the whole request
// when Redis is degraded.
func (a *App) listUserSessionsAllNamespaces(ctx context.Context, userID string) []adminSessionSnapshot {
	out := []adminSessionSnapshot{}
	if a.redis == nil {
		return out
	}
	for _, prefix := range sessionNamespaces {
		var sessionIDs []string
		_ = a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
			var err error
			sessionIDs, err = a.redis.Client().SMembers(ctx, prefix+"user:"+userID).Result()
			return err
		})
		for _, sid := range sessionIDs {
			var data map[string]string
			_ = a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
				var err error
				data, err = a.redis.Client().HGetAll(ctx, prefix+sid).Result()
				return err
			})
			if len(data) == 0 {
				continue
			}
			snap := adminSessionSnapshot{
				Prefix:     prefix,
				SessionID:  sid,
				DeviceInfo: data["device_info"],
				IPAddress:  data["ip_address"],
			}
			if lastSeen, err := strconv.ParseInt(data["last_seen_at"], 10, 64); err == nil {
				snap.LastSeenAt = time.Unix(lastSeen, 0)
			}
			out = append(out, snap)
		}
	}
	return out
}

// invalidateAllUserSessionsAllNamespaces deletes every session key
// belonging to userID across both panel namespaces. Returns the total
// number of session records deleted. Fire-and-forget: errors are
// swallowed so a flaky Redis doesn't abort the enclosing request (the
// primary state change — role update, ban, etc. — has already been
// committed to Postgres).
func (a *App) invalidateAllUserSessionsAllNamespaces(ctx context.Context, userID string) int {
	if a.redis == nil {
		return 0
	}
	deleted := 0
	for _, prefix := range sessionNamespaces {
		var sessionIDs []string
		_ = a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
			var err error
			sessionIDs, err = a.redis.Client().SMembers(ctx, prefix+"user:"+userID).Result()
			return err
		})
		for _, sid := range sessionIDs {
			_ = a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
				return a.redis.Client().Del(ctx, prefix+sid).Err()
			})
			deleted++
		}
		if len(sessionIDs) > 0 {
			_ = a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
				return a.redis.Client().Del(ctx, prefix+"user:"+userID).Err()
			})
		}
	}
	return deleted
}
