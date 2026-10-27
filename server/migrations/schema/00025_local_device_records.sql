-- +goose Up
-- Configuration backups belong to this local installation. Keep scale_code as
-- the historical label, not as a reference to the mutable current code.
ALTER TABLE wb_agent_config_backup DROP CONSTRAINT fk_wb_backup_scale;
ALTER TABLE wb_agent_runtime DROP CONSTRAINT fk_wb_runtime_scale;
ALTER TABLE wb_agent_runtime DROP CONSTRAINT wb_agent_runtime_pkey;
ALTER TABLE wb_agent_runtime ADD COLUMN local_device BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_device);
ALTER TABLE wb_agent_runtime ADD CONSTRAINT wb_agent_runtime_pkey PRIMARY KEY (local_device);

-- +goose Down
-- Old code-based runtime lookups cannot safely consume later snapshots.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local device records migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
