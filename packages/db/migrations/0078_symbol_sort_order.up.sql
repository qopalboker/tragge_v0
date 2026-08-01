ALTER TABLE symbols ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 999;

-- Crypto: ordered by market cap
UPDATE symbols SET sort_order = 1  WHERE symbol = 'BTC/USD';
UPDATE symbols SET sort_order = 2  WHERE symbol = 'ETH/USD';
UPDATE symbols SET sort_order = 3  WHERE symbol = 'SOL/USD';
UPDATE symbols SET sort_order = 4  WHERE symbol = 'XRP/USD';
UPDATE symbols SET sort_order = 5  WHERE symbol = 'DOGE/USD';
UPDATE symbols SET sort_order = 6  WHERE symbol = 'ADA/USD';
UPDATE symbols SET sort_order = 7  WHERE symbol = 'AVAX/USD';
UPDATE symbols SET sort_order = 8  WHERE symbol = 'LINK/USD';
UPDATE symbols SET sort_order = 9  WHERE symbol = 'DOT/USD';
UPDATE symbols SET sort_order = 10 WHERE symbol = 'LTC/USD';
UPDATE symbols SET sort_order = 11 WHERE symbol = 'BCH/USD';
UPDATE symbols SET sort_order = 12 WHERE symbol = 'POL/USD';
UPDATE symbols SET sort_order = 13 WHERE symbol = 'SHIB/USD';
UPDATE symbols SET sort_order = 14 WHERE symbol = 'UNI/USD';
UPDATE symbols SET sort_order = 15 WHERE symbol = 'XLM/USD';
UPDATE symbols SET sort_order = 16 WHERE symbol = 'NEAR/USD';
UPDATE symbols SET sort_order = 17 WHERE symbol = 'AAVE/USD';
UPDATE symbols SET sort_order = 18 WHERE symbol = 'SUI/USD';
UPDATE symbols SET sort_order = 19 WHERE symbol = 'PEPE/USD';
UPDATE symbols SET sort_order = 20 WHERE symbol = 'APT/USD';
UPDATE symbols SET sort_order = 21 WHERE symbol = 'HBAR/USD';
UPDATE symbols SET sort_order = 22 WHERE symbol = 'ICP/USD';
UPDATE symbols SET sort_order = 23 WHERE symbol = 'VET/USD';
UPDATE symbols SET sort_order = 24 WHERE symbol = 'CRO/USD';

-- Forex: ordered by liquidity
UPDATE symbols SET sort_order = 1  WHERE symbol = 'EUR/USD';
UPDATE symbols SET sort_order = 2  WHERE symbol = 'GBP/USD';
UPDATE symbols SET sort_order = 3  WHERE symbol = 'USD/JPY';
UPDATE symbols SET sort_order = 4  WHERE symbol = 'USD/CHF';
UPDATE symbols SET sort_order = 5  WHERE symbol = 'AUD/USD';
UPDATE symbols SET sort_order = 6  WHERE symbol = 'NZD/USD';
UPDATE symbols SET sort_order = 7  WHERE symbol = 'USD/CAD';
UPDATE symbols SET sort_order = 8  WHERE symbol = 'EUR/GBP';
UPDATE symbols SET sort_order = 9  WHERE symbol = 'EUR/JPY';
UPDATE symbols SET sort_order = 10 WHERE symbol = 'GBP/JPY';

-- Commodity
UPDATE symbols SET sort_order = 1  WHERE symbol = 'XAU/USD';
UPDATE symbols SET sort_order = 2  WHERE symbol = 'XAG/USD';
