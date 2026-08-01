package server

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// SymbolRegistry holds canonical symbols and provider-specific mappings loaded from the database.
type SymbolRegistry struct {
	// CanonicalSymbols is the list of all active canonical symbols (e.g., "EUR/USD", "BTC/USD").
	CanonicalSymbols []string

	// ToMassive maps canonical symbol → Massive provider symbol (e.g., "EUR/USD" → "EUR-USD").
	ToMassive map[string]string
	// FromMassive maps Massive provider symbol → canonical symbol (e.g., "EUR-USD" → "EUR/USD").
	FromMassive map[string]string

	// ToTwelveData maps canonical symbol → TwelveData provider symbol (e.g., "EUR/USD" → "EUR/USD").
	ToTwelveData map[string]string
	// FromTwelveData maps TwelveData provider symbol → canonical symbol.
	FromTwelveData map[string]string

	// ToNobitex maps canonical symbol → Nobitex provider symbol (e.g., "BTC/USD" → "BTCUSDT").
	ToNobitex map[string]string
	// FromNobitex maps Nobitex provider symbol → canonical symbol (e.g., "BTCUSDT" → "BTC/USD").
	FromNobitex map[string]string

	// ToBinance maps canonical symbol → Binance provider symbol (e.g., "BTC/USD" → "BTCUSDT").
	ToBinance map[string]string
	// FromBinance maps Binance provider symbol → canonical symbol (e.g., "BTCUSDT" → "BTC/USD").
	FromBinance map[string]string

	// ToFinnhub maps canonical symbol → Finnhub provider symbol (e.g., "EUR/USD" → "OANDA:EUR_USD").
	ToFinnhub map[string]string
	// FromFinnhub maps Finnhub provider symbol → canonical symbol.
	FromFinnhub map[string]string

	// AssetTypes maps canonical symbol → asset type (e.g., "EUR/USD" → "forex").
	AssetTypes map[string]string

	// ForexSymbols lists canonical symbols classified as forex (routed to Massive forex WS).
	ForexSymbols []string
	// CryptoSymbols lists canonical symbols classified as crypto (routed to Massive crypto WS).
	CryptoSymbols []string
}

// loadSymbolsFromDB queries the symbols table for active symbols and builds a SymbolRegistry.
func loadSymbolsFromDB(db *sql.DB) (*SymbolRegistry, error) {
	rows, err := db.Query(`
		SELECT symbol, provider_symbol_massive, provider_symbol_twelvedata,
		       provider_symbol_nobitex, provider_symbol_binance,
		       provider_symbol_finnhub, asset_type, is_active
		FROM symbols
		WHERE is_active = TRUE
	`)
	if err != nil {
		return nil, fmt.Errorf("query symbols: %w", err)
	}
	defer rows.Close()

	reg := &SymbolRegistry{
		ToMassive:      make(map[string]string),
		FromMassive:    make(map[string]string),
		ToTwelveData:   make(map[string]string),
		FromTwelveData: make(map[string]string),
		ToNobitex:      make(map[string]string),
		FromNobitex:    make(map[string]string),
		ToBinance:      make(map[string]string),
		FromBinance:    make(map[string]string),
		ToFinnhub:      make(map[string]string),
		FromFinnhub:    make(map[string]string),
		AssetTypes:     make(map[string]string),
	}

	for rows.Next() {
		var (
			symbol        string
			massiveSym    sql.NullString
			twelveDataSym sql.NullString
			nobitexSym    sql.NullString
			binanceSym    sql.NullString
			finnhubSym    sql.NullString
			assetType     string
			isActive      bool
		)
		if err := rows.Scan(&symbol, &massiveSym, &twelveDataSym, &nobitexSym, &binanceSym, &finnhubSym, &assetType, &isActive); err != nil {
			return nil, fmt.Errorf("scan symbol row: %w", err)
		}

		reg.CanonicalSymbols = append(reg.CanonicalSymbols, symbol)
		reg.AssetTypes[symbol] = assetType

		// Build Massive mappings
		if massiveSym.Valid && massiveSym.String != "" {
			reg.ToMassive[symbol] = massiveSym.String
			reg.FromMassive[massiveSym.String] = symbol
		}

		// Build TwelveData mappings
		if twelveDataSym.Valid && twelveDataSym.String != "" {
			reg.ToTwelveData[symbol] = twelveDataSym.String
			reg.FromTwelveData[twelveDataSym.String] = symbol
		}

		// Build Nobitex mappings
		if nobitexSym.Valid && nobitexSym.String != "" {
			reg.ToNobitex[symbol] = nobitexSym.String
			reg.FromNobitex[nobitexSym.String] = symbol
		}

		// Build Binance mappings
		if binanceSym.Valid && binanceSym.String != "" {
			reg.ToBinance[symbol] = binanceSym.String
			reg.FromBinance[binanceSym.String] = symbol
		}

		// Build Finnhub mappings
		if finnhubSym.Valid && finnhubSym.String != "" {
			reg.ToFinnhub[symbol] = finnhubSym.String
			reg.FromFinnhub[finnhubSym.String] = symbol
		}

		// Classify for Massive WS routing
		switch assetType {
		case "forex", "commodity":
			reg.ForexSymbols = append(reg.ForexSymbols, symbol)
		case "crypto":
			reg.CryptoSymbols = append(reg.CryptoSymbols, symbol)
		}
		// stocks are not currently routed through Massive WS
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate symbols: %w", err)
	}

	if len(reg.CanonicalSymbols) == 0 {
		return nil, fmt.Errorf("no active symbols found in database")
	}

	log.Printf("Loaded %d symbols from DB (%d forex, %d crypto)",
		len(reg.CanonicalSymbols), len(reg.ForexSymbols), len(reg.CryptoSymbols))

	return reg, nil
}

// buildRegistryFromEnv creates a SymbolRegistry from a list of symbols (env var fallback).
// It applies the same heuristic-based classification as the existing code.
func buildRegistryFromEnv(symbols []string) *SymbolRegistry {
	reg := &SymbolRegistry{
		ToMassive:      make(map[string]string),
		FromMassive:    make(map[string]string),
		ToTwelveData:   make(map[string]string),
		FromTwelveData: make(map[string]string),
		ToNobitex:      make(map[string]string),
		FromNobitex:    make(map[string]string),
		ToBinance:      make(map[string]string),
		FromBinance:    make(map[string]string),
		ToFinnhub:      make(map[string]string),
		FromFinnhub:    make(map[string]string),
		AssetTypes:     make(map[string]string),
	}

	for _, sym := range symbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		reg.CanonicalSymbols = append(reg.CanonicalSymbols, sym)

		// For env var fallback, build mappings using heuristics
		if strings.Contains(sym, "/") {
			// Slash-separated pair: e.g., EUR/USD, BTC/USD
			massiveSym := strings.ReplaceAll(sym, "/", "-")
			reg.ToMassive[sym] = massiveSym
			reg.FromMassive[massiveSym] = sym

			// TwelveData uses canonical format for pairs
			reg.ToTwelveData[sym] = sym
			reg.FromTwelveData[sym] = sym

			// Classify using the existing isForexSymbol heuristic
			if isForexSymbol(sym) {
				reg.AssetTypes[sym] = "forex"
				reg.ForexSymbols = append(reg.ForexSymbols, sym)

				// Finnhub forex format: OANDA:BASE_QUOTE
				parts := strings.SplitN(sym, "/", 2)
				if len(parts) == 2 {
					finnhubSym := "OANDA:" + parts[0] + "_" + parts[1]
					reg.ToFinnhub[sym] = finnhubSym
					reg.FromFinnhub[finnhubSym] = sym
				}
			} else {
				reg.AssetTypes[sym] = "crypto"
				reg.CryptoSymbols = append(reg.CryptoSymbols, sym)

				// Build Nobitex and Binance mappings for crypto (e.g., BTC/USD → BTCUSDT)
				if strings.Contains(sym, "/") {
					parts := strings.SplitN(sym, "/", 2)
					if len(parts) == 2 {
						base := strings.ToUpper(parts[0])
						quote := strings.ToUpper(parts[1])
						var pairSym string
						if quote == "USD" {
							pairSym = base + "USDT"
						}
						if pairSym != "" {
							reg.ToNobitex[sym] = pairSym
							reg.FromNobitex[pairSym] = sym
							reg.ToBinance[sym] = pairSym
							reg.FromBinance[pairSym] = sym
						}
					}
				}
			}
		} else {
			// No slash: stock symbol (e.g., AAPL)
			reg.AssetTypes[sym] = "stock"
			// TwelveData uses the symbol as-is
			reg.ToTwelveData[sym] = sym
			reg.FromTwelveData[sym] = sym
			// Massive uses the symbol as-is for stocks
			reg.ToMassive[sym] = sym
			reg.FromMassive[sym] = sym
		}
	}

	log.Printf("Built symbol registry from env var (%d symbols: %d forex, %d crypto)",
		len(reg.CanonicalSymbols), len(reg.ForexSymbols), len(reg.CryptoSymbols))

	return reg
}

// filterRegistry returns a new SymbolRegistry containing only symbols present in the filter list.
// This allows the SYMBOLS env var to narrow down the DB-loaded symbols.
func filterRegistry(reg *SymbolRegistry, filter []string) *SymbolRegistry {
	allowed := make(map[string]bool, len(filter))
	for _, s := range filter {
		allowed[strings.TrimSpace(s)] = true
	}

	filtered := &SymbolRegistry{
		ToMassive:      make(map[string]string),
		FromMassive:    make(map[string]string),
		ToTwelveData:   make(map[string]string),
		FromTwelveData: make(map[string]string),
		ToNobitex:      make(map[string]string),
		FromNobitex:    make(map[string]string),
		ToBinance:      make(map[string]string),
		FromBinance:    make(map[string]string),
		ToFinnhub:      make(map[string]string),
		FromFinnhub:    make(map[string]string),
		AssetTypes:     make(map[string]string),
	}

	for _, sym := range reg.CanonicalSymbols {
		if !allowed[sym] {
			continue
		}
		filtered.CanonicalSymbols = append(filtered.CanonicalSymbols, sym)
		filtered.AssetTypes[sym] = reg.AssetTypes[sym]

		if v, ok := reg.ToMassive[sym]; ok {
			filtered.ToMassive[sym] = v
			filtered.FromMassive[v] = sym
		}
		if v, ok := reg.ToTwelveData[sym]; ok {
			filtered.ToTwelveData[sym] = v
			filtered.FromTwelveData[v] = sym
		}
		if v, ok := reg.ToNobitex[sym]; ok {
			filtered.ToNobitex[sym] = v
			filtered.FromNobitex[v] = sym
		}
		if v, ok := reg.ToBinance[sym]; ok {
			filtered.ToBinance[sym] = v
			filtered.FromBinance[v] = sym
		}
		if v, ok := reg.ToFinnhub[sym]; ok {
			filtered.ToFinnhub[sym] = v
			filtered.FromFinnhub[v] = sym
		}

		switch reg.AssetTypes[sym] {
		case "forex", "commodity":
			filtered.ForexSymbols = append(filtered.ForexSymbols, sym)
		case "crypto":
			filtered.CryptoSymbols = append(filtered.CryptoSymbols, sym)
		}
	}

	return filtered
}

// MassiveSubscribeSymbol returns the full Massive subscription string for a canonical symbol.
// Forex: "C.EUR-USD", Crypto: "XQ.BTC-USD"
func (r *SymbolRegistry) MassiveSubscribeSymbol(canonical string) string {
	massiveSym, ok := r.ToMassive[canonical]
	if !ok {
		// Fallback to heuristic conversion
		massiveSym = strings.ReplaceAll(canonical, "/", "-")
	}

	assetType := r.AssetTypes[canonical]
	switch assetType {
	case "forex", "commodity":
		return "C." + massiveSym
	case "crypto":
		return "XQ." + massiveSym
	default:
		// Default to forex prefix for unknown
		return "C." + massiveSym
	}
}

// MassiveToCanonical converts a Massive pair (e.g., "EUR-USD") back to canonical format.
// It first checks the registry mapping, then falls back to heuristic conversion.
func (r *SymbolRegistry) MassiveToCanonical(massivePair string) string {
	if canonical, ok := r.FromMassive[massivePair]; ok {
		return canonical
	}
	// Fallback to heuristic
	return strings.ReplaceAll(massivePair, "-", "/")
}

// TwelveDataToCanonical converts a TwelveData symbol back to canonical format.
func (r *SymbolRegistry) TwelveDataToCanonical(tdSymbol string) string {
	if canonical, ok := r.FromTwelveData[tdSymbol]; ok {
		return canonical
	}
	// TwelveData symbols are usually already canonical
	return tdSymbol
}

// MassiveForexSubscriptions returns the list of Massive subscription strings for the forex WS.
func (r *SymbolRegistry) MassiveForexSubscriptions() []string {
	var subs []string
	for _, sym := range r.ForexSymbols {
		subs = append(subs, r.MassiveSubscribeSymbol(sym))
	}
	return subs
}

// MassiveCryptoSubscriptions returns the list of Massive subscription strings for the crypto WS.
func (r *SymbolRegistry) MassiveCryptoSubscriptions() []string {
	var subs []string
	for _, sym := range r.CryptoSymbols {
		subs = append(subs, r.MassiveSubscribeSymbol(sym))
	}
	return subs
}

// TwelveDataSubscriptions returns the list of TwelveData symbols to subscribe to.
func (r *SymbolRegistry) TwelveDataSubscriptions() []string {
	var subs []string
	for _, sym := range r.CanonicalSymbols {
		if td, ok := r.ToTwelveData[sym]; ok {
			subs = append(subs, td)
		}
	}
	return subs
}

// NobitexSubscriptions returns the list of Nobitex USDT symbols for crypto.
func (r *SymbolRegistry) NobitexSubscriptions() []string {
	var subs []string
	for _, sym := range r.CryptoSymbols {
		if nb, ok := r.ToNobitex[sym]; ok {
			subs = append(subs, nb)
		}
	}
	return subs
}

// BinanceSubscriptions returns the list of Binance symbols for crypto (lowercase).
func (r *SymbolRegistry) BinanceSubscriptions() []string {
	var subs []string
	for _, sym := range r.CryptoSymbols {
		if bn, ok := r.ToBinance[sym]; ok {
			subs = append(subs, strings.ToLower(bn))
		}
	}
	return subs
}

// BinanceToCanonical converts a Binance symbol (e.g. "BTCUSDT") back to canonical format (e.g. "BTC/USD").
func (r *SymbolRegistry) BinanceToCanonical(bnSymbol string) string {
	if canonical, ok := r.FromBinance[strings.ToUpper(bnSymbol)]; ok {
		return canonical
	}
	// Fallback heuristic: strip USDT suffix and add /USD
	upper := strings.ToUpper(bnSymbol)
	if strings.HasSuffix(upper, "USDT") {
		base := strings.TrimSuffix(upper, "USDT")
		return base + "/USD"
	}
	return bnSymbol
}

// NobitexToCanonical converts a Nobitex symbol (e.g. "BTCUSDT") back to canonical format (e.g. "BTC/USD").
func (r *SymbolRegistry) NobitexToCanonical(nbSymbol string) string {
	if canonical, ok := r.FromNobitex[nbSymbol]; ok {
		return canonical
	}
	// Fallback heuristic: strip USDT suffix and add /USD
	upper := strings.ToUpper(nbSymbol)
	if strings.HasSuffix(upper, "USDT") {
		base := strings.TrimSuffix(upper, "USDT")
		return base + "/USD"
	}
	return nbSymbol
}

// FinnhubSubscriptions returns the list of Finnhub symbols to subscribe to (forex/commodity only).
func (r *SymbolRegistry) FinnhubSubscriptions() []string {
	var subs []string
	for _, sym := range r.ForexSymbols {
		if fh, ok := r.ToFinnhub[sym]; ok {
			subs = append(subs, fh)
		}
	}
	return subs
}

// FinnhubToCanonical converts a Finnhub symbol (e.g. "OANDA:EUR_USD") back to canonical format (e.g. "EUR/USD").
func (r *SymbolRegistry) FinnhubToCanonical(finnhubSymbol string) string {
	if canonical, ok := r.FromFinnhub[finnhubSymbol]; ok {
		return canonical
	}
	// Fallback heuristic: strip OANDA: prefix and replace underscore with slash
	s := finnhubSymbol
	if strings.HasPrefix(s, "OANDA:") {
		s = strings.TrimPrefix(s, "OANDA:")
	}
	return strings.ReplaceAll(s, "_", "/")
}
