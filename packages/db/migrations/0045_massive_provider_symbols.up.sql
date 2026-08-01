-- Migration: 0045_massive_provider_symbols
-- Description: Seed all missing symbols with correct Massive and TwelveData mappings
-- After this migration the symbols table should contain 47+ tradable symbols:
--   20 forex, 24 crypto, 4 commodities (including USOIL alias)

-- ============================================================================
-- PART 1: Mark deprecated Finnhub column comment (already renamed in 0043)
-- ============================================================================
-- provider_symbol_finnhub was renamed to provider_symbol_massive in migration 0043
-- No additional column changes needed

-- ============================================================================
-- PART 2: Seed all missing symbols
-- ============================================================================

INSERT INTO symbols (symbol, name, asset_type, provider_symbol_twelvedata, provider_symbol_massive, is_active) VALUES
    -- Missing Forex (14 pairs)
    ('USD/CNY', 'US Dollar/Chinese Yuan', 'forex', 'USD/CNY', 'USD-CNY', true),
    ('USD/HKD', 'US Dollar/Hong Kong Dollar', 'forex', 'USD/HKD', 'USD-HKD', true),
    ('USD/SGD', 'US Dollar/Singapore Dollar', 'forex', 'USD/SGD', 'USD-SGD', true),
    ('USD/KRW', 'US Dollar/Korean Won', 'forex', 'USD/KRW', 'USD-KRW', true),
    ('USD/INR', 'US Dollar/Indian Rupee', 'forex', 'USD/INR', 'USD-INR', true),
    ('USD/MXN', 'US Dollar/Mexican Peso', 'forex', 'USD/MXN', 'USD-MXN', true),
    ('EUR/JPY', 'Euro/Japanese Yen', 'forex', 'EUR/JPY', 'EUR-JPY', true),
    ('EUR/GBP', 'Euro/British Pound', 'forex', 'EUR/GBP', 'EUR-GBP', true),
    ('NZD/USD', 'New Zealand Dollar/US Dollar', 'forex', 'NZD/USD', 'NZD-USD', true),
    ('USD/BRL', 'US Dollar/Brazilian Real', 'forex', 'USD/BRL', 'USD-BRL', true),
    ('USD/ZAR', 'US Dollar/South African Rand', 'forex', 'USD/ZAR', 'USD-ZAR', true),
    ('USD/PLN', 'US Dollar/Polish Zloty', 'forex', 'USD/PLN', 'USD-PLN', true),
    ('USD/SEK', 'US Dollar/Swedish Krona', 'forex', 'USD/SEK', 'USD-SEK', true),
    ('USD/NOK', 'US Dollar/Norwegian Krone', 'forex', 'USD/NOK', 'USD-NOK', true),

    -- USOIL commodity alias
    ('USOIL', 'US Oil (WTI Crude)', 'commodity', 'USOIL', NULL, true),

    -- Missing Crypto (18 coins)
    ('AAVE/USD', 'Aave', 'crypto', 'AAVE/USD', 'AAVE-USD', true),
    ('APT/USD', 'Aptos', 'crypto', 'APT/USD', 'APT-USD', true),
    ('AVAX/USD', 'Avalanche', 'crypto', 'AVAX/USD', 'AVAX-USD', true),
    ('BCH/USD', 'Bitcoin Cash', 'crypto', 'BCH/USD', 'BCH-USD', true),
    ('LINK/USD', 'Chainlink', 'crypto', 'LINK/USD', 'LINK-USD', true),
    ('CRO/USD', 'Cronos', 'crypto', 'CRO/USD', 'CRO-USD', true),
    ('HBAR/USD', 'Hedera', 'crypto', 'HBAR/USD', 'HBAR-USD', true),
    ('ICP/USD', 'Internet Computer', 'crypto', 'ICP/USD', 'ICP-USD', true),
    ('LTC/USD', 'Litecoin', 'crypto', 'LTC/USD', 'LTC-USD', true),
    ('NEAR/USD', 'NEAR Protocol', 'crypto', 'NEAR/USD', 'NEAR-USD', true),
    ('PEPE/USD', 'Pepe', 'crypto', 'PEPE/USD', 'PEPE-USD', true),
    ('DOT/USD', 'Polkadot', 'crypto', 'DOT/USD', 'DOT-USD', true),
    ('SHIB/USD', 'Shiba Inu', 'crypto', 'SHIB/USD', 'SHIB-USD', true),
    ('XLM/USD', 'Stellar', 'crypto', 'XLM/USD', 'XLM-USD', true),
    ('SUI/USD', 'Sui', 'crypto', 'SUI/USD', 'SUI-USD', true),
    ('UNI/USD', 'Uniswap', 'crypto', 'UNI/USD', 'UNI-USD', true),
    ('VET/USD', 'VeChain', 'crypto', 'VET/USD', 'VET-USD', true),
    ('POL/USD', 'Polygon', 'crypto', 'POL/USD', 'POL-USD', true)
ON CONFLICT (symbol) DO NOTHING;

-- ============================================================================
-- PART 3: Ensure existing symbols have correct provider_symbol_massive values
-- (Migration 0044 already set most of these, but ensure completeness)
-- ============================================================================

-- Forex
UPDATE symbols SET provider_symbol_massive = 'EUR-USD' WHERE symbol = 'EUR/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'EUR-USD');
UPDATE symbols SET provider_symbol_massive = 'GBP-USD' WHERE symbol = 'GBP/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'GBP-USD');
UPDATE symbols SET provider_symbol_massive = 'USD-JPY' WHERE symbol = 'USD/JPY' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'USD-JPY');
UPDATE symbols SET provider_symbol_massive = 'USD-CHF' WHERE symbol = 'USD/CHF' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'USD-CHF');
UPDATE symbols SET provider_symbol_massive = 'AUD-USD' WHERE symbol = 'AUD/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'AUD-USD');
UPDATE symbols SET provider_symbol_massive = 'USD-CAD' WHERE symbol = 'USD/CAD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'USD-CAD');

-- Crypto
UPDATE symbols SET provider_symbol_massive = 'BTC-USD' WHERE symbol = 'BTC/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'BTC-USD');
UPDATE symbols SET provider_symbol_massive = 'ETH-USD' WHERE symbol = 'ETH/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'ETH-USD');
UPDATE symbols SET provider_symbol_massive = 'SOL-USD' WHERE symbol = 'SOL/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'SOL-USD');
UPDATE symbols SET provider_symbol_massive = 'DOGE-USD' WHERE symbol = 'DOGE/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'DOGE-USD');
UPDATE symbols SET provider_symbol_massive = 'XRP-USD' WHERE symbol = 'XRP/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'XRP-USD');
UPDATE symbols SET provider_symbol_massive = 'ADA-USD' WHERE symbol = 'ADA/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'ADA-USD');

-- Commodities
UPDATE symbols SET provider_symbol_massive = 'XAU-USD' WHERE symbol = 'XAU/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'XAU-USD');
UPDATE symbols SET provider_symbol_massive = 'XAG-USD' WHERE symbol = 'XAG/USD' AND (provider_symbol_massive IS NULL OR provider_symbol_massive != 'XAG-USD');
