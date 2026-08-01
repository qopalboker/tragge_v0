-- 0005_candles.down.sql
-- Rollback candles table

DROP INDEX IF EXISTS idx_candles_lookup;
DROP INDEX IF EXISTS idx_candles_time;
DROP INDEX IF EXISTS idx_candles_symbol_resolution_time;
DROP TABLE IF EXISTS candles;
