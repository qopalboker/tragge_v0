-- 0024_admin_roles.down.sql
-- Rollback viewer and super_admin roles with permissions system

-- Drop indexes first
DROP INDEX IF EXISTS idx_role_permissions_permission_id;
DROP INDEX IF EXISTS idx_role_permissions_role_id;
DROP INDEX IF EXISTS idx_permissions_name;

-- Drop junction table
DROP TABLE IF EXISTS role_permissions;

-- Drop permissions table
DROP TABLE IF EXISTS permissions;

-- Remove added roles (keep existing user, admin, moderator)
DELETE FROM roles WHERE name IN ('viewer', 'super_admin');
