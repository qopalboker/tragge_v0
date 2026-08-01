package prize

import "testing"

func TestCommissionPercentToFraction(t *testing.T) {
	tests := []struct {
		name    string
		pct     float64
		want    float64
		wantErr bool
	}{
		{"zero", 0.0, 0.0, false},
		{"twenty_percent", 20.0, 0.20, false},
		{"hundred_percent", 100.0, 1.0, false},
		{"fractional", 15.5, 0.155, false},
		{"small", 0.5, 0.005, false},
		{"negative", -1.0, 0, true},
		{"over_hundred", 101.0, 0, true},
		{"large_negative", -50.0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CommissionPercentToFraction(tt.pct)
			if (err != nil) != tt.wantErr {
				t.Errorf("CommissionPercentToFraction(%v) error = %v, wantErr %v", tt.pct, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CommissionPercentToFraction(%v) = %v, want %v", tt.pct, got, tt.want)
			}
		})
	}
}

func TestMustCommissionPercentToFraction(t *testing.T) {
	// Valid case
	got := MustCommissionPercentToFraction(20.0)
	if got != 0.20 {
		t.Errorf("MustCommissionPercentToFraction(20.0) = %v, want 0.20", got)
	}
}

func TestMustCommissionPercentToFraction_PanicNegative(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative commission rate")
		}
	}()
	MustCommissionPercentToFraction(-1.0)
}

func TestMustCommissionPercentToFraction_PanicOver100(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for commission rate over 100")
		}
	}()
	MustCommissionPercentToFraction(101.0)
}
