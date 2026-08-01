ALTER TABLE tournament_templates
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Trigger to auto-update updated_at on row modification
CREATE OR REPLACE FUNCTION update_tournament_templates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_tournament_templates_updated_at
    BEFORE UPDATE ON tournament_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_tournament_templates_updated_at();
