-- +goose Up
-- Complete the built-in operator's default log navigation using existing RBAC.
-- Keep all business action grants and administrator/custom role grants intact.
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id
FROM sys_role r
CROSS JOIN sys_menu m
WHERE r.role_code = 'WEIGHING_OPERATOR' AND r.deleted = 0
  AND m.deleted = 0
  AND m.permission_code IN ('system:log', 'system:login-log', 'system:oper-log')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
-- These grants may have existed before this migration; preserve them on rollback.
SELECT 1;
