-- +goose Up
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin)
VALUES
    (0, '设备维护查看', 'MENU', 'agent:maintenance:view', 100, 0, 1, 1),
    (0, '设备维护操作', 'MENU', 'agent:maintenance:manage', 101, 0, 1, 1)
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id
FROM sys_role r
CROSS JOIN sys_menu m
WHERE r.role_code = 'ADMIN'
  AND r.deleted = 0
  AND m.permission_code IN ('agent:maintenance:view', 'agent:maintenance:manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu
WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('agent:maintenance:view', 'agent:maintenance:manage'));
DELETE FROM sys_menu WHERE permission_code IN ('agent:maintenance:view', 'agent:maintenance:manage') AND is_builtin = 1;
