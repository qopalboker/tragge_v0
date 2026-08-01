-- Add complete forex pairs and Dow Jones to symbols table
-- Only insert if not already exists

INSERT INTO symbols (symbol, name, asset_type, provider_symbol_massive, provider_symbol_twelvedata, provider_symbol_finnhub, is_active)
VALUES
  -- Majors (update if exists, insert if not)
  ('EUR/USD', 'Euro / US Dollar', 'forex', 'EUR-USD', 'EUR/USD', 'OANDA:EUR_USD', true),
  ('GBP/USD', 'British Pound / US Dollar', 'forex', 'GBP-USD', 'GBP/USD', 'OANDA:GBP_USD', true),
  ('USD/JPY', 'US Dollar / Japanese Yen', 'forex', 'USD-JPY', 'USD/JPY', 'OANDA:USD_JPY', true),
  ('USD/CHF', 'US Dollar / Swiss Franc', 'forex', 'USD-CHF', 'USD/CHF', 'OANDA:USD_CHF', true),
  ('AUD/USD', 'Australian Dollar / US Dollar', 'forex', 'AUD-USD', 'AUD/USD', 'OANDA:AUD_USD', true),
  ('USD/CAD', 'US Dollar / Canadian Dollar', 'forex', 'USD-CAD', 'USD/CAD', 'OANDA:USD_CAD', true),
  ('NZD/USD', 'New Zealand Dollar / US Dollar', 'forex', 'NZD-USD', 'NZD/USD', 'OANDA:NZD_USD', true),
  -- Minors
  ('EUR/GBP', 'Euro / British Pound', 'forex', 'EUR-GBP', 'EUR/GBP', 'OANDA:EUR_GBP', true),
  ('EUR/JPY', 'Euro / Japanese Yen', 'forex', 'EUR-JPY', 'EUR/JPY', 'OANDA:EUR_JPY', true),
  ('EUR/CHF', 'Euro / Swiss Franc', 'forex', 'EUR-CHF', 'EUR/CHF', 'OANDA:EUR_CHF', true),
  ('EUR/AUD', 'Euro / Australian Dollar', 'forex', 'EUR-AUD', 'EUR/AUD', 'OANDA:EUR_AUD', true),
  ('EUR/CAD', 'Euro / Canadian Dollar', 'forex', 'EUR-CAD', 'EUR/CAD', 'OANDA:EUR_CAD', true),
  ('EUR/NZD', 'Euro / New Zealand Dollar', 'forex', 'EUR-NZD', 'EUR/NZD', 'OANDA:EUR_NZD', true),
  ('GBP/JPY', 'British Pound / Japanese Yen', 'forex', 'GBP-JPY', 'GBP/JPY', 'OANDA:GBP_JPY', true),
  ('GBP/CHF', 'British Pound / Swiss Franc', 'forex', 'GBP-CHF', 'GBP/CHF', 'OANDA:GBP_CHF', true),
  ('GBP/AUD', 'British Pound / Australian Dollar', 'forex', 'GBP-AUD', 'GBP/AUD', 'OANDA:GBP_AUD', true),
  ('GBP/CAD', 'British Pound / Canadian Dollar', 'forex', 'GBP-CAD', 'GBP/CAD', 'OANDA:GBP_CAD', true),
  ('GBP/NZD', 'British Pound / New Zealand Dollar', 'forex', 'GBP-NZD', 'GBP/NZD', 'OANDA:GBP_NZD', true),
  ('AUD/JPY', 'Australian Dollar / Japanese Yen', 'forex', 'AUD-JPY', 'AUD/JPY', 'OANDA:AUD_JPY', true),
  ('AUD/CHF', 'Australian Dollar / Swiss Franc', 'forex', 'AUD-CHF', 'AUD/CHF', 'OANDA:AUD_CHF', true),
  ('AUD/CAD', 'Australian Dollar / Canadian Dollar', 'forex', 'AUD-CAD', 'AUD/CAD', 'OANDA:AUD_CAD', true),
  ('AUD/NZD', 'Australian Dollar / New Zealand Dollar', 'forex', 'AUD-NZD', 'AUD/NZD', 'OANDA:AUD_NZD', true),
  ('CAD/JPY', 'Canadian Dollar / Japanese Yen', 'forex', 'CAD-JPY', 'CAD/JPY', 'OANDA:CAD_JPY', true),
  ('CAD/CHF', 'Canadian Dollar / Swiss Franc', 'forex', 'CAD-CHF', 'CAD/CHF', 'OANDA:CAD_CHF', true),
  ('CHF/JPY', 'Swiss Franc / Japanese Yen', 'forex', 'CHF-JPY', 'CHF/JPY', 'OANDA:CHF_JPY', true),
  ('NZD/JPY', 'New Zealand Dollar / Japanese Yen', 'forex', 'NZD-JPY', 'NZD/JPY', 'OANDA:NZD_JPY', true),
  ('NZD/CHF', 'New Zealand Dollar / Swiss Franc', 'forex', 'NZD-CHF', 'NZD/CHF', 'OANDA:NZD_CHF', true),
  ('NZD/CAD', 'New Zealand Dollar / Canadian Dollar', 'forex', 'NZD-CAD', 'NZD/CAD', 'OANDA:NZD_CAD', true),
  -- Exotics
  ('USD/TRY', 'US Dollar / Turkish Lira', 'forex', 'USD-TRY', 'USD/TRY', 'OANDA:USD_TRY', true),
  ('USD/MXN', 'US Dollar / Mexican Peso', 'forex', 'USD-MXN', 'USD/MXN', 'OANDA:USD_MXN', true),
  ('USD/ZAR', 'US Dollar / South African Rand', 'forex', 'USD-ZAR', 'USD/ZAR', 'OANDA:USD_ZAR', true),
  ('USD/SGD', 'US Dollar / Singapore Dollar', 'forex', 'USD-SGD', 'USD/SGD', 'OANDA:USD_SGD', true),
  ('USD/HKD', 'US Dollar / Hong Kong Dollar', 'forex', 'USD-HKD', 'USD/HKD', 'OANDA:USD_HKD', true),
  ('USD/NOK', 'US Dollar / Norwegian Krone', 'forex', 'USD-NOK', 'USD/NOK', 'OANDA:USD_NOK', true),
  ('USD/SEK', 'US Dollar / Swedish Krona', 'forex', 'USD-SEK', 'USD/SEK', 'OANDA:USD_SEK', true),
  ('USD/DKK', 'US Dollar / Danish Krone', 'forex', 'USD-DKK', 'USD/DKK', 'OANDA:USD_DKK', true),
  ('USD/PLN', 'US Dollar / Polish Zloty', 'forex', 'USD-PLN', 'USD/PLN', 'OANDA:USD_PLN', true),
  ('USD/CZK', 'US Dollar / Czech Koruna', 'forex', 'USD-CZK', 'USD/CZK', 'OANDA:USD_CZK', true),
  ('USD/HUF', 'US Dollar / Hungarian Forint', 'forex', 'USD-HUF', 'USD/HUF', 'OANDA:USD_HUF', true),
  ('EUR/TRY', 'Euro / Turkish Lira', 'forex', 'EUR-TRY', 'EUR/TRY', 'OANDA:EUR_TRY', true),
  ('EUR/SEK', 'Euro / Swedish Krona', 'forex', 'EUR-SEK', 'EUR/SEK', 'OANDA:EUR_SEK', true),
  ('EUR/NOK', 'Euro / Norwegian Krone', 'forex', 'EUR-NOK', 'EUR/NOK', 'OANDA:EUR_NOK', true),
  ('EUR/PLN', 'Euro / Polish Zloty', 'forex', 'EUR-PLN', 'EUR/PLN', 'OANDA:EUR_PLN', true),
  ('GBP/TRY', 'British Pound / Turkish Lira', 'forex', 'GBP-TRY', 'GBP/TRY', 'OANDA:GBP_TRY', true),
  -- Index
  ('US30/USD', 'Dow Jones Industrial Average', 'commodity', 'US30-USD', 'DJ30', 'OANDA:US30_USD', true)
ON CONFLICT (symbol) DO UPDATE SET
  name = EXCLUDED.name,
  provider_symbol_massive = COALESCE(EXCLUDED.provider_symbol_massive, symbols.provider_symbol_massive),
  provider_symbol_twelvedata = COALESCE(EXCLUDED.provider_symbol_twelvedata, symbols.provider_symbol_twelvedata),
  provider_symbol_finnhub = COALESCE(EXCLUDED.provider_symbol_finnhub, symbols.provider_symbol_finnhub),
  is_active = EXCLUDED.is_active;
