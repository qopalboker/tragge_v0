-- Migration: 0051_nobitex_provider
-- Description: Add Nobitex as a crypto price provider with USDT pair mappings

-- ============================================================================
-- PART 1: Add provider_symbol_nobitex column to symbols table
-- ============================================================================

ALTER TABLE symbols ADD COLUMN provider_symbol_nobitex VARCHAR(50);

-- ============================================================================
-- PART 2: Upsert all 24 crypto symbols with Nobitex USDT pair mappings
-- ============================================================================

INSERT INTO symbols (symbol, name, asset_type, provider_symbol_nobitex, provider_symbol_massive, is_active) VALUES
    ('BTC/USD',    'Bitcoin',        'crypto', 'BTCUSDT',    'BTC-USD',    TRUE),
    ('ETH/USD',    'Ethereum',       'crypto', 'ETHUSDT',    'ETH-USD',    TRUE),
    ('SOL/USD',    'Solana',         'crypto', 'SOLUSDT',    'SOL-USD',    TRUE),
    ('DOGE/USD',   'Dogecoin',       'crypto', 'DOGEUSDT',   'DOGE-USD',   TRUE),
    ('XRP/USD',    'Ripple',         'crypto', 'XRPUSDT',    'XRP-USD',    TRUE),
    ('ADA/USD',    'Cardano',        'crypto', 'ADAUSDT',    'ADA-USD',    TRUE),
    ('AVAX/USD',   'Avalanche',      'crypto', 'AVAXUSDT',   'AVAX-USD',   TRUE),
    ('LINK/USD',   'Chainlink',      'crypto', 'LINKUSDT',   'LINK-USD',   TRUE),
    ('DOT/USD',    'Polkadot',       'crypto', 'DOTUSDT',    'DOT-USD',    TRUE),
    ('POL/USD',    'Polygon',        'crypto', 'POLUSDT',    'POL-USD',    TRUE),
    ('SHIB/USD',   'Shiba Inu',      'crypto', 'SHIBUSDT',   'SHIB-USD',   TRUE),
    ('LTC/USD',    'Litecoin',       'crypto', 'LTCUSDT',    'LTC-USD',    TRUE),
    ('UNI/USD',    'Uniswap',        'crypto', 'UNIUSDT',    'UNI-USD',    TRUE),
    ('ETC/USD',    'Ethereum Classic','crypto', 'ETCUSDT',    'ETC-USD',    TRUE),
    ('XLM/USD',    'Stellar',        'crypto', 'XLMUSDT',    'XLM-USD',    TRUE),
    ('NEAR/USD',   'NEAR Protocol',  'crypto', 'NEARUSDT',   'NEAR-USD',   TRUE),
    ('AAVE/USD',   'Aave',           'crypto', 'AAVEUSDT',   'AAVE-USD',   TRUE),
    ('SUI/USD',    'Sui',            'crypto', 'SUIUSDT',    'SUI-USD',    TRUE),
    ('PEPE/USD',   'Pepe',           'crypto', 'PEPEUSDT',   'PEPE-USD',   TRUE),
    ('ARB/USD',    'Arbitrum',       'crypto', 'ARBUSDT',    'ARB-USD',    TRUE),
    ('OP/USD',     'Optimism',       'crypto', 'OPUSDT',     'OP-USD',     TRUE),
    ('APT/USD',    'Aptos',          'crypto', 'APTUSDT',    'APT-USD',    TRUE),
    ('INJ/USD',    'Injective',      'crypto', 'INJUSDT',    'INJ-USD',    TRUE),
    ('RENDER/USD', 'Render',         'crypto', 'RENDERUSDT', 'RENDER-USD', TRUE)
ON CONFLICT (symbol) DO UPDATE SET
    provider_symbol_nobitex = EXCLUDED.provider_symbol_nobitex,
    updated_at = NOW();

-- ============================================================================
-- PART 3: Create index for Nobitex provider symbol lookups
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_symbols_nobitex ON symbols (provider_symbol_nobitex) WHERE provider_symbol_nobitex IS NOT NULL;
