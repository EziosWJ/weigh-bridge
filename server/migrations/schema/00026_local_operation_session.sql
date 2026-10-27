-- +goose Up
-- One local operation session survives a business scale-code correction.
-- The stored code is a label, not a device association or ownership key.
ALTER TABLE wb_scale_session DROP CONSTRAINT fk_wb_session_scale;
ALTER TABLE wb_scale_session DROP CONSTRAINT wb_scale_session_pkey;
ALTER TABLE wb_scale_session ADD COLUMN local_device BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_device);
ALTER TABLE wb_scale_session ADD CONSTRAINT wb_scale_session_pkey PRIMARY KEY (local_device);

-- +goose Down
-- Later session snapshots may not refer to the current scale code.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local operation session migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
