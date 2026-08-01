package server

import (
	"sort"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// ====================
// Price Book
// ====================

// PriceBook maintains the latest prices for each symbol
type PriceBook struct {
	mu     sync.RWMutex
	prices map[string]contracts.SymbolTick
}

// NewPriceBook creates a new PriceBook
func NewPriceBook() *PriceBook {
	return &PriceBook{
		prices: make(map[string]contracts.SymbolTick),
	}
}

// Update updates the price for a symbol
func (pb *PriceBook) Update(tick contracts.SymbolTick) {
	pb.mu.Lock()
	pb.prices[tick.Symbol] = tick
	pb.mu.Unlock()
}

// UpdateBatch updates prices from a tick snapshot
func (pb *PriceBook) UpdateBatch(snapshot *contracts.TickSnapshot) {
	pb.mu.Lock()
	for _, tick := range snapshot.Symbols {
		pb.prices[tick.Symbol] = tick
	}
	pb.mu.Unlock()
}

// GetSnapshot returns a TickSnapshot with up to maxSymbols
func (pb *PriceBook) GetSnapshot(maxSymbols int) *contracts.TickSnapshot {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	// Collect symbols and sort for deterministic ordering
	symbols := make([]string, 0, len(pb.prices))
	for sym := range pb.prices {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	// Limit to maxSymbols
	if len(symbols) > maxSymbols {
		symbols = symbols[:maxSymbols]
	}

	// Build snapshot
	ticks := make([]contracts.SymbolTick, len(symbols))
	for i, sym := range symbols {
		ticks[i] = pb.prices[sym]
	}

	return &contracts.TickSnapshot{
		Ts:      time.Now().UnixMilli(),
		Symbols: ticks,
	}
}
