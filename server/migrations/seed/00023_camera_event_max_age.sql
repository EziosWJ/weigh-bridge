-- +goose Up
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
SELECT '车牌事件有效期（分钟）', 'weighing.camera-event-max-age-minutes', '5', 'SYSTEM', 'NUMBER', 1, 1, '1 至 10080 分钟；修改后立即生效，超过有效期的上报丢弃并应答'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key = 'weighing.camera-event-max-age-minutes' AND deleted = 0);
-- +goose Down
DELETE FROM sys_config WHERE config_key = 'weighing.camera-event-max-age-minutes' AND is_builtin = 1;
