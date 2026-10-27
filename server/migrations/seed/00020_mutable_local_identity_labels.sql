-- +goose Up
UPDATE sys_config
SET remark='通过本厂身份页面维护；历史业务保留原快照', update_time=CURRENT_TIMESTAMP
WHERE config_key IN ('weighing.local-site-code','weighing.local-scale-code') AND is_builtin=1;

-- +goose Down
-- The metadata may have been customized; do not overwrite it during rollback.
SELECT 1;
