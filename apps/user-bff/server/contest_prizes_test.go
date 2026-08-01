package server

import "testing"

func TestResolveEffectiveFeeBps(t *testing.T) {
	tests := []struct {
		name           string
		platformFeeBps int
		commissionRate float64
		want           int
	}{
		{
			name:           "platform_fee_bps takes priority when set",
			platformFeeBps: 2500,
			commissionRate: 20.0,
			want:           2500,
		},
		{
			name:           "falls back to commission_rate when bps is 0",
			platformFeeBps: 0,
			commissionRate: 20.0,
			want:           2000,
		},
		{
			name:           "commission_rate 17.0 converts to 1700 bps",
			platformFeeBps: 0,
			commissionRate: 17.0,
			want:           1700,
		},
		{
			name:           "both zero returns default 2000",
			platformFeeBps: 0,
			commissionRate: 0.0,
			want:           DefaultPlatformFeeBps,
		},
		{
			name:           "commission_rate with floating-point rounding",
			platformFeeBps: 0,
			commissionRate: 19.995,
			want:           2000, // math.Round(19.995 * 100) = 2000
		},
		{
			name:           "negative platform_fee_bps ignored",
			platformFeeBps: -1,
			commissionRate: 20.0,
			want:           2000,
		},
		{
			name:           "negative commission_rate ignored",
			platformFeeBps: 0,
			commissionRate: -5.0,
			want:           DefaultPlatformFeeBps,
		},
		{
			name:           "commission_rate over 100 ignored",
			platformFeeBps: 0,
			commissionRate: 150.0,
			want:           DefaultPlatformFeeBps,
		},
		{
			name:           "small commission_rate 0.5 converts to 50 bps",
			platformFeeBps: 0,
			commissionRate: 0.5,
			want:           50,
		},
		{
			name:           "platform_fee_bps at boundary 10000",
			platformFeeBps: 10000,
			commissionRate: 0,
			want:           10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveEffectiveFeeBps(tt.platformFeeBps, tt.commissionRate)
			if got != tt.want {
				t.Errorf("ResolveEffectiveFeeBps(%d, %f) = %d, want %d",
					tt.platformFeeBps, tt.commissionRate, got, tt.want)
			}
		})
	}
}
