-- +goose Up
-- ADR 0030: the ticket title moves from the Server YAML file into sys_config.
-- The empty value is a one-time upgrade marker. The API replaces it with the
-- deployment's previous YAML value on first start; later edits by an
-- administrator are never overwritten because the value is no longer empty.
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
VALUES ('单据标题', 'weighing.ticket-title', '', 'SYSTEM', 'TEXT', 1, 1,
        '正式过磅单标题；升级时沿用原 YAML 配置并自此只在此处维护。')
ON CONFLICT (config_key) DO NOTHING;

-- The independent 本厂配置 menu is retired. Keep the permission rows so
-- existing roles keep exactly the same capability, but hide the legacy
-- navigation entry; the capability is exercised inside 系统管理 → 配置管理.
UPDATE sys_menu SET visible=0
WHERE permission_code IN ('weighing:site:manage','weighing:site:export') AND is_builtin=1;

-- +goose Down
UPDATE sys_menu SET visible=1 WHERE permission_code='weighing:site:manage' AND is_builtin=1;
UPDATE sys_menu SET visible=0 WHERE permission_code='weighing:site:export' AND is_builtin=1;
DELETE FROM sys_config WHERE config_key='weighing.ticket-title' AND is_builtin=1;
