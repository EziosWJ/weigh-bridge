-- +goose Up
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '云端连接', 'MENU', '/weighing/sync/connection', 'weighing/sync-connection', 'Link', 'weighing:sync:connection', 4, 1, 1, 1
FROM sys_menu WHERE permission_code='weighing:sync' ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.deleted=0 AND m.deleted=0 AND m.permission_code='weighing:sync:connection' AND r.role_code='ADMIN'
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:sync:connection' AND is_builtin=1);
DELETE FROM sys_menu WHERE permission_code='weighing:sync:connection' AND is_builtin=1;
