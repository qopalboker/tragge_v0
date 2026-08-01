DROP INDEX IF EXISTS idx_symbols_finnhub;
ALTER TABLE symbols DROP COLUMN IF EXISTS provider_symbol_finnhub;
