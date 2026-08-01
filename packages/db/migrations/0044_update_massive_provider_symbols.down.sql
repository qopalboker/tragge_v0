-- Revert: restore old Finnhub-style values for provider_symbol_massive

-- Crypto
UPDATE symbols SET provider_symbol_massive = 'BINANCE:BTCUSDT' WHERE symbol = 'BTC/USD';
UPDATE symbols SET provider_symbol_massive = 'BINANCE:ETHUSDT' WHERE symbol = 'ETH/USD';
UPDATE symbols SET provider_symbol_massive = 'BINANCE:SOLUSDT' WHERE symbol = 'SOL/USD';
UPDATE symbols SET provider_symbol_massive = 'BINANCE:DOGEUSDT' WHERE symbol = 'DOGE/USD';
UPDATE symbols SET provider_symbol_massive = 'BINANCE:XRPUSDT' WHERE symbol = 'XRP/USD';
UPDATE symbols SET provider_symbol_massive = 'BINANCE:ADAUSDT' WHERE symbol = 'ADA/USD';

-- Forex
UPDATE symbols SET provider_symbol_massive = 'OANDA:EUR_USD' WHERE symbol = 'EUR/USD';
UPDATE symbols SET provider_symbol_massive = 'OANDA:GBP_USD' WHERE symbol = 'GBP/USD';
UPDATE symbols SET provider_symbol_massive = 'OANDA:USD_JPY' WHERE symbol = 'USD/JPY';
UPDATE symbols SET provider_symbol_massive = 'OANDA:USD_CHF' WHERE symbol = 'USD/CHF';
UPDATE symbols SET provider_symbol_massive = 'OANDA:AUD_USD' WHERE symbol = 'AUD/USD';
UPDATE symbols SET provider_symbol_massive = 'OANDA:USD_CAD' WHERE symbol = 'USD/CAD';

-- Commodities
UPDATE symbols SET provider_symbol_massive = 'OANDA:XAU_USD' WHERE symbol = 'XAU/USD';
UPDATE symbols SET provider_symbol_massive = 'OANDA:XAG_USD' WHERE symbol = 'XAG/USD';
UPDATE symbols SET provider_symbol_massive = 'TVC:UKOIL' WHERE symbol = 'BRENT';
UPDATE symbols SET provider_symbol_massive = 'TVC:USOIL' WHERE symbol = 'WTI';
