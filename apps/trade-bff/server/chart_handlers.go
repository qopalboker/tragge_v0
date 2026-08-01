package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ====================
// Chart Types
// ====================

// TradingViewBar represents a single OHLCV bar in TradingView format
type TradingViewBar struct {
	Time   int64   `json:"time"`   // Unix timestamp in milliseconds
	Open   float64 `json:"open"`   // Opening price
	High   float64 `json:"high"`   // High price
	Low    float64 `json:"low"`    // Low price
	Close  float64 `json:"close"`  // Closing price
	Volume float64 `json:"volume"` // Volume
}

// TradingViewCandlesResponse represents the TradingView-compatible candles response
type TradingViewCandlesResponse struct {
	Bars   []TradingViewBar `json:"bars"`   // Array of OHLCV bars
	NoData bool             `json:"noData"` // True if no data available for the requested range
}

// ContestSymbol represents a symbol available in a contest
type ContestSymbol struct {
	Symbol  string `json:"symbol"`
	Enabled bool   `json:"enabled"`
}

// ContestSymbolsResponse represents the response for contest symbols
type ContestSymbolsResponse struct {
	ContestID string          `json:"contest_id"`
	Symbols   []ContestSymbol `json:"symbols"`
}

// mapTradingViewResolution converts TradingView resolution format to internal format
// TradingView uses: "1", "5", "15", "30", "60", "D", "W", "M" (minutes or D/W/M for daily/weekly/monthly)
// Internal uses: "1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w", "1M"
func mapTradingViewResolution(tvRes string) (string, bool) {
	switch tvRes {
	case "1":
		return "1m", true
	case "5":
		return "5m", true
	case "15":
		return "15m", true
	case "30":
		return "30m", true
	case "60", "1H":
		return "1h", true
	case "240", "4H":
		return "4h", true
	case "D", "1D":
		return "1d", true
	case "W", "1W":
		return "1w", true
	case "M", "1M":
		return "1M", true
	default:
		return "", false
	}
}

// handleCandles handles GET /api/trade/candles
// Returns OHLCV candles in TradingView-compatible format
// Query params:
//   - symbol (required): Trading symbol (e.g., "AAPL")
//   - resolution (required): Time resolution ("1", "5", "15", "30", "60", "D")
//   - from (required): Start time as Unix timestamp in seconds
//   - to (required): End time as Unix timestamp in seconds
func (a *App) handleCandles(w http.ResponseWriter, r *http.Request) {
	// Parse and validate symbol parameter
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.SymbolRequired})
		return
	}
	// Validate symbol format (alphanumeric, max 20 chars)
	if len(symbol) > 20 || !isValidSymbol(symbol) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.SymbolInvalid})
		return
	}

	// Parse and validate resolution parameter
	tvResolution := r.URL.Query().Get("resolution")
	if tvResolution == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ResolutionRequired})
		return
	}
	resolution, valid := mapTradingViewResolution(tvResolution)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ResolutionInvalid})
		return
	}

	// Parse and validate from parameter
	fromStr := r.URL.Query().Get("from")
	if fromStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.FromRequired})
		return
	}
	fromTS, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil || fromTS < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.FromInvalid})
		return
	}

	// Parse and validate to parameter
	toStr := r.URL.Query().Get("to")
	if toStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ToRequired})
		return
	}
	toTS, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil || toTS < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ToInvalid})
		return
	}

	// Validate time range
	if fromTS >= toTS {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.FromMustBeLessThanTo})
		return
	}

	ctx := r.Context()

	// Monthly candles are computed on-demand from daily candles
	if resolution == "1M" {
		bars, err := a.aggregateMonthlyCandles(ctx, symbol, fromTS, toTS)
		if err != nil {
			a.log().Error("Failed to aggregate monthly candles",
				zap.Error(err),
				zap.String("symbol", symbol),
			)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.DatabaseError})
			return
		}
		writeJSON(w, http.StatusOK, TradingViewCandlesResponse{
			Bars:   bars,
			NoData: len(bars) == 0,
		})
		return
	}

	// Query candles from database with time range, limit to 5000 bars max
	const maxBars = 5000
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT time, open, high, low, close, volume
		FROM candles
		WHERE symbol = $1 AND resolution = $2 AND time >= $3 AND time < $4
		ORDER BY time ASC
		LIMIT $5
	`, symbol, resolution, fromTS, toTS, maxBars)
	if err != nil {
		a.log().Error("Failed to query candles",
			zap.Error(err),
			zap.String("symbol", symbol),
			zap.String("resolution", resolution),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.DatabaseError})
		return
	}
	defer rows.Close()

	bars := make([]TradingViewBar, 0)
	for rows.Next() {
		var timeSec int64
		var open, high, low, closePrice, volume float64
		if err := rows.Scan(&timeSec, &open, &high, &low, &closePrice, &volume); err != nil {
			a.log().Error("Failed to scan candle row", zap.Error(err))
			continue
		}
		bars = append(bars, TradingViewBar{
			Time:   timeSec * 1000, // Convert seconds to milliseconds for TradingView
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating candle rows", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.DatabaseError})
		return
	}

	// Append the current in-progress candle from the aggregator to close
	// the gap between the last persisted candle and live WebSocket ticks.
	if ip := a.candleAggregator.GetInProgressCandle(symbol, resolution); ip != nil {
		if ip.Time >= fromTS && ip.Time < toTS {
			bar := TradingViewBar{
				Time:   ip.Time * 1000,
				Open:   ip.Open,
				High:   ip.High,
				Low:    ip.Low,
				Close:  ip.Close,
				Volume: ip.Volume,
			}
			if n := len(bars); n > 0 && bars[n-1].Time == bar.Time {
				bars[n-1] = bar
			} else {
				bars = append(bars, bar)
			}
		}
	}

	response := TradingViewCandlesResponse{
		Bars:   bars,
		NoData: len(bars) == 0,
	}

	writeJSON(w, http.StatusOK, response)
}

// aggregateMonthlyCandles computes monthly OHLCV candles from daily candles on-demand.
// Groups daily candles by calendar month and returns aggregated bars.
func (a *App) aggregateMonthlyCandles(ctx context.Context, symbol string, fromTS, toTS int64) ([]TradingViewBar, error) {
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT time, open, high, low, close, volume
		FROM candles
		WHERE symbol = $1 AND resolution = '1d' AND time >= $2 AND time < $3
		ORDER BY time ASC
	`, symbol, fromTS, toTS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Group daily candles by year-month
	type monthKey struct {
		year  int
		month time.Month
	}
	type monthCandle struct {
		time   int64 // Unix timestamp of first day in the month
		open   float64
		high   float64
		low    float64
		close  float64
		volume float64
		set    bool
	}

	months := make(map[monthKey]*monthCandle)
	var monthOrder []monthKey

	for rows.Next() {
		var timeSec int64
		var open, high, low, closePrice, volume float64
		if err := rows.Scan(&timeSec, &open, &high, &low, &closePrice, &volume); err != nil {
			continue
		}

		t := time.Unix(timeSec, 0).UTC()
		mk := monthKey{year: t.Year(), month: t.Month()}

		mc, exists := months[mk]
		if !exists {
			// Month start timestamp (first day of the month at 00:00 UTC)
			monthStart := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
			mc = &monthCandle{
				time:   monthStart.Unix(),
				open:   open,
				high:   high,
				low:    low,
				close:  closePrice,
				volume: volume,
				set:    true,
			}
			months[mk] = mc
			monthOrder = append(monthOrder, mk)
			continue
		}

		// Update existing month candle
		if high > mc.high {
			mc.high = high
		}
		if low < mc.low {
			mc.low = low
		}
		mc.close = closePrice
		mc.volume += volume
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	bars := make([]TradingViewBar, 0, len(monthOrder))
	for _, mk := range monthOrder {
		mc := months[mk]
		bars = append(bars, TradingViewBar{
			Time:   mc.time * 1000, // Convert to milliseconds for TradingView
			Open:   mc.open,
			High:   mc.high,
			Low:    mc.low,
			Close:  mc.close,
			Volume: mc.volume,
		})
	}

	return bars, nil
}

// isValidSymbol checks if a symbol contains only valid characters
func isValidSymbol(s string) bool {
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' || r == '/') {
			return false
		}
	}
	return len(s) > 0
}

// handleContestSymbols handles GET /api/trade/contest/{contest_id}/symbols
// Returns symbols available for a specific contest
func (a *App) handleContestSymbols(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "contest_id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ContestIDRequired})
		return
	}

	ctx := r.Context()

	// Query contest_symbols table
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT symbol, enabled
		FROM contest_symbols
		WHERE contest_id = $1
		ORDER BY symbol ASC
	`, contestID)
	if err != nil {
		a.log().Error("Failed to query contest symbols", zap.Error(err), zap.String("contest_id", contestID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}
	defer rows.Close()

	symbols := make([]ContestSymbol, 0)
	for rows.Next() {
		var cs ContestSymbol
		if err := rows.Scan(&cs.Symbol, &cs.Enabled); err != nil {
			a.log().Error("Failed to scan contest symbol", zap.Error(err))
			continue
		}
		symbols = append(symbols, cs)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating contest symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	response := ContestSymbolsResponse{
		ContestID: contestID,
		Symbols:   symbols,
	}

	writeJSON(w, http.StatusOK, response)
}
