-- +goose Up
-- The business tables remain the referential records. These protected entries
-- are maintained transactionally with them by the local identity service.
INSERT INTO sys_config (config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
VALUES
 ('厂区编码', 'weighing.local-site-code', COALESCE((SELECT site_code FROM wb_local_identity WHERE id=1), ''), 'SYSTEM', 'TEXT', 1, 1, '由本厂身份页面首次确定，不可修改'),
 ('地磅编码', 'weighing.local-scale-code', COALESCE((SELECT scale_code FROM wb_local_identity WHERE id=1), ''), 'SYSTEM', 'TEXT', 1, 1, '由本厂身份页面首次确定，不可修改'),
 ('厂区名称', 'weighing.local-site-name', COALESCE((SELECT s.name FROM wb_site s JOIN wb_local_identity i ON i.site_code=s.site_code WHERE i.id=1), ''), 'SYSTEM', 'TEXT', 1, 1, '由本厂身份页面维护'),
 ('地磅名称', 'weighing.local-scale-name', COALESCE((SELECT s.name FROM wb_scale s JOIN wb_local_identity i ON i.site_code=s.site_code AND i.scale_code=s.code WHERE i.id=1), ''), 'SYSTEM', 'TEXT', 1, 1, '由本厂身份页面维护');

-- +goose Down
DELETE FROM sys_config WHERE config_key IN ('weighing.local-site-code', 'weighing.local-scale-code', 'weighing.local-site-name', 'weighing.local-scale-name') AND is_builtin=1;
