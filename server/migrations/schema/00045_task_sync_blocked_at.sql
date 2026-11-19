-- +goose Up
ALTER TABLE wb_task_sync_outbox ADD COLUMN blocked_at TIMESTAMPTZ;
DROP INDEX wb_task_sync_outbox_due;
CREATE INDEX wb_task_sync_outbox_due ON wb_task_sync_outbox (next_attempt_at, id)
    WHERE confirmed_at IS NULL AND blocked_at IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'The previous worker can resend blocked task changes; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
