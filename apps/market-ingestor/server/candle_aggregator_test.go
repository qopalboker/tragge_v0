package server

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestGetCandleStartTime_Resolution4h(t *testing.T) {
	ca := &CandleAggregator{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name      string
		input     time.Time
		wantHour  int
		wantMin   int
	}{
		{
			name:     "10:37 UTC should align to 08:00 (08-12 block)",
			input:    time.Date(2026, 3, 26, 10, 37, 0, 0, time.UTC),
			wantHour: 8,
			wantMin:  0,
		},
		{
			name:     "00:15 UTC should align to 00:00 (00-04 block)",
			input:    time.Date(2026, 3, 26, 0, 15, 0, 0, time.UTC),
			wantHour: 0,
			wantMin:  0,
		},
		{
			name:     "04:00 UTC should align to 04:00 (04-08 block)",
			input:    time.Date(2026, 3, 26, 4, 0, 0, 0, time.UTC),
			wantHour: 4,
			wantMin:  0,
		},
		{
			name:     "07:59 UTC should align to 04:00 (04-08 block)",
			input:    time.Date(2026, 3, 26, 7, 59, 59, 0, time.UTC),
			wantHour: 4,
			wantMin:  0,
		},
		{
			name:     "12:00 UTC should align to 12:00 (12-16 block)",
			input:    time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC),
			wantHour: 12,
			wantMin:  0,
		},
		{
			name:     "23:30 UTC should align to 20:00 (20-24 block)",
			input:    time.Date(2026, 3, 26, 23, 30, 0, 0, time.UTC),
			wantHour: 20,
			wantMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := tt.input.Unix()
			got := ca.getCandleStartTime(timestamp, Resolution4h)
			gotTime := time.Unix(got, 0).UTC()

			if gotTime.Hour() != tt.wantHour || gotTime.Minute() != tt.wantMin {
				t.Errorf("getCandleStartTime(%v, 4h) = %v (hour=%d, min=%d), want hour=%d, min=%d",
					tt.input, gotTime, gotTime.Hour(), gotTime.Minute(), tt.wantHour, tt.wantMin)
			}
			// Verify same date
			if gotTime.Day() != tt.input.Day() || gotTime.Month() != tt.input.Month() {
				t.Errorf("getCandleStartTime changed the date: got %v, input was %v", gotTime, tt.input)
			}
		})
	}
}

func TestGetCandleStartTime_AllResolutions(t *testing.T) {
	ca := &CandleAggregator{
		logger: zap.NewNop(),
	}

	// Test at 10:37:42 UTC
	input := time.Date(2026, 3, 26, 10, 37, 42, 0, time.UTC)
	timestamp := input.Unix()

	tests := []struct {
		resolution Resolution
		wantHour   int
		wantMin    int
	}{
		{Resolution1m, 10, 37},
		{Resolution5m, 10, 35},
		{Resolution15m, 10, 30},
		{Resolution30m, 10, 30},
		{Resolution1h, 10, 0},
		{Resolution4h, 8, 0},
		{Resolution1d, 0, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.resolution), func(t *testing.T) {
			got := ca.getCandleStartTime(timestamp, tt.resolution)
			gotTime := time.Unix(got, 0).UTC()

			if gotTime.Hour() != tt.wantHour || gotTime.Minute() != tt.wantMin {
				t.Errorf("getCandleStartTime(%v, %s) = hour=%d min=%d, want hour=%d min=%d",
					input, tt.resolution, gotTime.Hour(), gotTime.Minute(), tt.wantHour, tt.wantMin)
			}
		})
	}
}
