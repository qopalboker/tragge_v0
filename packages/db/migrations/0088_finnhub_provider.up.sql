-- Add Finnhub provider symbol column
ALTER TABLE symbols ADD COLUMN IF NOT EXISTS provider_symbol_finnhub VARCHAR(50);

-- Populate Finnhub symbols for existing forex/commodity pairs
-- Finnhub format: OANDA:BASE_QUOTE (underscore separator)
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:EUR_USD' WHERE symbol = 'EUR/USD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:GBP_USD' WHERE symbol = 'GBP/USD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:USD_JPY' WHERE symbol = 'USD/JPY';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:USD_CHF' WHERE symbol = 'USD/CHF';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:AUD_USD' WHERE symbol = 'AUD/USD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:USD_CAD' WHERE symbol = 'USD/CAD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:NZD_USD' WHERE symbol = 'NZD/USD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:EUR_GBP' WHERE symbol = 'EUR/GBP';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:EUR_JPY' WHERE symbol = 'EUR/JPY';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:GBP_JPY' WHERE symbol = 'GBP/JPY';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:EUR_CHF' WHERE symbol = 'EUR/CHF';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:EUR_AUD' WHERE symbol = 'EUR/AUD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:GBP_CHF' WHERE symbol = 'GBP/CHF';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:GBP_AUD' WHERE symbol = 'GBP/AUD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:AUD_JPY' WHERE symbol = 'AUD/JPY';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:CAD_JPY' WHERE symbol = 'CAD/JPY';
-- Commodities
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:XAU_USD' WHERE symbol = 'XAU/USD';
UPDATE symbols SET provider_symbol_finnhub = 'OANDA:XAG_USD' WHERE symbol = 'XAG/USD';

-- Index
CREATE INDEX IF NOT EXISTS idx_symbols_finnhub ON symbols (provider_symbol_finnhub) WHERE provider_symbol_finnhub IS NOT NULL;
