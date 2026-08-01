-- Migration: 0051_nobitex_provider (rollback)
-- Description: Remove Nobitex provider column and index

DROP INDEX IF EXISTS idx_symbols_nobitex;

ALTER TABLE symbols DROP COLUMN IF EXISTS provider_symbol_nobitex;
