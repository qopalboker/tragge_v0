-- 0005_candles.up.sql
-- Add candles table for persistent OHLCV candle storage

-- ============================================================================
-- CANDLES TABLE
-- ============================================================================

CREATE TABLE candles (
    symbol VARCHAR(20) NOT NULL,
    resolution VARCHAR(10) NOT NULL, -- "1m", "5m", "15m", "30m", "1h", "4h", "1d"
    time BIGINT NOT NULL, -- Unix timestamp in seconds (start of candle window)
    open DOUBLE PRECISION NOT NULL,
    high DOUBLE PRECISION NOT NULL,
    low DOUBLE PRECISION NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    volume DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (symbol, resolution, time)
);

-- Index for efficient time range queries
CREATE INDEX idx_candles_symbol_resolution_time ON candles(symbol, resolution, time DESC);

-- Index for time-based queries
CREATE INDEX idx_candles_time ON candles(time DESC);

-- Composite index for common query pattern
CREATE INDEX idx_candles_lookup ON candles(symbol, resolution) INCLUDE (time, open, high, low, close, volume);

-- Add comment
COMMENT ON TABLE candles IS 'OHLCV candles aggregated from market data ticks';
COMMENT ON COLUMN candles.symbol IS 'Trading symbol (e.g., AAPL, GOOGL)';
COMMENT ON COLUMN candles.resolution IS 'Candle resolution: 1m, 5m, 15m, 30m, 1h, 4h, 1d';
COMMENT ON COLUMN candles.time IS 'Unix timestamp in seconds (start of candle window)';
COMMENT ON COLUMN candles.volume IS 'Tick count or volume proxy';
