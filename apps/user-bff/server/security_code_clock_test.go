package server

import (
	"testing"
	"time"
)

type fixedSecurityCodeClock struct {
	now time.Time
}

func (c fixedSecurityCodeClock) Now() time.Time { return c.now }

func TestSecurityCodeClockIsControllable(t *testing.T) {
	fixture := time.Date(2026, time.July, 28, 10, 30, 0, 0, time.FixedZone("fixture", 3*60*60))
	app := &App{codeClock: fixedSecurityCodeClock{now: fixture}}
	if got := app.securityCodeNow(); !got.Equal(fixture.UTC()) || got.Location() != time.UTC {
		t.Fatalf("unexpected normalized clock value: %s", got)
	}
	if expires := app.securityCodeNow().Add(securityCodeTTL); !expires.Equal(fixture.UTC().Add(10 * time.Minute)) {
		t.Fatalf("unexpected expiry: %s", expires)
	}
}
