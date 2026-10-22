-- +goose Up
INSERT INTO wb_agent_setting (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
INSERT INTO sys_menu (parent_id,menu_name,menu_type,path,component,permission_code,sort_order,visible,status,is_builtin)
SELECT id,'厂区配置','MENU','/weighing/site-config','weighing/site-config','weighing:site:manage',115,1,1,1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_menu (parent_id,menu_name,menu_type,permission_code,sort_order,visible,status,is_builtin)
SELECT id,'启动配置导出','MENU','weighing:site:export',116,0,1,1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_role_menu (role_id,menu_id) SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.permission_code IN ('weighing:site:manage','weighing:site:export') ON CONFLICT DO NOTHING;
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:site:manage','weighing:site:export'));
DELETE FROM sys_menu WHERE permission_code IN ('weighing:site:manage','weighing:site:export');
