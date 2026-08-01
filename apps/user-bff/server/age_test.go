package server

import (
	"testing"
	"time"
)

func TestCalculateAgeAt(t *testing.T) {
	tests := []struct {
		name     string
		dob      time.Time
		now      time.Time
		expected int
	}{
		// Basic cases
		{
			name:     "exact birthday today",
			dob:      time.Date(2000, 1, 15, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: 26,
		},
		{
			name:     "birthday tomorrow",
			dob:      time.Date(2000, 1, 15, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC),
			expected: 25,
		},
		{
			name:     "birthday yesterday",
			dob:      time.Date(2000, 1, 15, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
			expected: 26,
		},

		// Leap year edge cases (the bug)
		{
			name:     "born Mar 1 leap year, now Feb 28 non-leap year",
			dob:      time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC),
			expected: 24,
		},
		{
			name:     "born Mar 1 leap year, now Mar 1 non-leap year",
			dob:      time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			expected: 25,
		},
		{
			name:     "born Feb 28 non-leap year, now Feb 28 leap year",
			dob:      time.Date(2001, 2, 28, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC),
			expected: 23,
		},
		{
			name:     "born Feb 29 leap year, now Feb 28 non-leap year",
			dob:      time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC),
			expected: 25, // Feb 29 birthday treated as Feb 28 in non-leap years
		},
		{
			name:     "born Feb 29 leap year, now Mar 1 non-leap year",
			dob:      time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			expected: 25,
		},
		{
			name:     "born Feb 29 leap year, now Feb 29 next leap year",
			dob:      time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			expected: 24,
		},
		{
			name:     "born Mar 1 non-leap year, now Feb 29 leap year",
			dob:      time.Date(2001, 3, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			expected: 22,
		},

		// Regression test for the exact reported bug scenario
		{
			name:     "regression: born Mar 1 2000 checked Mar 1 2025",
			dob:      time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			expected: 25,
		},

		// KYC age boundary (must be 18+)
		{
			name:     "exactly 18 today",
			dob:      time.Date(2008, 2, 17, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			expected: 18,
		},
		{
			name:     "17 turning 18 tomorrow",
			dob:      time.Date(2008, 2, 18, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			expected: 17,
		},

		// Year boundary
		{
			name:     "born Dec 31, now Jan 1 next year",
			dob:      time.Date(2000, 12, 31, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: 25,
		},
		{
			name:     "same day same year",
			dob:      time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateAgeAt(tt.dob, tt.now)
			if got != tt.expected {
				t.Errorf("calculateAgeAt(%s, %s) = %d, want %d",
					tt.dob.Format("2006-01-02"), tt.now.Format("2006-01-02"),
					got, tt.expected)
			}
		})
	}
}
