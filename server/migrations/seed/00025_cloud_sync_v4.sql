-- +goose Up
-- Factory-side cloud sync module. Only administrators see the module; ordinary
-- operators keep the brief sync status on the ticket list.
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, icon, permission_code, sort_order, visible, status, is_builtin)
VALUES (0, '厂云同步', 'DIR', '/weighing/sync', 'Cloud', 'weighing:sync', 11, 1, 1, 1)
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '单据同步', 'MENU', '/weighing/sync/tickets', 'weighing/sync-tickets', 'RefreshCw', 'weighing:sync:tickets', 1, 1, 1, 1
FROM sys_menu WHERE permission_code='weighing:sync' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '基础资料同步', 'MENU', '/weighing/sync/master', 'weighing/sync-master', 'Database', 'weighing:sync:master', 2, 1, 1, 1
FROM sys_menu WHERE permission_code='weighing:sync' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '同步日志', 'MENU', '/weighing/sync/logs', 'weighing/sync-logs', 'ListChecks', 'weighing:sync:logs', 3, 1, 1, 1
FROM sys_menu WHERE permission_code='weighing:sync' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin)
SELECT id, '同步处理', 'MENU', 'weighing:sync:manage', 100, 0, 1, 1
FROM sys_menu WHERE permission_code='weighing:sync' ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.deleted=0 AND m.deleted=0 AND m.permission_code IN ('weighing:sync','weighing:sync:tickets','weighing:sync:master','weighing:sync:logs','weighing:sync:manage')
  AND r.role_code='ADMIN'
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- Do not leak the new module into the built-in operator role.
DELETE FROM sys_role_menu
WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:sync','weighing:sync:tickets','weighing:sync:master','weighing:sync:logs','weighing:sync:manage'))
  AND role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR');

INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
SELECT '自动对账时间（北京时间）', 'weighing.sync-reconcile-time', '02:30', 'SYSTEM', 'STRING', 1, 1, 'HH:MM，每天检查最近 7 天及更早未确认变化；错过时启动后补做'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key = 'weighing.sync-reconcile-time' AND deleted = 0);

-- +goose Down
DELETE FROM sys_config WHERE config_key = 'weighing.sync-reconcile-time' AND is_builtin = 1;
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:sync','weighing:sync:tickets','weighing:sync:master','weighing:sync:logs','weighing:sync:manage'));
DELETE FROM sys_menu WHERE permission_code IN ('weighing:sync:tickets','weighing:sync:master','weighing:sync:logs','weighing:sync:manage') AND is_builtin=1;
DELETE FROM sys_menu WHERE permission_code='weighing:sync' AND is_builtin=1;
