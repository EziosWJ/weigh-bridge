-- +goose Up
-- Explicit permission for the manual return-weighing exception. Front-end
-- button visibility never replaces this server-side check.
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '人工补录回磅', 'MENU', 'weighing:manual-return', 100, 0, 1, 1
FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.deleted=0 AND m.deleted=0 AND m.permission_code='weighing:manual-return'
  AND r.role_code IN ('ADMIN','WEIGHING_OPERATOR')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:manual-return');
DELETE FROM sys_menu WHERE permission_code='weighing:manual-return' AND is_builtin=1;
