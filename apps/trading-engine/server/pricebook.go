package server

import (
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// normalizeToMillis converts a timestamp to milliseconds.
// If the timestamp appears to be in seconds (< 1e12), it multiplies by 1000.
// If already in milliseconds, it returns as-is.
func normalizeToMillis(ts int64) int64 {
	if ts < 1e12 {
		return ts * 1000
	}
	return ts
}

// DefaultSpreadBps is the default spread in basis points used for synthetic bid/ask derivation.
const DefaultSpreadBps = 10 // 0.1% spread (10 basis points)

// Quote represents the current bid/ask/last for a symbol.
type Quote struct {
	Symbol    string
	Bid       float64
	Ask       float64
	Last      float64
	Timestamp int64 // Unix milliseconds
}

// HasBidAsk returns true if both bid and ask are available.
func (q *Quote) HasBidAsk() bool {
	return q.Bid > 0 && q.Ask > 0
}

// PriceBook maintains in-memory bid/ask prices per symbol.
type PriceBook struct {
	quotes    map[string]*Quote // symbol -> quote
	mu        sync.RWMutex
	spreadBps int // spread in basis points for synthetic bid/ask
}

// NewPriceBook creates a new price book.
func NewPriceBook() *PriceBook {
	return &PriceBook{
		quotes:    make(map[string]*Quote),
		spreadBps: DefaultSpreadBps,
	}
}

// UpdateFromTick updates the price book from a tick snapshot.
func (pb *PriceBook) UpdateFromTick(tick *contracts.TickSnapshot) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	for _, st := range tick.Symbols {
		quote := pb.getOrCreateQuoteLocked(st.Symbol)

		// Update timestamp (normalize to milliseconds)
		quote.Timestamp = normalizeToMillis(tick.Ts)

		// Update last price if available
		if st.Last > 0 {
			quote.Last = st.Last
		}

		// Update bid/ask from tick data.
		// When provider sends zero, reset stored value so synthetic generation
		// from Last price can kick in (prevents stale real values from persisting).
		if st.Bid > 0 {
			quote.Bid = st.Bid
		} else {
			quote.Bid = 0
		}
		if st.Ask > 0 {
			quote.Ask = st.Ask
		} else {
			quote.Ask = 0
		}

		// Derive synthetic bid/ask ONLY for missing values (preserve real data)
		if quote.Last > 0 {
			halfSpreadPct := float64(pb.spreadBps) / 2 / 10000.0
			if quote.Bid <= 0 {
				quote.Bid = quote.Last * (1 - halfSpreadPct)
			}
			if quote.Ask <= 0 {
				quote.Ask = quote.Last * (1 + halfSpreadPct)
			}
		}
	}
}

// getOrCreateQuoteLocked gets or creates a quote for a symbol.
// Must be called with lock held.
func (pb *PriceBook) getOrCreateQuoteLocked(symbol string) *Quote {
	if quote, exists := pb.quotes[symbol]; exists {
		return quote
	}

	quote := &Quote{
		Symbol:    symbol,
		Timestamp: time.Now().UnixMilli(),
	}
	pb.quotes[symbol] = quote
	return quote
}

// GetQuote returns the current quote for a symbol.
func (pb *PriceBook) GetQuote(symbol string) (*Quote, bool) {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	quote, exists := pb.quotes[symbol]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	quoteCopy := *quote
	return &quoteCopy, true
}

// GetBidAsk returns the bid and ask for a symbol.
// If not available, returns false.
// Note: use GetBidAskDirect on hot paths to avoid the Quote copy.
func (pb *PriceBook) GetBidAsk(symbol string) (bid, ask float64, ok bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists || quote.Bid <= 0 || quote.Ask <= 0 {
		return 0, 0, false
	}
	return quote.Bid, quote.Ask, true
}

// GetBidAskDirect returns bid and ask scalars directly under a single RLock,
// avoiding the full Quote struct copy. Preferred on hot paths like tick processing
// and pending order evaluation.
func (pb *PriceBook) GetBidAskDirect(symbol string) (bid, ask float64, ok bool) {
	pb.mu.RLock()
	q, exists := pb.quotes[symbol]
	if !exists || q.Bid <= 0 || q.Ask <= 0 {
		pb.mu.RUnlock()
		return 0, 0, false
	}
	bid, ask = q.Bid, q.Ask
	pb.mu.RUnlock()
	return bid, ask, true
}

// GetFillPriceDirect returns the fill price and age for a symbol/side directly
// under a single RLock, avoiding the Quote struct copy. Preferred on hot paths.
func (pb *PriceBook) GetFillPriceDirect(symbol string, side contracts.OrderSide) (price float64, age time.Duration, ok bool) {
	pb.mu.RLock()
	q, exists := pb.quotes[symbol]
	if !exists {
		pb.mu.RUnlock()
		return 0, 0, false
	}
	ts := q.Timestamp
	if side == contracts.OrderSideBuy {
		if q.Ask > 0 {
			price = q.Ask
		} else if q.Last > 0 {
			price = q.Last
		}
	} else {
		if q.Bid > 0 {
			price = q.Bid
		} else if q.Last > 0 {
			price = q.Last
		}
	}
	pb.mu.RUnlock()
	if price == 0 {
		return 0, 0, false
	}
	return price, time.Since(time.UnixMilli(ts)), true
}

// GetAllSymbols returns a list of all symbols in the price book.
func (pb *PriceBook) GetAllSymbols() []string {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	symbols := make([]string, 0, len(pb.quotes))
	for symbol := range pb.quotes {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// IsStale returns true if the quote is older than the given duration.
func (pb *PriceBook) IsStale(symbol string, maxAge time.Duration) bool {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return true
	}

	age := time.Since(time.UnixMilli(quote.Timestamp))
	return age > maxAge
}

// GetExitPrice returns the appropriate mark-to-market price for unrealized P&L calculation.
// For LONG positions (BUY side), use Bid price (you would sell at bid to exit).
// For SHORT positions (SELL side), use Ask price (you would buy at ask to exit).
func (pb *PriceBook) GetExitPrice(symbol string, positionSide contracts.OrderSide) (float64, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return 0, false
	}

	if positionSide == contracts.OrderSideBuy { // LONG position - exit at bid
		if quote.Bid > 0 {
			return quote.Bid, true
		}
		// Fall back to last price if bid not available
		return quote.Last, quote.Last > 0
	}

	// SHORT position - exit at ask
	if quote.Ask > 0 {
		return quote.Ask, true
	}
	// Fall back to last price if ask not available
	return quote.Last, quote.Last > 0
}

// GetFillPrice returns the appropriate fill price for a given side.
// For BUY orders, use Ask price. For SELL orders, use Bid price.
func (pb *PriceBook) GetFillPrice(symbol string, side contracts.OrderSide) (float64, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return 0, false
	}

	if side == contracts.OrderSideBuy {
		if quote.Ask > 0 {
			return quote.Ask, true
		}
		// Fall back to last price if ask not available
		return quote.Last, quote.Last > 0
	}

	// SELL
	if quote.Bid > 0 {
		return quote.Bid, true
	}
	// Fall back to last price if bid not available
	return quote.Last, quote.Last > 0
}

// GetLast returns the last traded price for a symbol.
func (pb *PriceBook) GetLast(symbol string) (float64, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists || quote.Last <= 0 {
		return 0, false
	}
	return quote.Last, true
}

// GetPriceAge returns the age of the price for a symbol.
// Returns the duration since the last price update and whether the quote exists.
func (pb *PriceBook) GetPriceAge(symbol string) (time.Duration, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return 0, false
	}
	return time.Since(time.UnixMilli(quote.Timestamp)), true
}

// GetFillPriceWithAge returns the appropriate fill price for a given side along with the price age.
// For BUY orders, use Ask price. For SELL orders, use Bid price.
func (pb *PriceBook) GetFillPriceWithAge(symbol string, side contracts.OrderSide) (float64, time.Duration, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return 0, 0, false
	}

	age := time.Since(time.UnixMilli(quote.Timestamp))

	if side == contracts.OrderSideBuy {
		if quote.Ask > 0 {
			return quote.Ask, age, true
		}
		// Fall back to last price if ask not available
		if quote.Last > 0 {
			return quote.Last, age, true
		}
		return 0, 0, false
	}

	// SELL
	if quote.Bid > 0 {
		return quote.Bid, age, true
	}
	// Fall back to last price if bid not available
	if quote.Last > 0 {
		return quote.Last, age, true
	}
	return 0, 0, false
}

// GetPrice returns the appropriate market price for closing a position on the given side.
// For "long" positions (BUY side), returns Bid (sell at bid to exit).
// For "short" positions (SELL side), returns Ask (buy at ask to exit).
// Falls back to Last price if bid/ask is unavailable.
func (pb *PriceBook) GetPrice(symbol string, positionSide string) (float64, bool) {
	quote, exists := pb.GetQuote(symbol)
	if !exists {
		return 0, false
	}

	if positionSide == "long" {
		if quote.Bid > 0 {
			return quote.Bid, true
		}
		return quote.Last, quote.Last > 0
	}

	// short position - exit at ask
	if quote.Ask > 0 {
		return quote.Ask, true
	}
	return quote.Last, quote.Last > 0
}
