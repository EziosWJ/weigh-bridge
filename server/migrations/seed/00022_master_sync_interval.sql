-- +goose Up
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
SELECT '基础资料同步间隔（分钟）', 'weighing.master-sync-interval-minutes', '60', 'SYSTEM', 'NUMBER', 1, 1, '1 至 10080 分钟，保存后重新计时'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key = 'weighing.master-sync-interval-minutes' AND deleted = 0);
-- +goose Down
DELETE FROM sys_config WHERE config_key = 'weighing.master-sync-interval-minutes' AND is_builtin = 1;
