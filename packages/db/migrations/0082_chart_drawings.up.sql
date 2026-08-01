CREATE TABLE chart_drawings (
    user_id    UUID NOT NULL,
    contest_id UUID NOT NULL,
    symbol     VARCHAR(20) NOT NULL,
    drawings   JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, contest_id, symbol)
);

CREATE INDEX idx_chart_drawings_user
    ON chart_drawings(user_id);

COMMENT ON TABLE chart_drawings IS 'User chart drawings (trend lines, fib, etc.) per contest and symbol';
