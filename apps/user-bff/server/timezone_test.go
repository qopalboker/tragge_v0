package server

import (
	"strings"
	"testing"
	"time"
)

func TestTehranLocLoaded(t *testing.T) {
	if tehranLoc == nil {
		t.Fatal("tehranLoc should be loaded via init()")
	}
	if tehranLoc.String() != "Asia/Tehran" {
		t.Errorf("expected Asia/Tehran, got %s", tehranLoc.String())
	}
}

func TestToIRST_Winter(t *testing.T) {
	// January 15, 2026 12:00 UTC — winter, expect IRST (UTC+03:30)
	utc := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	irst := toIRST(utc)

	if irst.Hour() != 15 || irst.Minute() != 30 {
		t.Errorf("winter: expected 15:30, got %02d:%02d", irst.Hour(), irst.Minute())
	}
	zone, offset := irst.Zone()
	if offset != 3*3600+30*60 {
		t.Errorf("winter: expected offset 12600, got %d", offset)
	}
	if zone != "IRST" && zone != "+0330" {
		t.Errorf("winter: expected zone IRST, got %s", zone)
	}
}

func TestToIRST_Summer(t *testing.T) {
	// July 15, 2026 12:00 UTC — Iran abolished DST from 2022, so summer is also IRST (UTC+03:30)
	utc := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	irst := toIRST(utc)

	if irst.Hour() != 15 || irst.Minute() != 30 {
		t.Errorf("summer: expected 15:30, got %02d:%02d", irst.Hour(), irst.Minute())
	}
	zone, offset := irst.Zone()
	if offset != 3*3600+30*60 {
		t.Errorf("summer: expected offset 12600, got %d", offset)
	}
	if zone != "IRST" && zone != "+0330" {
		t.Errorf("summer: expected zone IRST or +0330, got %s", zone)
	}
}

func TestNewDualTime_WinterLabel(t *testing.T) {
	// Winter: label should contain IRST (not IRDT)
	utc := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	dt := newDualTime(utc)

	if !strings.Contains(dt.IRST, "IRST") && !strings.Contains(dt.IRST, "+0330") {
		t.Errorf("winter DualTime IRST field should contain IRST, got: %s", dt.IRST)
	}
	if strings.Contains(dt.IRST, "IRDT") {
		t.Errorf("winter DualTime should not contain IRDT, got: %s", dt.IRST)
	}
}

func TestNewDualTime_SummerLabel(t *testing.T) {
	// Summer 2026: Iran abolished DST from 2022, so summer is also IRST (UTC+03:30)
	utc := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	dt := newDualTime(utc)

	if !strings.Contains(dt.IRST, "IRST") && !strings.Contains(dt.IRST, "+0330") {
		t.Errorf("summer DualTime IRST field should contain IRST or +0330, got: %s", dt.IRST)
	}
}

func TestIsWeekendIRST(t *testing.T) {
	// Friday 2026-01-16 is a Friday in IRST
	friday := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)
	irstFriday := toIRST(friday)
	if irstFriday.Weekday() != time.Friday {
		t.Skipf("test date is not Friday in IRST, got %s", irstFriday.Weekday())
	}
	if !isWeekendIRST(friday) {
		t.Error("Friday should be weekend in Iran")
	}

	// Saturday
	saturday := friday.Add(24 * time.Hour)
	if !isWeekendIRST(saturday) {
		t.Error("Saturday should be weekend in Iran")
	}

	// Sunday (workday in Iran)
	sunday := saturday.Add(24 * time.Hour)
	if isWeekendIRST(sunday) {
		t.Error("Sunday should not be weekend in Iran")
	}
}

func TestIsWeekendIRST_DSTBoundary(t *testing.T) {
	// Test near DST boundary — late Thursday UTC could be Friday in IRST/IRDT
	// March 21, 2026 (approximate DST start) — 20:30 UTC = 01:00 Friday IRDT
	lateThursday := time.Date(2026, 3, 20, 20, 30, 0, 0, time.UTC) // Friday in Tehran
	iranTime := toIRST(lateThursday)
	if iranTime.Weekday() == time.Friday {
		if !isWeekendIRST(lateThursday) {
			t.Error("should be weekend when it's Friday in Tehran timezone")
		}
	}
}

func TestJalaliWeekday(t *testing.T) {
	// Saturday 2026-01-17 in IRST
	sat := time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC)
	irstSat := toIRST(sat)
	if irstSat.Weekday() != time.Saturday {
		t.Skipf("test date is not Saturday in IRST, got %s", irstSat.Weekday())
	}
	name := jalaliWeekday(sat)
	if name != "شنبه" {
		t.Errorf("expected شنبه for Saturday, got %s", name)
	}
}

func TestGregorianToJalali(t *testing.T) {
	tests := []struct {
		gy, gm, gd int
		jy, jm, jd int
	}{
		{2026, 3, 21, 1405, 1, 1},   // Nowruz
		{2026, 1, 15, 1404, 10, 25}, // Winter date
		{2026, 7, 15, 1405, 4, 24},  // Summer date
	}
	for _, tt := range tests {
		jy, jm, jd := gregorianToJalali(tt.gy, tt.gm, tt.gd)
		if jy != tt.jy || jm != tt.jm || jd != tt.jd {
			t.Errorf("gregorianToJalali(%d, %d, %d) = %d/%d/%d, want %d/%d/%d",
				tt.gy, tt.gm, tt.gd, jy, jm, jd, tt.jy, tt.jm, tt.jd)
		}
	}
}
