-- +goose Up
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:simulate','weighing:page:settings'));
DELETE FROM sys_menu WHERE permission_code IN ('weighing:simulate','weighing:page:settings');

-- +goose Down
-- Removed simulator routes are not restored by a seed rollback.
SELECT 1;
