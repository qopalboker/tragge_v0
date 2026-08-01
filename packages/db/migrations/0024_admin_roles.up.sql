-- 0024_admin_roles.up.sql
-- Add viewer and super_admin roles with granular permissions system

-- ============================================================================
-- ADD NEW ADMIN ROLES
-- ============================================================================

-- Add new roles (viewer for read-only access, super_admin for full access)
INSERT INTO roles (name) VALUES ('viewer'), ('super_admin') ON CONFLICT (name) DO NOTHING;

-- ============================================================================
-- PERMISSIONS TABLE
-- ============================================================================

CREATE TABLE permissions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index for permission lookups
CREATE INDEX idx_permissions_name ON permissions(name);

-- ============================================================================
-- ROLE-PERMISSIONS JUNCTION TABLE
-- ============================================================================

CREATE TABLE role_permissions (
    role_id INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);

-- ============================================================================
-- INSERT PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    -- User management permissions
    ('users.view', 'View user list and details'),
    ('users.edit', 'Edit user profiles and status'),
    ('users.wallet.charge', 'Charge user wallets'),

    -- Contest permissions
    ('contests.view', 'View contests'),
    ('contests.create', 'Create and edit contests'),
    ('contests.manage', 'Start, stop, cancel contests'),

    -- KYC permissions
    ('kyc.view', 'View KYC submissions'),
    ('kyc.review', 'Approve/reject KYC'),

    -- Withdrawal permissions
    ('withdrawals.view', 'View withdrawal requests'),
    ('withdrawals.manage', 'Approve/reject withdrawals'),

    -- Audit permissions
    ('audit.view', 'View audit logs'),

    -- Shard permissions
    ('shards.view', 'View shard status'),

    -- Settings permissions
    ('settings.manage', 'Manage system settings'),

    -- Affiliate permissions
    ('affiliate.manage', 'Manage affiliate program'),

    -- Financial permissions
    ('financial.view', 'View financial reports');

-- ============================================================================
-- ASSIGN PERMISSIONS TO ROLES
-- ============================================================================

-- Get role IDs for assignment
DO $$
DECLARE
    viewer_role_id INT;
    admin_role_id INT;
    super_admin_role_id INT;
BEGIN
    -- Get role IDs
    SELECT id INTO viewer_role_id FROM roles WHERE name = 'viewer';
    SELECT id INTO admin_role_id FROM roles WHERE name = 'admin';
    SELECT id INTO super_admin_role_id FROM roles WHERE name = 'super_admin';

    -- VIEWER: All *.view permissions (read-only access)
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT viewer_role_id, p.id
    FROM permissions p
    WHERE p.name LIKE '%.view'
    ON CONFLICT DO NOTHING;

    -- ADMIN: Viewer permissions + most management permissions
    -- First, give all viewer permissions
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT admin_role_id, p.id
    FROM permissions p
    WHERE p.name LIKE '%.view'
    ON CONFLICT DO NOTHING;

    -- Then add management permissions (except sensitive ones)
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT admin_role_id, p.id
    FROM permissions p
    WHERE p.name IN (
        'users.edit',
        'contests.create',
        'contests.manage',
        'kyc.review',
        'withdrawals.manage',
        'affiliate.manage'
    )
    ON CONFLICT DO NOTHING;

    -- SUPER_ADMIN: All permissions
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT super_admin_role_id, p.id
    FROM permissions p
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE permissions IS 'Granular permissions for admin panel access control';
COMMENT ON COLUMN permissions.name IS 'Permission identifier in format: resource.action (e.g., users.view)';
COMMENT ON COLUMN permissions.description IS 'Human-readable description of what the permission grants';

COMMENT ON TABLE role_permissions IS 'Junction table linking roles to their granted permissions';
COMMENT ON COLUMN role_permissions.granted_at IS 'Timestamp when the permission was granted to the role';
