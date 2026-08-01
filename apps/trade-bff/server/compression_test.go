package server

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"testing"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// BenchmarkCompressionLevels benchmarks different compression levels
// to help choose the optimal setting for high concurrency scenarios
func BenchmarkCompressionLevels(b *testing.B) {
	// Generate a typical tick_snapshot message (similar to production)
	snapshot := generateTickSnapshot(30) // 30 symbols, typical size
	msg := WSMessage{
		Type:    "tick_snapshot",
		Payload: snapshot,
	}
	data, _ := json.Marshal(msg)

	b.Logf("Original message size: %d bytes", len(data))

	levels := []struct {
		name  string
		level int
	}{
		{"BestSpeed", flate.BestSpeed},                   // Level 1
		{"Level3", 3},                                    // Level 3
		{"DefaultCompression", flate.DefaultCompression}, // Level -1 (typically 6)
		{"Level6", 6},                                    // Level 6
		{"BestCompression", flate.BestCompression},       // Level 9
	}

	for _, l := range levels {
		b.Run(l.name, func(b *testing.B) {
			var totalCompressedSize int64
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				compressed := compressData(data, l.level)
				totalCompressedSize += int64(len(compressed))
			}
			avgCompressed := totalCompressedSize / int64(b.N)
			ratio := 1.0 - float64(avgCompressed)/float64(len(data))
			b.ReportMetric(float64(avgCompressed), "compressed_bytes")
			b.ReportMetric(ratio*100, "compression_%")
		})
	}
}

// BenchmarkMessageSizes benchmarks compression on different message sizes
func BenchmarkMessageSizes(b *testing.B) {
	sizes := []int{1, 5, 10, 20, 30, 50}

	for _, symbols := range sizes {
		snapshot := generateTickSnapshot(symbols)
		msg := WSMessage{
			Type:    "tick_snapshot",
			Payload: snapshot,
		}
		data, _ := json.Marshal(msg)

		b.Run("Symbols_"+string(rune('0'+symbols/10))+string(rune('0'+symbols%10)), func(b *testing.B) {
			b.ReportMetric(float64(len(data)), "original_bytes")

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				compressData(data, flate.BestSpeed)
			}
		})
	}
}

// BenchmarkSmallMessages benchmarks compression overhead for small messages
// to validate the minimum message size threshold
func BenchmarkSmallMessages(b *testing.B) {
	smallMessages := []struct {
		name string
		msg  interface{}
	}{
		{"welcome_50bytes", WSMessage{Type: "contest_state", Phase: "CONNECTING"}},
		{"order_ack_150bytes", WSMessage{
			Type: "order_ack",
			Payload: contracts.OrderAck{
				OrderID: "ord-12345678-abcd-1234-efgh-123456789012",
				Status:  "filled",
			},
		}},
		{"position_update_200bytes", WSMessage{
			Type: "position_update",
			Payload: contracts.PositionUpdate{
				UserID:    "user-12345678-abcd-1234-efgh-123456789012",
				ContestID: "contest-12345678-abcd-1234-efgh-123456789012",
				Positions: []contracts.Position{
					{
						PositionID: "pos-1",
						Symbol:     "AAPL",
						Side:       "long",
						Qty:        100,
						EntryPrice: 175.50,
						MarkPrice:  176.25,
					},
				},
			},
		}},
	}

	for _, sm := range smallMessages {
		data, _ := json.Marshal(sm.msg)

		b.Run(sm.name, func(b *testing.B) {
			b.ReportMetric(float64(len(data)), "original_bytes")

			// Benchmark with compression
			b.Run("compressed", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					compressData(data, flate.BestSpeed)
				}
			})

			// Benchmark without compression (baseline)
			b.Run("uncompressed", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					// Simulate message copying (what happens without compression)
					_ = make([]byte, len(data))
					copy(make([]byte, len(data)), data)
				}
			})
		})
	}
}

// TestCompressionRatio tests actual compression ratios for typical messages
func TestCompressionRatio(t *testing.T) {
	testCases := []struct {
		name     string
		symbols  int
		minRatio float64 // Minimum expected compression ratio
	}{
		{"small_5_symbols", 5, 0.40},
		{"medium_15_symbols", 15, 0.55},
		{"typical_30_symbols", 30, 0.60},
		{"large_50_symbols", 50, 0.65},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := generateTickSnapshot(tc.symbols)
			msg := WSMessage{
				Type:    "tick_snapshot",
				Payload: snapshot,
			}
			data, _ := json.Marshal(msg)

			compressed := compressData(data, flate.BestSpeed)
			ratio := 1.0 - float64(len(compressed))/float64(len(data))

			t.Logf("Original: %d bytes, Compressed: %d bytes, Ratio: %.2f%%",
				len(data), len(compressed), ratio*100)

			if ratio < tc.minRatio {
				t.Errorf("Compression ratio %.2f%% below expected minimum %.2f%%",
					ratio*100, tc.minRatio*100)
			}
		})
	}
}

// TestMinMessageSizeThreshold validates the 100 byte threshold for compression
func TestMinMessageSizeThreshold(t *testing.T) {
	// Messages below 100 bytes - compression overhead may exceed savings
	smallMsg := WSMessage{Type: "contest_state", Phase: "CONNECTING"}
	smallData, _ := json.Marshal(smallMsg)

	smallCompressed := compressData(smallData, flate.BestSpeed)

	t.Logf("Small message: original=%d, compressed=%d", len(smallData), len(smallCompressed))

	// For very small messages, compressed size may be larger due to overhead
	if len(smallData) < 100 {
		// This is expected - small messages shouldn't be compressed
		t.Logf("Message under 100 bytes - compression may not be beneficial")
	}

	// Messages above 100 bytes - compression should help
	mediumSnapshot := generateTickSnapshot(5)
	mediumMsg := WSMessage{Type: "tick_snapshot", Payload: mediumSnapshot}
	mediumData, _ := json.Marshal(mediumMsg)

	mediumCompressed := compressData(mediumData, flate.BestSpeed)

	t.Logf("Medium message: original=%d, compressed=%d", len(mediumData), len(mediumCompressed))

	if len(mediumData) > 100 && len(mediumCompressed) >= len(mediumData) {
		t.Errorf("Expected compression benefit for message > 100 bytes")
	}
}

// Helper functions

func generateTickSnapshot(numSymbols int) *contracts.TickSnapshot {
	symbols := []string{
		"AAPL", "GOOGL", "MSFT", "AMZN", "TSLA",
		"META", "NVDA", "AMD", "INTC", "NFLX",
		"JPM", "BAC", "GS", "V", "MA",
		"JNJ", "UNH", "PFE", "ABBV", "MRK",
		"WMT", "HD", "KO", "PEP", "NKE",
		"DIS", "XOM", "CVX", "BA", "CRM",
		"PYPL", "ADBE", "CRM", "ORCL", "IBM",
		"CSCO", "QCOM", "TXN", "AVGO", "SHOP",
		"SQ", "UBER", "LYFT", "SNAP", "PINS",
		"TWTR", "ZM", "DOCU", "ROKU", "SPOT",
	}

	if numSymbols > len(symbols) {
		numSymbols = len(symbols)
	}

	ticks := make([]contracts.SymbolTick, numSymbols)
	basePrice := 100.0
	for i := 0; i < numSymbols; i++ {
		price := basePrice + float64(i)*10
		ticks[i] = contracts.SymbolTick{
			Symbol: symbols[i],
			Bid:    price - 0.05,
			Ask:    price + 0.05,
			Last:   price,
		}
	}

	return &contracts.TickSnapshot{
		Ts:      1704067200000, // Fixed timestamp for reproducible tests
		Symbols: ticks,
	}
}

func compressData(data []byte, level int) []byte {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, level)
	if err != nil {
		return data
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}
