-- Add take_profit and stop_loss columns to positions table (partitioned)
-- These are needed by the trading engine for TP/SL evaluation on open positions

ALTER TABLE positions ADD COLUMN IF NOT EXISTS take_profit numeric(20,8);
ALTER TABLE positions ADD COLUMN IF NOT EXISTS stop_loss numeric(20,8);
