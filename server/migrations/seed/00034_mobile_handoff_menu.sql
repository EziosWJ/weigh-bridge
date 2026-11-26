-- +goose Up
-- Factory feedback page for mobile QR first/second handoffs. The scanner API
-- itself uses weighing:confirm; this menu is the operator feedback/exception
-- page.
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '手机扫码', 'MENU', '/weighing/mobile-handoffs', 'weighing/mobile-handoffs', 'ScanLine', 'weighing:mobile-handoff', 8, 1, 1, 1
FROM sys_menu WHERE permission_code='weighing'
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.deleted=0 AND m.deleted=0
  AND r.role_code IN ('ADMIN', 'WEIGHING_OPERATOR')
  AND m.permission_code='weighing:mobile-handoff'
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (
  SELECT id FROM sys_menu WHERE permission_code='weighing:mobile-handoff' AND is_builtin=1
);
DELETE FROM sys_menu WHERE permission_code='weighing:mobile-handoff' AND is_builtin=1;
