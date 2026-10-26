-- +goose Up
-- Fresh installations use the database default; preserve any title already set.
UPDATE sys_config
SET config_value = '酒厂过磅单', update_time = CURRENT_TIMESTAMP
WHERE config_key = 'weighing.ticket-title'
  AND is_builtin = 1
  AND btrim(config_value) = '';

-- +goose Down
-- Restore the migration marker only when the value is still the default.
UPDATE sys_config
SET config_value = '', update_time = CURRENT_TIMESTAMP
WHERE config_key = 'weighing.ticket-title'
  AND is_builtin = 1
  AND config_value = '酒厂过磅单';
