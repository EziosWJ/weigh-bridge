-- +goose Up
ALTER TABLE wb_agent_config_backup
    ADD COLUMN name VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN notes VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN created_by BIGINT,
    ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE wb_agent_config_backup DROP CONSTRAINT wb_agent_config_backup_source_check;
ALTER TABLE wb_agent_config_backup ADD CONSTRAINT wb_agent_config_backup_source_check CHECK (source IN ('auto','restore','manual'));
ALTER TABLE wb_agent_config_backup DROP CONSTRAINT wb_agent_config_backup_scale_id_config_version_digest_key;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM wb_agent_config_backup WHERE source='manual') THEN
        RAISE EXCEPTION 'manual configuration backups require restoring the previous database backup to roll back without losing named records';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE wb_agent_config_backup DROP CONSTRAINT wb_agent_config_backup_source_check;
ALTER TABLE wb_agent_config_backup ADD CONSTRAINT wb_agent_config_backup_source_check CHECK (source IN ('auto','restore'));
ALTER TABLE wb_agent_config_backup ADD UNIQUE (scale_id,config_version,digest);
ALTER TABLE wb_agent_config_backup DROP COLUMN name, DROP COLUMN notes, DROP COLUMN created_by, DROP COLUMN deleted_at;
