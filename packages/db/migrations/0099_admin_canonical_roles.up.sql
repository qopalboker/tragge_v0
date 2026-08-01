-- 0099_admin_canonical_roles.up.sql
-- SEC-004: migrate the current legacy development schema to the canonical
-- USER / SUPPORT_ADMIN / SUPER_ADMIN authorization model. The clean target
-- auth schema remains owned by later Platform data/architecture migrations.

INSERT INTO roles (name) VALUES ('support_admin')
ON CONFLICT (name) DO NOTHING;

-- Support Admin is intentionally limited to KYC permissions. Support-ticket
-- routes use their existing authenticated Admin boundary and do not currently
-- define granular permission rows.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN ('kyc.view', 'kyc.review')
WHERE r.name = 'support_admin'
ON CONFLICT DO NOTHING;

-- Migrate legacy Admin assignments without preserving the deprecated elevated
-- role in active sessions. SUPER_ADMIN assignments, when present, are retained.
INSERT INTO user_roles (user_id, role_id)
SELECT ur.user_id, support.id
FROM user_roles ur
JOIN roles legacy ON legacy.id = ur.role_id AND legacy.name = 'admin'
CROSS JOIN roles support
WHERE support.name = 'support_admin'
ON CONFLICT DO NOTHING;

DELETE FROM user_roles ur
USING roles legacy
WHERE ur.role_id = legacy.id
  AND legacy.name = 'admin';
