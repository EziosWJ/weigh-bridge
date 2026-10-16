-- +goose Up
ALTER TABLE sys_notification DROP CONSTRAINT ck_sys_notification_source;
ALTER TABLE sys_notification ADD CONSTRAINT ck_sys_notification_source CHECK (source_type IN ('MANUAL','ROLE_CHANGE','BUSINESS'));
-- +goose Down
UPDATE sys_notification SET source_type='MANUAL' WHERE source_type='BUSINESS';
ALTER TABLE sys_notification DROP CONSTRAINT ck_sys_notification_source;
ALTER TABLE sys_notification ADD CONSTRAINT ck_sys_notification_source CHECK (source_type IN ('MANUAL','ROLE_CHANGE'));
