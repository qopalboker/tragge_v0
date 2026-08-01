-- Revert: 0077_binance_provider
DROP INDEX IF EXISTS idx_symbols_binance;
ALTER TABLE symbols DROP COLUMN IF EXISTS provider_symbol_binance;
DROP TABLE IF EXISTS provider_config;
