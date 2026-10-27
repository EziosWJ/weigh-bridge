-- +goose Up
-- The single local Agent association is not keyed by a mutable scale code.
-- Keep the old code as a historical label and preserve its existing Agent ID.
ALTER TABLE wb_agent_binding DROP CONSTRAINT fk_wb_binding_scale;
ALTER TABLE wb_agent_binding DROP CONSTRAINT wb_agent_binding_pkey;
ALTER TABLE wb_agent_binding ADD COLUMN local_device BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_device);
ALTER TABLE wb_agent_binding ADD CONSTRAINT wb_agent_binding_pkey PRIMARY KEY (local_device);

-- +goose Down
-- A later code correction may leave the legacy label different from the current code.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local Agent binding migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
