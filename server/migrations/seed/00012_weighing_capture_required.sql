-- +goose Up
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
VALUES ('称重强制抓拍', 'weighing.capture-required', 'true', 'SYSTEM', 'BOOLEAN', 1, 1,
        '开启后须完成车前车后抓拍及上传；关闭后允许无图确认称重，缺少图片追溯依据。')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM sys_config WHERE config_key = 'weighing.capture-required' AND is_builtin = 1;
