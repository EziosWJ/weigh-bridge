-- +goose Up
-- This identifier only isolates the Agent's durable print ledger between
-- database installations; it is not a factory or business-data identity.
-- Restoring the database preserves it, while a fresh database gets a new one.
ALTER TABLE wb_local_identity ADD COLUMN installation_id UUID NOT NULL DEFAULT gen_random_uuid();

-- +goose Down
-- New Agent ledgers may already be bound to this identifier.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Server installation identity migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
