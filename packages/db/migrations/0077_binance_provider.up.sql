-- Migration: 0077_binance_provider
-- Description: Add Binance provider column to symbols table and provider_config table
-- for runtime crypto/forex provider selection.

-- PART 1: Add Binance provider column to symbols table
ALTER TABLE symbols ADD COLUMN IF NOT EXISTS provider_symbol_binance VARCHAR(50);

-- PART 2: Create provider_config table for runtime provider selection
CREATE TABLE IF NOT EXISTS provider_config (
    asset_class VARCHAR(20) PRIMARY KEY,  -- 'crypto', 'forex'
    active_provider VARCHAR(30) NOT NULL DEFAULT 'nobitex',
    fallback_provider VARCHAR(30),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by UUID
);

INSERT INTO provider_config (asset_class, active_provider, fallback_provider)
VALUES
    ('crypto', 'nobitex', 'binance'),
    ('forex', 'massive', 'twelvedata')
ON CONFLICT DO NOTHING;

-- PART 3: Populate Binance symbols for all crypto pairs
UPDATE symbols SET provider_symbol_binance = 'BTCUSDT'   WHERE symbol = 'BTC/USD';
UPDATE symbols SET provider_symbol_binance = 'ETHUSDT'   WHERE symbol = 'ETH/USD';
UPDATE symbols SET provider_symbol_binance = 'SOLUSDT'   WHERE symbol = 'SOL/USD';
UPDATE symbols SET provider_symbol_binance = 'DOGEUSDT'  WHERE symbol = 'DOGE/USD';
UPDATE symbols SET provider_symbol_binance = 'XRPUSDT'   WHERE symbol = 'XRP/USD';
UPDATE symbols SET provider_symbol_binance = 'ADAUSDT'   WHERE symbol = 'ADA/USD';
UPDATE symbols SET provider_symbol_binance = 'AVAXUSDT'  WHERE symbol = 'AVAX/USD';
UPDATE symbols SET provider_symbol_binance = 'LINKUSDT'  WHERE symbol = 'LINK/USD';
UPDATE symbols SET provider_symbol_binance = 'DOTUSDT'   WHERE symbol = 'DOT/USD';
UPDATE symbols SET provider_symbol_binance = 'POLUSDT'   WHERE symbol = 'POL/USD';
UPDATE symbols SET provider_symbol_binance = 'SHIBUSDT'  WHERE symbol = 'SHIB/USD';
UPDATE symbols SET provider_symbol_binance = 'LTCUSDT'   WHERE symbol = 'LTC/USD';
UPDATE symbols SET provider_symbol_binance = 'UNIUSDT'   WHERE symbol = 'UNI/USD';
UPDATE symbols SET provider_symbol_binance = 'XLMUSDT'   WHERE symbol = 'XLM/USD';
UPDATE symbols SET provider_symbol_binance = 'NEARUSDT'  WHERE symbol = 'NEAR/USD';
UPDATE symbols SET provider_symbol_binance = 'AAVEUSDT'  WHERE symbol = 'AAVE/USD';
UPDATE symbols SET provider_symbol_binance = 'SUIUSDT'   WHERE symbol = 'SUI/USD';
UPDATE symbols SET provider_symbol_binance = 'PEPEUSDT'  WHERE symbol = 'PEPE/USD';
UPDATE symbols SET provider_symbol_binance = 'APTUSDT'   WHERE symbol = 'APT/USD';
UPDATE symbols SET provider_symbol_binance = 'BCHUSDT'   WHERE symbol = 'BCH/USD';
UPDATE symbols SET provider_symbol_binance = 'CROUSDT'   WHERE symbol = 'CRO/USD';
UPDATE symbols SET provider_symbol_binance = 'HBARUSDT'  WHERE symbol = 'HBAR/USD';
UPDATE symbols SET provider_symbol_binance = 'ICPUSDT'   WHERE symbol = 'ICP/USD';
UPDATE symbols SET provider_symbol_binance = 'VETUSDT'   WHERE symbol = 'VET/USD';

-- PART 4: Index for Binance provider lookups
CREATE INDEX IF NOT EXISTS idx_symbols_binance
    ON symbols (provider_symbol_binance)
    WHERE provider_symbol_binance IS NOT NULL;
