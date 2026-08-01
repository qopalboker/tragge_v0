/**
 * Tick data for a single symbol.
 */
export interface SymbolTick {
  symbol: string;
  bid: number;
  ask: number;
  last: number;
  volume?: number;
}

/**
 * Market tick snapshot containing bid/ask/last prices for symbols.
 */
export interface TickSnapshot {
  ts: number;
  symbols: SymbolTick[];
}
