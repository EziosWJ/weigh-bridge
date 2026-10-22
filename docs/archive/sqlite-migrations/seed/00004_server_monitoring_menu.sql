-- +goose Up
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, icon, permission_code, sort_order, visible, status, is_builtin)
VALUES (0, '系统监控', 'DIR', '/monitor', 'monitor', 'monitor', 4, 1, 1, 1)
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '服务器监控', 'MENU', '/monitor/server', 'monitor/server/index', 'monitor', 'monitor:server', 1, 1, 1, 1
FROM sys_menu WHERE permission_code = 'monitor'
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code = 'ADMIN' AND r.deleted = 0 AND m.permission_code IN ('monitor', 'monitor:server')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM sys_menu WHERE permission_code IN ('monitor', 'monitor:server') AND is_builtin = 1
);
DELETE FROM sys_menu WHERE permission_code IN ('monitor', 'monitor:server') AND is_builtin = 1;
