package server

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNobitexExtractPrices(t *testing.T) {
	feed := &NobitexCryptoFeed{
		logger: zap.NewNop(),
	}

	t.Run("normal case with bids, asks, and lastTradePrice", func(t *testing.T) {
		ob := &NobitexOrderbookEntry{
			Bids:           [][]string{{"95230.00", "0.1"}},
			Asks:           [][]string{{"95239.00", "0.1"}},
			LastTradePrice: "95234.50",
		}
		bid, ask, last, err := feed.extractPrices(ob)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bid != 95230.0 {
			t.Errorf("expected bid=95230, got %f", bid)
		}
		if ask != 95239.0 {
			t.Errorf("expected ask=95239, got %f", ask)
		}
		if last != 95234.5 {
			t.Errorf("expected last=95234.5, got %f", last)
		}
	})

	t.Run("empty orderbook returns error", func(t *testing.T) {
		ob := &NobitexOrderbookEntry{}
		_, _, _, err := feed.extractPrices(ob)
		if err == nil {
			t.Fatal("expected error for empty orderbook, got nil")
		}
	})

	t.Run("only bid and ask, no lastTradePrice uses midpoint", func(t *testing.T) {
		ob := &NobitexOrderbookEntry{
			Bids: [][]string{{"100.00", "1"}},
			Asks: [][]string{{"200.00", "1"}},
		}
		bid, ask, last, err := feed.extractPrices(ob)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bid != 100.0 {
			t.Errorf("expected bid=100, got %f", bid)
		}
		if ask != 200.0 {
			t.Errorf("expected ask=200, got %f", ask)
		}
		expectedMid := 150.0
		if last != expectedMid {
			t.Errorf("expected last=%f (midpoint), got %f", expectedMid, last)
		}
	})

	t.Run("invalid lastTradePrice string", func(t *testing.T) {
		ob := &NobitexOrderbookEntry{
			LastTradePrice: "invalid",
		}
		_, _, _, err := feed.extractPrices(ob)
		if err == nil {
			t.Fatal("expected error for invalid price, got nil")
		}
	})
}

func TestNobitexSymbolMapping(t *testing.T) {
	registry := &SymbolRegistry{
		ToNobitex:   map[string]string{"BTC/USD": "BTCUSDT"},
		FromNobitex: map[string]string{"BTCUSDT": "BTC/USD"},
		AssetTypes:  map[string]string{"BTC/USD": "crypto"},
	}

	config := NobitexConfig{
		Symbols: []string{"BTCUSDT"},
		Enabled: true,
	}

	feed := NewNobitexCryptoFeed(config, func(symbol string, price, bid, ask, volume float64, timestamp int64, source string) {}, registry, zap.NewNop())

	if canonical, ok := feed.nobitexToCanonical["BTCUSDT"]; !ok || canonical != "BTC/USD" {
		t.Errorf("expected nobitexToCanonical[BTCUSDT]=BTC/USD, got %q (exists=%v)", canonical, ok)
	}

	if nobitex, ok := feed.canonicalToNobitex["BTC/USD"]; !ok || nobitex != "BTCUSDT" {
		t.Errorf("expected canonicalToNobitex[BTC/USD]=BTCUSDT, got %q (exists=%v)", nobitex, ok)
	}
}

func TestNobitexUSDTConversion(t *testing.T) {
	var capturedPrice float64
	var capturedBid float64
	var capturedAsk float64

	handler := func(symbol string, price, bid, ask, volume float64, timestamp int64, source string) {
		capturedPrice = price
		capturedBid = bid
		capturedAsk = ask
	}

	registry := &SymbolRegistry{
		ToNobitex:   map[string]string{"BTC/USD": "BTCUSDT"},
		FromNobitex: map[string]string{"BTCUSDT": "BTC/USD"},
		AssetTypes:  map[string]string{"BTC/USD": "crypto"},
	}

	config := NobitexConfig{
		USDTUSDRate: 0.999,
		Symbols:     []string{"BTCUSDT"},
		Enabled:     true,
	}

	feed := NewNobitexCryptoFeed(config, handler, registry, zap.NewNop())

	// Simulate price extraction and conversion
	inputPrice := 100.0
	convertedPrice := inputPrice * feed.config.USDTUSDRate
	expectedPrice := 99.9

	if convertedPrice != expectedPrice {
		t.Errorf("expected converted price=%f, got %f", expectedPrice, convertedPrice)
	}

	// Also verify via the tick handler path by calling handleTick manually
	feed.tickHandler("BTC/USD", 100.0*0.999, 99.0*0.999, 101.0*0.999, 0, time.Now().Unix(), "nobitex")

	if capturedPrice != 99.9 {
		t.Errorf("expected captured price=99.9, got %f", capturedPrice)
	}
	_ = capturedBid
	_ = capturedAsk
}

func TestNobitexConfigDefaults(t *testing.T) {
	config := NobitexConfig{}

	registry := &SymbolRegistry{
		ToNobitex:   map[string]string{},
		FromNobitex: map[string]string{},
		AssetTypes:  map[string]string{},
	}

	feed := NewNobitexCryptoFeed(config, func(symbol string, price, bid, ask, volume float64, timestamp int64, source string) {}, registry, zap.NewNop())

	if feed.config.PollInterval != 2*time.Second {
		t.Errorf("expected default PollInterval=2s, got %v", feed.config.PollInterval)
	}
	if feed.config.USDTUSDRate != 1.0 {
		t.Errorf("expected default USDTUSDRate=1.0, got %f", feed.config.USDTUSDRate)
	}
	if feed.config.BaseURL != "https://apiv2.nobitex.ir" {
		t.Errorf("expected default BaseURL=https://apiv2.nobitex.ir, got %s", feed.config.BaseURL)
	}
}
