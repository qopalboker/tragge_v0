package server

import (
	"testing"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// TestGetMaxPriceAge verifies the per-asset-class price freshness threshold resolution.
func TestGetMaxPriceAge(t *testing.T) {
	engine := &Engine{
		config: &Config{
			MaxPriceAgeOpenCrypto:  30 * time.Second,
			MaxPriceAgeOpenForex:   60 * time.Second,
			MaxPriceAgeCloseCrypto: 60 * time.Second,
			MaxPriceAgeCloseForex:  120 * time.Second,
			MaxPriceAgeMarket:      30 * time.Second,
			MaxPriceAgePending:     60 * time.Second,
		},
	}

	tests := []struct {
		name       string
		assetClass string
		isClose    bool
		expected   time.Duration
	}{
		{
			name:       "crypto open",
			assetClass: "crypto",
			isClose:    false,
			expected:   30 * time.Second,
		},
		{
			name:       "crypto close",
			assetClass: "crypto",
			isClose:    true,
			expected:   60 * time.Second,
		},
		{
			name:       "forex open",
			assetClass: "forex",
			isClose:    false,
			expected:   60 * time.Second,
		},
		{
			name:       "forex close",
			assetClass: "forex",
			isClose:    true,
			expected:   120 * time.Second,
		},
		{
			name:       "unknown asset class open falls back to MaxPriceAgeMarket",
			assetClass: "stocks",
			isClose:    false,
			expected:   30 * time.Second,
		},
		{
			name:       "unknown asset class close falls back to MaxPriceAgePending",
			assetClass: "stocks",
			isClose:    true,
			expected:   60 * time.Second,
		},
		{
			name:       "empty asset class open falls back to MaxPriceAgeMarket",
			assetClass: "",
			isClose:    false,
			expected:   30 * time.Second,
		},
		{
			name:       "empty asset class close falls back to MaxPriceAgePending",
			assetClass: "",
			isClose:    true,
			expected:   60 * time.Second,
		},
		{
			name:       "case insensitive Crypto",
			assetClass: "Crypto",
			isClose:    false,
			expected:   30 * time.Second,
		},
		{
			name:       "case insensitive FOREX",
			assetClass: "FOREX",
			isClose:    true,
			expected:   120 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.getMaxPriceAge(tc.assetClass, tc.isClose)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

// TestPriceBookFreshness verifies PriceBook stale price detection.
func TestPriceBookFreshness(t *testing.T) {
	t.Run("fresh price is not stale", func(t *testing.T) {
		pb := NewPriceBook()
		// Simulate a fresh tick
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().UnixMilli(),
			Symbols: []contracts.SymbolTick{
				{Symbol: "BTCUSD", Bid: 50000.0, Ask: 50100.0, Last: 50050.0},
			},
		})

		// 5-second-old price should be accepted with 30s threshold
		if pb.IsStale("BTCUSD", 30*time.Second) {
			t.Error("expected fresh price to not be stale with 30s threshold")
		}
	})

	t.Run("old price is stale", func(t *testing.T) {
		pb := NewPriceBook()
		// Simulate a 45-second-old tick
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-45 * time.Second).UnixMilli(),
			Symbols: []contracts.SymbolTick{
				{Symbol: "BTCUSD", Bid: 50000.0, Ask: 50100.0, Last: 50050.0},
			},
		})

		// 45-second-old crypto price should be rejected with 30s threshold
		if !pb.IsStale("BTCUSD", 30*time.Second) {
			t.Error("expected 45s old price to be stale with 30s threshold")
		}
	})

	t.Run("no price at all is stale", func(t *testing.T) {
		pb := NewPriceBook()
		// No tick has been received for this symbol
		if !pb.IsStale("UNKNOWN", 30*time.Second) {
			t.Error("expected missing symbol to be stale")
		}
	})

	t.Run("GetFillPriceWithAge returns correct age", func(t *testing.T) {
		pb := NewPriceBook()
		tickAge := 5 * time.Second
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-tickAge).UnixMilli(),
			Symbols: []contracts.SymbolTick{
				{Symbol: "AAPL", Bid: 150.0, Ask: 151.0, Last: 150.5},
			},
		})

		price, age, ok := pb.GetFillPriceWithAge("AAPL", contracts.OrderSideBuy)
		if !ok {
			t.Fatal("expected to get price")
		}
		if price != 151.0 {
			t.Errorf("expected ask price 151.0 for BUY, got %f", price)
		}
		// Age should be approximately 5 seconds (allow 2s tolerance for test execution time)
		if age < 4*time.Second || age > 7*time.Second {
			t.Errorf("expected age ~5s, got %v", age)
		}
	})

	t.Run("GetFillPriceWithAge returns false for missing symbol", func(t *testing.T) {
		pb := NewPriceBook()
		_, _, ok := pb.GetFillPriceWithAge("MISSING", contracts.OrderSideBuy)
		if ok {
			t.Error("expected false for missing symbol")
		}
	})
}

// TestPriceFreshnessAcceptReject verifies the accept/reject scenarios from the requirements.
func TestPriceFreshnessAcceptReject(t *testing.T) {
	t.Run("5-second-old price accepted for crypto open (30s threshold)", func(t *testing.T) {
		engine := &Engine{
			config: &Config{
				MaxPriceAgeOpenCrypto:  30 * time.Second,
				MaxPriceAgeOpenForex:   60 * time.Second,
				MaxPriceAgeCloseCrypto: 60 * time.Second,
				MaxPriceAgeCloseForex:  120 * time.Second,
				MaxPriceAgeMarket:      30 * time.Second,
				MaxPriceAgePending:     60 * time.Second,
			},
		}

		maxAge := engine.getMaxPriceAge("crypto", false)
		priceAge := 5 * time.Second

		if priceAge > maxAge {
			t.Errorf("5s old price should be accepted with %v threshold", maxAge)
		}
	})

	t.Run("45-second-old crypto price rejected for open (30s threshold)", func(t *testing.T) {
		engine := &Engine{
			config: &Config{
				MaxPriceAgeOpenCrypto:  30 * time.Second,
				MaxPriceAgeOpenForex:   60 * time.Second,
				MaxPriceAgeCloseCrypto: 60 * time.Second,
				MaxPriceAgeCloseForex:  120 * time.Second,
				MaxPriceAgeMarket:      30 * time.Second,
				MaxPriceAgePending:     60 * time.Second,
			},
		}

		maxAge := engine.getMaxPriceAge("crypto", false)
		priceAge := 45 * time.Second

		if priceAge <= maxAge {
			t.Errorf("45s old crypto price should be rejected with %v threshold", maxAge)
		}
	})

	t.Run("45-second-old crypto price accepted for close (60s threshold)", func(t *testing.T) {
		engine := &Engine{
			config: &Config{
				MaxPriceAgeOpenCrypto:  30 * time.Second,
				MaxPriceAgeOpenForex:   60 * time.Second,
				MaxPriceAgeCloseCrypto: 60 * time.Second,
				MaxPriceAgeCloseForex:  120 * time.Second,
				MaxPriceAgeMarket:      30 * time.Second,
				MaxPriceAgePending:     60 * time.Second,
			},
		}

		maxAge := engine.getMaxPriceAge("crypto", true)
		priceAge := 45 * time.Second

		if priceAge > maxAge {
			t.Errorf("45s old crypto price should be accepted for close with %v threshold", maxAge)
		}
	})

	t.Run("90-second-old forex price accepted for close (120s threshold)", func(t *testing.T) {
		engine := &Engine{
			config: &Config{
				MaxPriceAgeOpenCrypto:  30 * time.Second,
				MaxPriceAgeOpenForex:   60 * time.Second,
				MaxPriceAgeCloseCrypto: 60 * time.Second,
				MaxPriceAgeCloseForex:  120 * time.Second,
				MaxPriceAgeMarket:      30 * time.Second,
				MaxPriceAgePending:     60 * time.Second,
			},
		}

		maxAge := engine.getMaxPriceAge("forex", true)
		priceAge := 90 * time.Second

		if priceAge > maxAge {
			t.Errorf("90s old forex price should be accepted for close with %v threshold", maxAge)
		}
	})

	t.Run("90-second-old forex price rejected for open (60s threshold)", func(t *testing.T) {
		engine := &Engine{
			config: &Config{
				MaxPriceAgeOpenCrypto:  30 * time.Second,
				MaxPriceAgeOpenForex:   60 * time.Second,
				MaxPriceAgeCloseCrypto: 60 * time.Second,
				MaxPriceAgeCloseForex:  120 * time.Second,
				MaxPriceAgeMarket:      30 * time.Second,
				MaxPriceAgePending:     60 * time.Second,
			},
		}

		maxAge := engine.getMaxPriceAge("forex", false)
		priceAge := 90 * time.Second

		if priceAge <= maxAge {
			t.Errorf("90s old forex price should be rejected for open with %v threshold", maxAge)
		}
	})
}

// TestPriceBookGetPriceAge verifies the GetPriceAge method on PriceBook.
func TestPriceBookGetPriceAge(t *testing.T) {
	t.Run("returns age for existing symbol", func(t *testing.T) {
		pb := NewPriceBook()
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-10 * time.Second).UnixMilli(),
			Symbols: []contracts.SymbolTick{
				{Symbol: "EURUSD", Last: 1.1234},
			},
		})

		age, exists := pb.GetPriceAge("EURUSD")
		if !exists {
			t.Fatal("expected symbol to exist")
		}
		if age < 9*time.Second || age > 12*time.Second {
			t.Errorf("expected age ~10s, got %v", age)
		}
	})

	t.Run("returns false for missing symbol", func(t *testing.T) {
		pb := NewPriceBook()
		_, exists := pb.GetPriceAge("MISSING")
		if exists {
			t.Error("expected false for missing symbol")
		}
	})

	t.Run("returns correct age when Ts is in seconds", func(t *testing.T) {
		pb := NewPriceBook()
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-10 * time.Second).Unix(), // seconds, not millis
			Symbols: []contracts.SymbolTick{
				{Symbol: "GBPUSD", Last: 1.2500},
			},
		})

		age, exists := pb.GetPriceAge("GBPUSD")
		if !exists {
			t.Fatal("expected symbol to exist")
		}
		if age < 9*time.Second || age > 12*time.Second {
			t.Errorf("expected age ~10s, got %v", age)
		}
	})
}

// TestNormalizeToMillis verifies the timestamp normalization helper.
func TestNormalizeToMillis(t *testing.T) {
	t.Run("converts seconds to milliseconds", func(t *testing.T) {
		sec := int64(1700000000) // clearly seconds (< 1e12)
		got := normalizeToMillis(sec)
		if got != sec*1000 {
			t.Errorf("expected %d, got %d", sec*1000, got)
		}
	})

	t.Run("passes through milliseconds unchanged", func(t *testing.T) {
		ms := int64(1700000000000) // clearly milliseconds (>= 1e12)
		got := normalizeToMillis(ms)
		if got != ms {
			t.Errorf("expected %d, got %d", ms, got)
		}
	})

	t.Run("zero stays zero", func(t *testing.T) {
		got := normalizeToMillis(0)
		if got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})
}

// TestPriceBookFreshnessWithSecondsTimestamp verifies PriceBook handles seconds-format timestamps.
func TestPriceBookFreshnessWithSecondsTimestamp(t *testing.T) {
	t.Run("fresh price in seconds is not stale", func(t *testing.T) {
		pb := NewPriceBook()
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Unix(), // seconds
			Symbols: []contracts.SymbolTick{
				{Symbol: "BTCUSD", Bid: 50000.0, Ask: 50100.0, Last: 50050.0},
			},
		})

		if pb.IsStale("BTCUSD", 30*time.Second) {
			t.Error("expected fresh price (in seconds) to not be stale")
		}
	})

	t.Run("old price in seconds is stale", func(t *testing.T) {
		pb := NewPriceBook()
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-45 * time.Second).Unix(), // seconds
			Symbols: []contracts.SymbolTick{
				{Symbol: "BTCUSD", Bid: 50000.0, Ask: 50100.0, Last: 50050.0},
			},
		})

		if !pb.IsStale("BTCUSD", 30*time.Second) {
			t.Error("expected 45s old price (in seconds) to be stale")
		}
	})

	t.Run("GetFillPriceWithAge correct with seconds timestamp", func(t *testing.T) {
		pb := NewPriceBook()
		pb.UpdateFromTick(&contracts.TickSnapshot{
			Ts: time.Now().Add(-5 * time.Second).Unix(), // seconds
			Symbols: []contracts.SymbolTick{
				{Symbol: "AAPL", Bid: 150.0, Ask: 151.0, Last: 150.5},
			},
		})

		price, age, ok := pb.GetFillPriceWithAge("AAPL", contracts.OrderSideBuy)
		if !ok {
			t.Fatal("expected to get price")
		}
		if price != 151.0 {
			t.Errorf("expected ask price 151.0 for BUY, got %f", price)
		}
		if age < 4*time.Second || age > 7*time.Second {
			t.Errorf("expected age ~5s, got %v", age)
		}
	})
}
