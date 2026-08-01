package traggepoint

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// testNow is a fixed reference time for deterministic tests.
var testNow = time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)

func TestCalculateParticipantMultiplier(t *testing.T) {
	tests := []struct {
		name             string
		participants     int
		expectedApprox   float64
		expectedMin      float64
		expectedMax      float64
		expectExactClamp bool
	}{
		{
			name:             "10 participants",
			participants:     10,
			expectedApprox:   0.333,
			expectedMin:      0.3,
			expectedMax:      0.4,
			expectExactClamp: false,
		},
		{
			name:             "100 participants",
			participants:     100,
			expectedApprox:   0.667,
			expectedMin:      0.6,
			expectedMax:      0.7,
			expectExactClamp: false,
		},
		{
			name:             "1000 participants",
			participants:     1000,
			expectedApprox:   1.0,
			expectedMin:      0.99,
			expectedMax:      1.01,
			expectExactClamp: false,
		},
		{
			name:             "10000 participants",
			participants:     10000,
			expectedApprox:   1.333,
			expectedMin:      1.3,
			expectedMax:      1.4,
			expectExactClamp: false,
		},
		{
			name:             "100000 participants - clamped to max",
			participants:     100000,
			expectedApprox:   1.5,
			expectedMin:      1.5,
			expectedMax:      1.5,
			expectExactClamp: true,
		},
		{
			name:             "1 participant - clamped to min",
			participants:     1,
			expectedApprox:   0.1,
			expectedMin:      0.1,
			expectedMax:      0.1,
			expectExactClamp: true,
		},
		{
			name:             "0 participants",
			participants:     0,
			expectedApprox:   0.1,
			expectedMin:      0.1,
			expectedMax:      0.1,
			expectExactClamp: true,
		},
		{
			name:             "negative participants",
			participants:     -10,
			expectedApprox:   0.1,
			expectedMin:      0.1,
			expectedMax:      0.1,
			expectExactClamp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateParticipantMultiplier(tt.participants)
			f := ToFloat64(result)

			if tt.expectExactClamp {
				expected := decimal.NewFromFloat(tt.expectedApprox)
				if !result.Equal(expected) {
					t.Errorf("CalculateParticipantMultiplier(%d) = %s, want %s", tt.participants, result.String(), expected.String())
				}
			} else {
				if f < tt.expectedMin || f > tt.expectedMax {
					t.Errorf("CalculateParticipantMultiplier(%d) = %f, want between %f and %f", tt.participants, f, tt.expectedMin, tt.expectedMax)
				}
			}
		})
	}
}

func TestCalculateRankBonus(t *testing.T) {
	tests := []struct {
		name        string
		rank        int
		total       int
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "#1 in 100",
			rank:        1,
			total:       100,
			expectedMin: 1.49,
			expectedMax: 1.5,
		},
		{
			name:        "#10 in 100",
			rank:        10,
			total:       100,
			expectedMin: 1.44,
			expectedMax: 1.46,
		},
		{
			name:        "#50 in 100",
			rank:        50,
			total:       100,
			expectedMin: 1.24,
			expectedMax: 1.26,
		},
		{
			name:        "#100 in 100 (last place)",
			rank:        100,
			total:       100,
			expectedMin: 1.0,
			expectedMax: 1.0,
		},
		{
			name:        "#1 in 10",
			rank:        1,
			total:       10,
			expectedMin: 1.44,
			expectedMax: 1.46,
		},
		{
			name:        "#1 in 1000",
			rank:        1,
			total:       1000,
			expectedMin: 1.499,
			expectedMax: 1.5,
		},
		{
			name:        "Invalid rank 0",
			rank:        0,
			total:       100,
			expectedMin: 1.0,
			expectedMax: 1.0,
		},
		{
			name:        "Invalid total 0",
			rank:        1,
			total:       0,
			expectedMin: 1.0,
			expectedMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateRankBonus(tt.rank, tt.total)
			f := ToFloat64(result)

			if f < tt.expectedMin || f > tt.expectedMax {
				t.Errorf("CalculateRankBonus(%d, %d) = %f, want between %f and %f", tt.rank, tt.total, f, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestCalculateContribution(t *testing.T) {
	calc := NewCalculator()

	tests := []struct {
		name              string
		tournamentScore   float64
		rank              int
		totalParticipants int
		expectedMin       float64
		expectedMax       float64
	}{
		{
			name:              "Win 1000-person tournament with score 100",
			tournamentScore:   100,
			rank:              1,
			totalParticipants: 1000,
			// 100 * 1.0 * 1.5 = 150
			expectedMin: 149,
			expectedMax: 151,
		},
		{
			name:              "Win 10-person tournament with score 100",
			tournamentScore:   100,
			rank:              1,
			totalParticipants: 10,
			// 100 * 0.33 * 1.45 = ~48
			expectedMin: 45,
			expectedMax: 52,
		},
		{
			name:              "Last place in 100-person tournament with score 50",
			tournamentScore:   50,
			rank:              100,
			totalParticipants: 100,
			// 50 * 0.67 * 1.0 = ~33.5
			expectedMin: 30,
			expectedMax: 37,
		},
		{
			name:              "Negative score",
			tournamentScore:   -100,
			rank:              1,
			totalParticipants: 100,
			expectedMin:       0,
			expectedMax:       0,
		},
		{
			name:              "Zero score",
			tournamentScore:   0,
			rank:              1,
			totalParticipants: 100,
			expectedMin:       0,
			expectedMax:       0,
		},
		{
			name:              "Invalid rank",
			tournamentScore:   100,
			rank:              0,
			totalParticipants: 100,
			expectedMin:       0,
			expectedMax:       0,
		},
		{
			name:              "Invalid participants",
			tournamentScore:   100,
			rank:              1,
			totalParticipants: 0,
			expectedMin:       0,
			expectedMax:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateContribution(ContestResult{
				TournamentScore:   decimal.NewFromFloat(tt.tournamentScore),
				Rank:              tt.rank,
				TotalParticipants: tt.totalParticipants,
				CompletedAt:       testNow,
			}, testNow)

			f := ToFloat64(result)
			if f < tt.expectedMin || f > tt.expectedMax {
				t.Errorf("CalculateContribution(%f, %d, %d) = %f, want between %f and %f",
					tt.tournamentScore, tt.rank, tt.totalParticipants, f, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestCalculateContributionWithBreakdown(t *testing.T) {
	calc := NewCalculator()

	breakdown := calc.CalculateContributionWithBreakdown(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 1000,
		CompletedAt:       testNow,
	}, testNow)

	if !breakdown.BaseScore.Equal(decimal.NewFromFloat(100)) {
		t.Errorf("BaseScore = %s, want 100", breakdown.BaseScore.String())
	}

	// Participant multiplier for 1000 should be 1.0
	pmf := ToFloat64(breakdown.ParticipantMultiplier)
	if pmf < 0.99 || pmf > 1.01 {
		t.Errorf("ParticipantMultiplier = %f, want ~1.0", pmf)
	}

	// Rank bonus for #1 in 1000 should be close to 1.5
	rbf := ToFloat64(breakdown.RankBonus)
	if rbf < 1.49 || rbf > 1.5 {
		t.Errorf("RankBonus = %f, want ~1.5", rbf)
	}

	// Decay factor for same-time should be 1.0
	if !breakdown.DecayFactor.Equal(decOne) {
		t.Errorf("DecayFactor = %s, want 1.0", breakdown.DecayFactor.String())
	}

	// Final contribution should be ~150
	fcf := ToFloat64(breakdown.FinalContribution)
	if fcf < 149 || fcf > 151 {
		t.Errorf("FinalContribution = %f, want ~150", fcf)
	}
}

func TestCalculator_CalculateTotalScore(t *testing.T) {
	calc := NewCalculator()

	results := []ContestResult{
		{TournamentScore: decimal.NewFromFloat(100), Rank: 1, TotalParticipants: 1000, CompletedAt: testNow}, // ~150
		{TournamentScore: decimal.NewFromFloat(50), Rank: 10, TotalParticipants: 100, CompletedAt: testNow},  // ~47
		{TournamentScore: decimal.NewFromFloat(25), Rank: 5, TotalParticipants: 10, CompletedAt: testNow},    // ~15
	}

	total := calc.CalculateTotalScore(results, testNow)
	f := ToFloat64(total)

	// Rough expectation: ~150 + ~47 + ~15 = ~212
	if f < 180 || f > 250 {
		t.Errorf("CalculateTotalScore = %f, want between 180 and 250", f)
	}
}

func TestCalculateWinRate(t *testing.T) {
	tests := []struct {
		name     string
		wins     int
		total    int
		expected float64
	}{
		{
			name:     "5 wins out of 10",
			wins:     5,
			total:    10,
			expected: 50.0,
		},
		{
			name:     "10 wins out of 10",
			wins:     10,
			total:    10,
			expected: 100.0,
		},
		{
			name:     "0 wins out of 10",
			wins:     0,
			total:    10,
			expected: 0.0,
		},
		{
			name:     "0 total contests",
			wins:     0,
			total:    0,
			expected: 0.0,
		},
		{
			name:     "3 wins out of 45",
			wins:     3,
			total:    45,
			expected: 6.66666667,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateWinRate(tt.wins, tt.total)
			f := ToFloat64(result)

			diff := f - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("CalculateWinRate(%d, %d) = %f, want %f", tt.wins, tt.total, f, tt.expected)
			}
		})
	}
}

func TestScoreComparison(t *testing.T) {
	// Verify that winning a 1000-person tournament is worth more than
	// winning a 10-person tournament with the same score
	calc := NewCalculator()

	largeTournament := calc.CalculateContribution(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 1000,
		CompletedAt:       testNow,
	}, testNow)
	smallTournament := calc.CalculateContribution(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 10,
		CompletedAt:       testNow,
	}, testNow)

	if largeTournament.LessThanOrEqual(smallTournament) {
		t.Errorf("Large tournament contribution (%s) should be greater than small tournament (%s)",
			largeTournament.String(), smallTournament.String())
	}

	// The ratio should be approximately 3x (1.0/0.33)
	ratio := ToFloat64(largeTournament) / ToFloat64(smallTournament)
	if ratio < 2.0 || ratio > 4.0 {
		t.Errorf("Ratio between large and small tournament = %f, expected between 2.0 and 4.0", ratio)
	}
}

func TestRankImpact(t *testing.T) {
	// Verify that higher ranks get better contributions
	calc := NewCalculator()

	rank1 := calc.CalculateContribution(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 100,
		CompletedAt:       testNow,
	}, testNow)
	rank50 := calc.CalculateContribution(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              50,
		TotalParticipants: 100,
		CompletedAt:       testNow,
	}, testNow)
	rank100 := calc.CalculateContribution(ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              100,
		TotalParticipants: 100,
		CompletedAt:       testNow,
	}, testNow)

	if rank1.LessThanOrEqual(rank50) {
		t.Errorf("Rank 1 contribution (%s) should be greater than rank 50 (%s)", rank1.String(), rank50.String())
	}

	if rank50.LessThanOrEqual(rank100) {
		t.Errorf("Rank 50 contribution (%s) should be greater than rank 100 (%s)", rank50.String(), rank100.String())
	}
}

// --- Decay-specific tests ---

func TestDecayFactor(t *testing.T) {
	calc := NewCalculator() // 180-day half-life default

	tests := []struct {
		name        string
		daysAgo     int
		expectedMin float64
		expectedMax float64
	}{
		{"just completed", 0, 1.0, 1.0},
		{"90 days ago", 90, 0.70, 0.72},   // 0.5^(90/180) = 0.5^0.5 ≈ 0.707
		{"180 days ago", 180, 0.49, 0.51},  // 0.5^1 = 0.5
		{"360 days ago", 360, 0.24, 0.26},  // 0.5^2 = 0.25
		{"1 year ago", 365, 0.23, 0.26},    // slightly less than 0.25
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completedAt := testNow.AddDate(0, 0, -tt.daysAgo)
			factor := calc.CalculateDecayFactor(completedAt, testNow)
			f := ToFloat64(factor)
			if f < tt.expectedMin || f > tt.expectedMax {
				t.Errorf("decay factor for %d days ago = %f, want [%f, %f]",
					tt.daysAgo, f, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestDecayFactorFutureTime(t *testing.T) {
	calc := NewCalculator()
	completedAt := testNow.Add(24 * time.Hour) // future
	factor := calc.CalculateDecayFactor(completedAt, testNow)
	if !factor.Equal(decOne) {
		t.Errorf("decay factor for future completion should be 1.0, got %s", factor.String())
	}
}

func TestDecayFactorZeroTime(t *testing.T) {
	calc := NewCalculator()
	factor := calc.CalculateDecayFactor(time.Time{}, testNow)
	if !factor.Equal(decOne) {
		t.Errorf("decay factor for zero time should be 1.0, got %s", factor.String())
	}
}

func TestCustomHalfLife(t *testing.T) {
	calc := NewCalculator(WithDecayHalfLife(90)) // 90-day half-life
	completedAt := testNow.AddDate(0, 0, -90)
	factor := calc.CalculateDecayFactor(completedAt, testNow)
	f := ToFloat64(factor)
	// At exactly 1 half-life, factor should be 0.5
	if f < 0.49 || f > 0.51 {
		t.Errorf("decay factor at 1 half-life = %f, want ~0.5", f)
	}
}

func TestDecayImpactOnContribution(t *testing.T) {
	calc := NewCalculator()

	recentResult := ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 1000,
		CompletedAt:       testNow,
	}
	oldResult := ContestResult{
		TournamentScore:   decimal.NewFromFloat(100),
		Rank:              1,
		TotalParticipants: 1000,
		CompletedAt:       testNow.AddDate(0, 0, -180),
	}

	recent := calc.CalculateContribution(recentResult, testNow)
	old := calc.CalculateContribution(oldResult, testNow)

	// Old result should be approximately half of recent (180 days = 1 half-life)
	ratio := ToFloat64(old) / ToFloat64(recent)
	if ratio < 0.45 || ratio > 0.55 {
		t.Errorf("old/recent ratio = %f, want ~0.5", ratio)
	}
}

func TestDecayTotalScoreDecreases(t *testing.T) {
	calc := NewCalculator()

	// Same results but evaluated at different "now" times
	results := []ContestResult{
		{
			TournamentScore:   decimal.NewFromFloat(100),
			Rank:              1,
			TotalParticipants: 1000,
			CompletedAt:       testNow,
		},
	}

	scoreAtCompletion := calc.CalculateTotalScore(results, testNow)
	score6MonthsLater := calc.CalculateTotalScore(results, testNow.AddDate(0, 6, 0))
	score1YearLater := calc.CalculateTotalScore(results, testNow.AddDate(1, 0, 0))

	if scoreAtCompletion.LessThanOrEqual(score6MonthsLater) {
		t.Errorf("Score at completion (%s) should be greater than 6 months later (%s)",
			scoreAtCompletion.String(), score6MonthsLater.String())
	}
	if score6MonthsLater.LessThanOrEqual(score1YearLater) {
		t.Errorf("Score 6 months later (%s) should be greater than 1 year later (%s)",
			score6MonthsLater.String(), score1YearLater.String())
	}
}

func TestDecimalPrecisionAccumulation(t *testing.T) {
	// Verify that decimal precision prevents accumulation errors
	// that would occur with float64 over many contests
	calc := NewCalculator()

	var results []ContestResult
	for i := 0; i < 10000; i++ {
		results = append(results, ContestResult{
			TournamentScore:   decimal.NewFromFloat(0.01),
			Rank:              1,
			TotalParticipants: 100,
			CompletedAt:       testNow,
		})
	}

	total := calc.CalculateTotalScore(results, testNow)

	// Each: 0.01 * ~0.667 * ~1.495 = ~0.00997
	// 10000 * ~0.00997 = ~99.7
	// Exact value depends on multiplier/bonus precision, but should be close
	f := ToFloat64(total)
	if f < 90 || f > 110 {
		t.Errorf("Accumulated total over 10000 contests = %f, expected ~99.7", f)
	}
}
