DROP TRIGGER IF EXISTS trg_tournament_templates_updated_at ON tournament_templates;
DROP FUNCTION IF EXISTS update_tournament_templates_updated_at();
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS updated_at;
