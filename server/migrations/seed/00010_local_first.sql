-- +goose Up
INSERT INTO sys_menu (parent_id,menu_name,menu_type,permission_code,sort_order,visible,status,is_builtin)
SELECT id,'临时主体补录','MENU','weighing:temporary:create',100,0,1,1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r,sys_menu m WHERE r.role_code IN ('ADMIN','WEIGHING_OPERATOR') AND m.permission_code='weighing:temporary:create'
ON CONFLICT (role_id,menu_id) DO NOTHING;
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:temporary:create');
DELETE FROM sys_menu WHERE permission_code='weighing:temporary:create';
