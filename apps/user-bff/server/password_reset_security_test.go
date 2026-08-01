package server

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordResetUsesCanonicalSecurityCodePolicy(t *testing.T) {
	if securityCodeTTL != 10*time.Minute {
		t.Fatalf("password-reset TTL is %s", securityCodeTTL)
	}
	if securityCodeCooldown != 60*time.Second {
		t.Fatalf("password-reset cooldown is %s", securityCodeCooldown)
	}
	if securityCodeMaxAttempts != 5 {
		t.Fatalf("password-reset maximum attempts is %d", securityCodeMaxAttempts)
	}
	if passwordSetTokenTTL != securityCodeTTL {
		t.Fatal("password-set token and reset-code lifetimes diverged")
	}
}

func TestPasswordResetRedisKeysRemainInUserContext(t *testing.T) {
	for _, key := range []string{
		pwResetSessionKey("fixture"),
		pwResetSetTokenKey("fixture"),
		pwResetDailyKey("fixture-user"),
		pwResetIPKey("192.0.2.1"),
	} {
		if !strings.HasPrefix(key, "auth:user:password-reset:") {
			t.Fatalf("reset state escaped the SEC-001 User namespace: %q", key)
		}
		if strings.Contains(key, "admin") {
			t.Fatalf("reset state collided with Admin context: %q", key)
		}
	}
}
