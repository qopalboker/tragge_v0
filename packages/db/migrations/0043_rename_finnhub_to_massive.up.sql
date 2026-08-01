-- Rename provider_symbol_finnhub to provider_symbol_massive in contest_symbols
ALTER TABLE contest_symbols
  RENAME COLUMN provider_symbol_finnhub TO provider_symbol_massive;

-- Rename provider_symbol_finnhub to provider_symbol_massive in symbols
ALTER TABLE symbols
  RENAME COLUMN provider_symbol_finnhub TO provider_symbol_massive;

-- Update column comment
COMMENT ON COLUMN symbols.provider_symbol_massive IS 'Symbol mapping for Massive provider';
