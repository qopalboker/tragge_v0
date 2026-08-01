-- Revert: rename provider_symbol_massive back to provider_symbol_finnhub
ALTER TABLE contest_symbols
  RENAME COLUMN provider_symbol_massive TO provider_symbol_finnhub;

ALTER TABLE symbols
  RENAME COLUMN provider_symbol_massive TO provider_symbol_finnhub;

COMMENT ON COLUMN symbols.provider_symbol_finnhub IS 'Symbol mapping for Finnhub provider';
