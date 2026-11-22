-- +goose Up
ALTER TABLE wb_master_sync_log ADD COLUMN task_no VARCHAR(40) NOT NULL DEFAULT '';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Task sync logs contain task references; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
