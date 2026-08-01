export interface SymbolMetadata {
  /** Number of decimal places for price display */
  decimals: number;
  /** Base currency/asset (e.g., "EUR" for EUR/USD) */
  base: string;
  /** Quote currency (e.g., "USD" for EUR/USD) */
  quote: string;
  /** Trading session time info (e.g., "24/7" for crypto, "Mon-Fri" for forex) */
  sessionTime?: string;
}

/** Default decimals for known asset classes */
const CRYPTO_SYMBOLS = new Set([
  'BTC/USD', 'ETH/USD', 'BNB/USD', 'SOL/USD', 'XRP/USD',
  'ADA/USD', 'DOGE/USD', 'DOT/USD', 'AVAX/USD', 'MATIC/USD',
  'LINK/USD', 'UNI/USD', 'ATOM/USD', 'LTC/USD', 'BCH/USD',
  'NEAR/USD', 'FIL/USD', 'APT/USD', 'ARB/USD', 'OP/USD',
  'SHIB/USD', 'TRX/USD', 'ETC/USD', 'AAVE/USD',
]);

/** JPY pairs typically have 3 decimals; most forex pairs use 5 */
const JPY_PAIRS = new Set([
  'USD/JPY', 'EUR/JPY', 'GBP/JPY', 'AUD/JPY', 'NZD/JPY',
  'CAD/JPY', 'CHF/JPY',
]);

/** Metals and commodities */
const METALS = new Set(['XAU/USD', 'XAG/USD']);

function parseSymbolParts(symbol: string): { base: string; quote: string } {
  const sep = symbol.indexOf('/');
  if (sep > 0) {
    return { base: symbol.slice(0, sep), quote: symbol.slice(sep + 1) };
  }
  // Fallback: treat first 3 chars as base
  return { base: symbol.slice(0, 3), quote: symbol.slice(3) || 'USD' };
}

function getDecimals(symbol: string): number {
  if (CRYPTO_SYMBOLS.has(symbol)) {
    // BTC/ETH need 2, smaller coins may need more
    if (symbol === 'BTC/USD') return 2;
    if (symbol === 'ETH/USD') return 2;
    if (symbol === 'SHIB/USD') return 8;
    if (symbol === 'DOGE/USD') return 6;
    if (symbol === 'TRX/USD') return 6;
    if (symbol === 'XRP/USD') return 4;
    return 4;
  }
  if (JPY_PAIRS.has(symbol)) return 3;
  if (METALS.has(symbol)) {
    return symbol === 'XAU/USD' ? 2 : 4;
  }
  // Default forex: 5 decimals (pipette precision)
  return 5;
}

/**
 * Returns display metadata for a trading symbol.
 */
export function getSymbolMetadata(symbol: string): SymbolMetadata {
  const { base, quote } = parseSymbolParts(symbol);
  return {
    decimals: getDecimals(symbol),
    base,
    quote,
  };
}
