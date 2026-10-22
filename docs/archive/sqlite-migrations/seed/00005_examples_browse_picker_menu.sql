-- +goose Up
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '浏览选择 Demo', 'MENU', '/examples/browse-picker', 'examples/browse-picker', 'ListOrdered', 'examples:browse-picker', 6, 1, 1, 1
FROM sys_menu WHERE path = '/examples' AND deleted = 0
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code = 'ADMIN' AND r.deleted = 0 AND m.permission_code = 'examples:browse-picker'
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM sys_menu WHERE permission_code = 'examples:browse-picker' AND is_builtin = 1
);
DELETE FROM sys_menu WHERE permission_code = 'examples:browse-picker' AND is_builtin = 1;
