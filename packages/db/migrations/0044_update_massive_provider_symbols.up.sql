-- Migration: 0044_update_massive_provider_symbols
-- Description: Update provider_symbol_massive values from old Finnhub format to Massive format
-- Massive uses dash-separated pairs (e.g., EUR-USD, BTC-USD)

-- Crypto: BINANCE:BTCUSDT → BTC-USD, etc.
UPDATE symbols SET provider_symbol_massive = 'BTC-USD' WHERE symbol = 'BTC/USD';
UPDATE symbols SET provider_symbol_massive = 'ETH-USD' WHERE symbol = 'ETH/USD';
UPDATE symbols SET provider_symbol_massive = 'SOL-USD' WHERE symbol = 'SOL/USD';
UPDATE symbols SET provider_symbol_massive = 'DOGE-USD' WHERE symbol = 'DOGE/USD';
UPDATE symbols SET provider_symbol_massive = 'XRP-USD' WHERE symbol = 'XRP/USD';
UPDATE symbols SET provider_symbol_massive = 'ADA-USD' WHERE symbol = 'ADA/USD';

-- Forex: OANDA:EUR_USD → EUR-USD, etc.
UPDATE symbols SET provider_symbol_massive = 'EUR-USD' WHERE symbol = 'EUR/USD';
UPDATE symbols SET provider_symbol_massive = 'GBP-USD' WHERE symbol = 'GBP/USD';
UPDATE symbols SET provider_symbol_massive = 'USD-JPY' WHERE symbol = 'USD/JPY';
UPDATE symbols SET provider_symbol_massive = 'USD-CHF' WHERE symbol = 'USD/CHF';
UPDATE symbols SET provider_symbol_massive = 'AUD-USD' WHERE symbol = 'AUD/USD';
UPDATE symbols SET provider_symbol_massive = 'USD-CAD' WHERE symbol = 'USD/CAD';

-- Commodities: OANDA:XAU_USD → XAU-USD, etc.
UPDATE symbols SET provider_symbol_massive = 'XAU-USD' WHERE symbol = 'XAU/USD';
UPDATE symbols SET provider_symbol_massive = 'XAG-USD' WHERE symbol = 'XAG/USD';
-- BRENT and WTI don't have Massive equivalents (stocks-like)
UPDATE symbols SET provider_symbol_massive = NULL WHERE symbol IN ('BRENT', 'WTI');

-- Stocks: Massive uses same symbol as canonical for stocks
-- (AAPL stays AAPL, etc.) - already correct from Finnhub mapping
