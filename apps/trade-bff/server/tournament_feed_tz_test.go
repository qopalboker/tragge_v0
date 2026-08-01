package server

import (
	"strings"
	"testing"
	"time"
)

func TestTehranLocLoaded_TradeBFF(t *testing.T) {
	if tehranLoc == nil {
		t.Fatal("tehranLoc should be loaded via init()")
	}
	if tehranLoc.String() != "Asia/Tehran" {
		t.Errorf("expected Asia/Tehran, got %s", tehranLoc.String())
	}
}

func TestTehranLoc_WinterFormat(t *testing.T) {
	// January 15, 2026 12:00 UTC — winter, expect IRST (UTC+03:30)
	utc := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	formatted := utc.In(tehranLoc).Format("2006-01-02 15:04 MST")

	if !strings.Contains(formatted, "15:30") {
		t.Errorf("winter: expected 15:30 in formatted time, got %s", formatted)
	}
	if !strings.Contains(formatted, "IRST") && !strings.Contains(formatted, "+0330") {
		t.Errorf("winter: expected IRST in formatted time, got %s", formatted)
	}
}

func TestTehranLoc_SummerFormat(t *testing.T) {
	// July 15, 2026 12:00 UTC — Iran abolished DST from 2022, so summer is also IRST (UTC+03:30)
	utc := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	formatted := utc.In(tehranLoc).Format("2006-01-02 15:04 MST")

	if !strings.Contains(formatted, "15:30") {
		t.Errorf("summer: expected 15:30 in formatted time, got %s", formatted)
	}
	if !strings.Contains(formatted, "IRST") && !strings.Contains(formatted, "+0330") {
		t.Errorf("summer: expected IRST or +0330 in formatted time, got %s", formatted)
	}
}

func TestTehranLoc_OffsetWinter(t *testing.T) {
	utc := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	iran := utc.In(tehranLoc)
	_, offset := iran.Zone()
	if offset != 12600 { // 3*3600 + 30*60
		t.Errorf("winter offset: expected 12600, got %d", offset)
	}
}

func TestTehranLoc_OffsetSummer(t *testing.T) {
	// Iran abolished DST from 2022, summer offset is also IRST (UTC+03:30)
	utc := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	iran := utc.In(tehranLoc)
	_, offset := iran.Zone()
	if offset != 12600 { // 3*3600 + 30*60 (no DST since 2022)
		t.Errorf("summer offset: expected 12600, got %d", offset)
	}
}
