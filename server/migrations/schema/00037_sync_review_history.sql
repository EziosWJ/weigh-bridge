-- +goose Up
ALTER TABLE wb_ticket_sync_review DROP CONSTRAINT wb_ticket_sync_review_ticket_id_digest_reason_code_key;
CREATE UNIQUE INDEX wb_ticket_sync_review_open_identity
    ON wb_ticket_sync_review(ticket_id, digest, reason_code) WHERE status = 'open';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Sync review history cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
