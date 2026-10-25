-- +goose Up
-- Run on the newly prepared database. Do not silently erase business evidence.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM wb_transport_task) OR EXISTS(SELECT 1 FROM wb_sample) OR EXISTS(SELECT 1 FROM wb_weigh_record) OR EXISTS(SELECT 1 FROM wb_ticket) OR EXISTS(SELECT 1 FROM wb_print_attempt) OR EXISTS(SELECT 1 FROM wb_operation) OR EXISTS(SELECT 1 FROM wb_business_event) OR EXISTS(SELECT 1 FROM wb_material_category) OR EXISTS(SELECT 1 FROM wb_material) OR EXISTS(SELECT 1 FROM wb_party) OR EXISTS(SELECT 1 FROM wb_party_contact) OR EXISTS(SELECT 1 FROM wb_number_sequence) OR EXISTS(SELECT 1 FROM sys_user WHERE id<>1) THEN
  RAISE EXCEPTION 'Use a fresh local database: old business data must be explicitly reset before local-only initialization';
 END IF;
END $$;
-- +goose StatementEnd
DELETE FROM wb_user_site;
DELETE FROM wb_agent_binding;
DELETE FROM wb_scale;
DELETE FROM wb_site;
SELECT setval(pg_get_serial_sequence('wb_scale','id'),1,false);
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:temporary:create','weighing:page:settings'));
DELETE FROM sys_menu WHERE permission_code IN ('weighing:temporary:create','weighing:page:settings');
UPDATE sys_menu SET menu_name='本厂称重' WHERE permission_code='weighing:page:workbench';
UPDATE sys_menu SET menu_name='本厂配置' WHERE permission_code='weighing:site:manage';

-- +goose Down
-- Restoring dummy sites would invent new identities. Restore the prior DB for rollback.
SELECT 1;
