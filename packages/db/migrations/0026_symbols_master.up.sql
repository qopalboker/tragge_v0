-- Migration: 0026_symbols_master
-- Description: Create master symbols table for tradable assets management

-- Create asset_type enum
CREATE TYPE asset_type AS ENUM ('stock', 'crypto', 'forex', 'commodity');

-- Create master symbols table
CREATE TABLE symbols (
    symbol VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    asset_type asset_type NOT NULL,
    provider_symbol_twelvedata VARCHAR(50),
    provider_symbol_finnhub VARCHAR(50),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for common queries
CREATE INDEX idx_symbols_asset_type ON symbols(asset_type);
CREATE INDEX idx_symbols_is_active ON symbols(is_active);
CREATE INDEX idx_symbols_created_at ON symbols(created_at DESC);

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_symbols_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER symbols_updated_at_trigger
    BEFORE UPDATE ON symbols
    FOR EACH ROW
    EXECUTE FUNCTION update_symbols_updated_at();

-- Insert common default symbols
INSERT INTO symbols (symbol, name, asset_type, provider_symbol_twelvedata, provider_symbol_finnhub, is_active) VALUES
    -- Stocks
    ('AAPL', 'Apple Inc.', 'stock', 'AAPL', 'AAPL', true),
    ('MSFT', 'Microsoft Corporation', 'stock', 'MSFT', 'MSFT', true),
    ('GOOGL', 'Alphabet Inc.', 'stock', 'GOOGL', 'GOOGL', true),
    ('AMZN', 'Amazon.com Inc.', 'stock', 'AMZN', 'AMZN', true),
    ('TSLA', 'Tesla Inc.', 'stock', 'TSLA', 'TSLA', true),
    ('META', 'Meta Platforms Inc.', 'stock', 'META', 'META', true),
    ('NVDA', 'NVIDIA Corporation', 'stock', 'NVDA', 'NVDA', true),
    ('JPM', 'JPMorgan Chase & Co.', 'stock', 'JPM', 'JPM', true),
    ('V', 'Visa Inc.', 'stock', 'V', 'V', true),
    ('JNJ', 'Johnson & Johnson', 'stock', 'JNJ', 'JNJ', true),

    -- Crypto
    ('BTC/USD', 'Bitcoin', 'crypto', 'BTC/USD', 'BINANCE:BTCUSDT', true),
    ('ETH/USD', 'Ethereum', 'crypto', 'ETH/USD', 'BINANCE:ETHUSDT', true),
    ('SOL/USD', 'Solana', 'crypto', 'SOL/USD', 'BINANCE:SOLUSDT', true),
    ('DOGE/USD', 'Dogecoin', 'crypto', 'DOGE/USD', 'BINANCE:DOGEUSDT', true),
    ('XRP/USD', 'Ripple', 'crypto', 'XRP/USD', 'BINANCE:XRPUSDT', true),
    ('ADA/USD', 'Cardano', 'crypto', 'ADA/USD', 'BINANCE:ADAUSDT', true),

    -- Forex
    ('EUR/USD', 'Euro/US Dollar', 'forex', 'EUR/USD', 'OANDA:EUR_USD', true),
    ('GBP/USD', 'British Pound/US Dollar', 'forex', 'GBP/USD', 'OANDA:GBP_USD', true),
    ('USD/JPY', 'US Dollar/Japanese Yen', 'forex', 'USD/JPY', 'OANDA:USD_JPY', true),
    ('USD/CHF', 'US Dollar/Swiss Franc', 'forex', 'USD/CHF', 'OANDA:USD_CHF', true),
    ('AUD/USD', 'Australian Dollar/US Dollar', 'forex', 'AUD/USD', 'OANDA:AUD_USD', true),
    ('USD/CAD', 'US Dollar/Canadian Dollar', 'forex', 'USD/CAD', 'OANDA:USD_CAD', true),

    -- Commodities
    ('XAU/USD', 'Gold', 'commodity', 'XAU/USD', 'OANDA:XAU_USD', true),
    ('XAG/USD', 'Silver', 'commodity', 'XAG/USD', 'OANDA:XAG_USD', true),
    ('BRENT', 'Brent Crude Oil', 'commodity', 'BRENT', 'TVC:UKOIL', true),
    ('WTI', 'WTI Crude Oil', 'commodity', 'WTI', 'TVC:USOIL', true);

COMMENT ON TABLE symbols IS 'Master table of tradable symbols/assets that can be assigned to contests';
COMMENT ON COLUMN symbols.symbol IS 'Unique symbol identifier (e.g., AAPL, BTC/USD)';
COMMENT ON COLUMN symbols.name IS 'Human-readable name of the asset';
COMMENT ON COLUMN symbols.asset_type IS 'Type of asset: stock, crypto, forex, or commodity';
COMMENT ON COLUMN symbols.provider_symbol_twelvedata IS 'Symbol mapping for TwelveData provider';
COMMENT ON COLUMN symbols.provider_symbol_finnhub IS 'Symbol mapping for Finnhub provider';
COMMENT ON COLUMN symbols.is_active IS 'Whether the symbol is available for use in contests';

-- ============================================================================
-- ADD SYMBOL MANAGEMENT PERMISSIONS
-- ============================================================================

-- Insert symbol permissions
INSERT INTO permissions (name, description) VALUES
    ('symbols.view', 'View symbol list and details'),
    ('symbols.manage', 'Create, edit, and manage symbols')
ON CONFLICT (name) DO NOTHING;

-- Assign permissions to roles
DO $$
DECLARE
    viewer_role_id INT;
    admin_role_id INT;
    super_admin_role_id INT;
    symbols_view_id INT;
    symbols_manage_id INT;
BEGIN
    -- Get role IDs
    SELECT id INTO viewer_role_id FROM roles WHERE name = 'viewer';
    SELECT id INTO admin_role_id FROM roles WHERE name = 'admin';
    SELECT id INTO super_admin_role_id FROM roles WHERE name = 'super_admin';

    -- Get permission IDs
    SELECT id INTO symbols_view_id FROM permissions WHERE name = 'symbols.view';
    SELECT id INTO symbols_manage_id FROM permissions WHERE name = 'symbols.manage';

    -- VIEWER: symbols.view (read-only)
    IF viewer_role_id IS NOT NULL AND symbols_view_id IS NOT NULL THEN
        INSERT INTO role_permissions (role_id, permission_id)
        VALUES (viewer_role_id, symbols_view_id)
        ON CONFLICT DO NOTHING;
    END IF;

    -- ADMIN: symbols.view + symbols.manage
    IF admin_role_id IS NOT NULL THEN
        IF symbols_view_id IS NOT NULL THEN
            INSERT INTO role_permissions (role_id, permission_id)
            VALUES (admin_role_id, symbols_view_id)
            ON CONFLICT DO NOTHING;
        END IF;
        IF symbols_manage_id IS NOT NULL THEN
            INSERT INTO role_permissions (role_id, permission_id)
            VALUES (admin_role_id, symbols_manage_id)
            ON CONFLICT DO NOTHING;
        END IF;
    END IF;

    -- SUPER_ADMIN: all permissions (both view and manage)
    IF super_admin_role_id IS NOT NULL THEN
        IF symbols_view_id IS NOT NULL THEN
            INSERT INTO role_permissions (role_id, permission_id)
            VALUES (super_admin_role_id, symbols_view_id)
            ON CONFLICT DO NOTHING;
        END IF;
        IF symbols_manage_id IS NOT NULL THEN
            INSERT INTO role_permissions (role_id, permission_id)
            VALUES (super_admin_role_id, symbols_manage_id)
            ON CONFLICT DO NOTHING;
        END IF;
    END IF;
END $$;
