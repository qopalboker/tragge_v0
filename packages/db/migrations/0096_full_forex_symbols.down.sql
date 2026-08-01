-- Remove exotic and cross forex pairs added in 0096 (keep the original majors)
DELETE FROM symbols WHERE symbol IN (
  'EUR/GBP','EUR/CHF','EUR/AUD','EUR/CAD','EUR/NZD',
  'GBP/CHF','GBP/AUD','GBP/CAD','GBP/NZD',
  'AUD/CHF','AUD/CAD','AUD/NZD',
  'CAD/CHF','NZD/CHF','NZD/CAD',
  'USD/TRY','USD/MXN','USD/ZAR','USD/SGD','USD/HKD',
  'USD/NOK','USD/SEK','USD/DKK','USD/PLN','USD/CZK','USD/HUF',
  'EUR/TRY','EUR/SEK','EUR/NOK','EUR/PLN','GBP/TRY',
  'US30/USD'
);
