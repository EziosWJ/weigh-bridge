-- +goose Up
ALTER TABLE wb_ticket_sync_change
    DROP CONSTRAINT wb_ticket_sync_change_status_check,
    ALTER COLUMN status TYPE VARCHAR(24),
    ADD CHECK (status IN ('pending','synced','failed','needs_admin'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'The legacy worker can resend held ticket changes; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
