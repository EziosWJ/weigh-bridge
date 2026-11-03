-- +goose Up
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
VALUES ('回磅完成自动打印', 'weighing.auto-print', 'false', 'SYSTEM', 'BOOLEAN', 1, 1,
        '开启后，工作台回磅确认成功时自动发起一次首次打印；结果未知时先核实。')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM sys_config WHERE config_key = 'weighing.auto-print' AND is_builtin = 1;
