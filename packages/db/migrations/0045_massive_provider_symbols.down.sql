-- Revert: 0045_massive_provider_symbols
-- Remove all symbols that were added in this migration

DELETE FROM symbols WHERE symbol IN (
    -- Forex additions
    'USD/CNY', 'USD/HKD', 'USD/SGD', 'USD/KRW', 'USD/INR', 'USD/MXN',
    'EUR/JPY', 'EUR/GBP', 'NZD/USD', 'USD/BRL', 'USD/ZAR', 'USD/PLN',
    'USD/SEK', 'USD/NOK',
    -- USOIL commodity alias
    'USOIL',
    -- Crypto additions
    'AAVE/USD', 'APT/USD', 'AVAX/USD', 'BCH/USD', 'LINK/USD', 'CRO/USD',
    'HBAR/USD', 'ICP/USD', 'LTC/USD', 'NEAR/USD', 'PEPE/USD', 'DOT/USD',
    'SHIB/USD', 'XLM/USD', 'SUI/USD', 'UNI/USD', 'VET/USD', 'POL/USD'
);
