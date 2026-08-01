-- Migration: 0076_update_crypto_symbols
-- Description: Add 5 new crypto symbols (BCH, CRO, HBAR, ICP, VET) and soft-disable 5 removed symbols

-- ============================================================================
-- PART 1: Add new crypto symbols with Nobitex USDT pair mappings
-- ============================================================================

INSERT INTO symbols (symbol, name, asset_type, provider_symbol_nobitex, provider_symbol_massive, provider_symbol_twelvedata, is_active) VALUES
    ('BCH/USD',  'Bitcoin Cash',       'crypto', 'BCHUSDT',  'BCH-USD',  'BCH/USD',  TRUE),
    ('CRO/USD',  'Cronos',             'crypto', 'CROUSDT',  'CRO-USD',  'CRO/USD',  TRUE),
    ('HBAR/USD', 'Hedera',             'crypto', 'HBARUSDT', 'HBAR-USD', 'HBAR/USD', TRUE),
    ('ICP/USD',  'Internet Computer',  'crypto', 'ICPUSDT',  'ICP-USD',  'ICP/USD',  TRUE),
    ('VET/USD',  'VeChain',            'crypto', 'VETUSDT',  'VET-USD',  'VET/USD',  TRUE)
ON CONFLICT (symbol) DO UPDATE SET
    provider_symbol_nobitex = EXCLUDED.provider_symbol_nobitex,
    provider_symbol_massive = EXCLUDED.provider_symbol_massive,
    provider_symbol_twelvedata = EXCLUDED.provider_symbol_twelvedata,
    is_active = TRUE,
    updated_at = NOW();

-- ============================================================================
-- PART 2: Soft-disable removed symbols (preserve for settlement history)
-- Only disable if no active contests reference them
-- ============================================================================

UPDATE symbols SET is_active = FALSE, updated_at = NOW()
WHERE symbol IN ('ETC/USD', 'ARB/USD', 'OP/USD', 'INJ/USD', 'RENDER/USD')
AND NOT EXISTS (
    SELECT 1 FROM contest_symbols cs
    JOIN contests c ON c.id = cs.contest_id
    WHERE c.status IN ('scheduled', 'registration_open', 'running')
    AND cs.symbol = symbols.symbol
);
