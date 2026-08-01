-- 0099_admin_canonical_roles.down.sql
-- Development rollback only. Production migration policy requires a reviewed
-- compensating forward migration instead of destructive down execution.

INSERT INTO user_roles (user_id, role_id)
SELECT ur.user_id, legacy.id
FROM user_roles ur
JOIN roles support ON support.id = ur.role_id AND support.name = 'support_admin'
CROSS JOIN roles legacy
WHERE legacy.name = 'admin'
ON CONFLICT DO NOTHING;

DELETE FROM roles WHERE name = 'support_admin';
