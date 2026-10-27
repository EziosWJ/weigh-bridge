-- +goose Up
-- Placeholder second site/scale. The code branch applies once the ADR 0033
-- identity migration has run; the id branch keeps the historical upgrade
-- replay working against an older schema.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='wb_site' AND column_name='site_code') THEN
  INSERT INTO wb_site (site_code,name) VALUES ('SITE01','原厂区'),('SITE02','新增厂区') ON CONFLICT (site_code) DO NOTHING;
  -- Preserve original-site access for existing users; new users require explicit grants.
  INSERT INTO wb_user_site (user_id,site_code,can_operate) SELECT id,'SITE01',1 FROM sys_user WHERE deleted=0 ON CONFLICT (user_id,site_code) DO NOTHING;
  INSERT INTO wb_scale (site_code,code,name,enabled) VALUES ('SITE02','SCALE02','二号地磅',1) ON CONFLICT (code) DO NOTHING;
  INSERT INTO wb_user_site (user_id,site_code,can_operate)
  SELECT DISTINCT ur.user_id,'SITE02',1 FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id
  WHERE r.role_code='ADMIN' AND r.deleted=0 ON CONFLICT (user_id,site_code) DO NOTHING;
 ELSE
  INSERT INTO wb_site (id,code,name) VALUES (1,'SITE01','原厂区'),(2,'SITE02','新增厂区') ON CONFLICT (id) DO NOTHING;
  INSERT INTO wb_user_site (user_id,site_id,can_operate) SELECT id,1,1 FROM sys_user WHERE deleted=0 ON CONFLICT (user_id,site_id) DO NOTHING;
  INSERT INTO wb_scale (code,name,enabled,site_id) VALUES ('SCALE02','二号地磅',1,2) ON CONFLICT (code) DO NOTHING;
  INSERT INTO wb_user_site (user_id,site_id,can_operate)
  SELECT DISTINCT ur.user_id,2,1 FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id
  WHERE r.role_code='ADMIN' AND r.deleted=0 ON CONFLICT (user_id,site_id) DO NOTHING;
 END IF;
END $$;
-- +goose StatementEnd
UPDATE sys_menu SET menu_name='厂区工作台',path='/weighing/sites',component='weighing/sites' WHERE permission_code='weighing:page:workbench';
INSERT INTO sys_menu (parent_id,menu_name,menu_type,permission_code,sort_order,visible,status,is_builtin)
SELECT id,'踢出 Agent','MENU','weighing:agent:kick',110,0,1,1 FROM sys_menu WHERE permission_code='weighing' ON CONFLICT (permission_code) DO NOTHING;
INSERT INTO sys_role_menu (role_id,menu_id) SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.permission_code='weighing:agent:kick' ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:agent:kick');
DELETE FROM sys_menu WHERE permission_code='weighing:agent:kick';
UPDATE sys_menu SET menu_name='称重工作台',path='/weighing/workbench',component='weighing/workbench' WHERE permission_code='weighing:page:workbench';
-- Site and scale identities may already own business data; retain them.
