-- Rollback migration: 0026_symbols_master

-- Remove symbol permissions from role_permissions
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN ('symbols.view', 'symbols.manage')
);

-- Remove symbol permissions
DELETE FROM permissions WHERE name IN ('symbols.view', 'symbols.manage');

-- Drop trigger first
DROP TRIGGER IF EXISTS symbols_updated_at_trigger ON symbols;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_symbols_updated_at();

-- Drop table
DROP TABLE IF EXISTS symbols;

-- Drop enum type
DROP TYPE IF EXISTS asset_type;
