-- +goose Up
-- Placeholder site/scale rows for first-time weighing permissions. A database
-- that already has the code identity (ADR 0033) stores codes; replaying this
-- seed against an older schema keeps the original shape so the documented
-- upgrade path stays reproducible. Seed 00013 clears these placeholders on a
-- fresh database before the local identity is installed.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='wb_scale' AND column_name='site_code') THEN
   INSERT INTO wb_site (site_code, name) VALUES ('SITE01', '原厂区') ON CONFLICT (site_code) DO NOTHING;
   INSERT INTO wb_scale (site_code, code, name, enabled) VALUES ('SITE01', 'SCALE01', '一号地磅', 1) ON CONFLICT (code) DO NOTHING;
 ELSE
   INSERT INTO wb_scale (code, name, enabled) VALUES ('SCALE01', '一号地磅', 1) ON CONFLICT (code) DO NOTHING;
 END IF;
END $$;
-- +goose StatementEnd
INSERT INTO sys_role (role_name, role_code, status, sort_order, is_builtin, remark)
VALUES ('地磅操作员', 'WEIGHING_OPERATOR', 1, 10, 1, '称重及异常处置，无主管审批') ON CONFLICT (role_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, icon, permission_code, sort_order, visible, status, is_builtin)
VALUES (0, '地磅业务', 'DIR', '/weighing', 'Scale', 'weighing', -40, 1, 1, 1) ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '称重工作台', 'MENU', '/weighing/workbench', 'weighing/workbench', 'Scale', 'weighing:page:workbench', 0, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '运输任务', 'MENU', '/weighing/tasks', 'weighing/tasks', 'Table2', 'weighing:page:tasks', 1, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '原始称重', 'MENU', '/weighing/originals', 'weighing/originals', 'FileSearch', 'weighing:page:originals', 2, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '最终单据', 'MENU', '/weighing/tickets', 'weighing/tickets', 'FileText', 'weighing:page:tickets', 3, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '物料资料', 'MENU', '/weighing/materials', 'weighing/materials', 'Table2', 'weighing:page:materials', 4, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '往来主体', 'MENU', '/weighing/parties', 'weighing/parties', 'Network', 'weighing:page:parties', 5, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '物料分类', 'MENU', '/weighing/categories', 'weighing/categories', 'ListOrdered', 'weighing:page:categories', 6, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin) SELECT id, '演示设置', 'MENU', '/weighing/settings', 'weighing/settings', 'PanelLeft', 'weighing:page:settings', 7, 1, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '资料查询', 'MENU', 'weighing:master:read', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '资料维护', 'MENU', 'weighing:master:write', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '业务查询', 'MENU', 'weighing:read', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '称重确认', 'MENU', 'weighing:confirm', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '信息纠错', 'MENU', 'weighing:correct', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '重量调整', 'MENU', 'weighing:adjust', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '单磅重称', 'MENU', 'weighing:reweigh', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '任务终止', 'MENU', 'weighing:terminate', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '单据作废', 'MENU', 'weighing:void', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '更正重开', 'MENU', 'weighing:reopen', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '打印', 'MENU', 'weighing:print', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '补打', 'MENU', 'weighing:reprint', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '导出', 'MENU', 'weighing:export', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '接管', 'MENU', 'weighing:takeover', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin) SELECT id, '测试采集', 'MENU', 'weighing:simulate', 100, 0, 1, 1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.deleted=0 AND m.deleted=0 AND (m.permission_code='weighing' OR m.permission_code LIKE 'weighing:%')
AND (r.role_code='ADMIN' OR (r.role_code='WEIGHING_OPERATOR' AND m.permission_code <> 'weighing:master:write'))
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE (permission_code='weighing' OR permission_code LIKE 'weighing:%') AND is_builtin=1);
DELETE FROM sys_menu WHERE (permission_code='weighing' OR permission_code LIKE 'weighing:%') AND is_builtin=1;
DELETE FROM sys_role WHERE role_code='WEIGHING_OPERATOR' AND is_builtin=1 AND NOT EXISTS (SELECT 1 FROM sys_user_role WHERE role_id=sys_role.id);
-- Keep the stable scale identity on seed rollback; schema rollback owns its removal.
