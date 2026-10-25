-- +goose Up
-- Retire replication and central approval state. Existing installations must
-- use the explicitly reset local database; business rows are never reassigned.
DROP TABLE wb_site_proposal;
DROP TABLE wb_temporary_party;
DROP TABLE wb_sync_state;
DROP TABLE wb_sync_digest;
DROP TABLE wb_node_identity;
ALTER TABLE wb_operation DROP COLUMN origin_request_id;
CREATE TABLE wb_local_identity (
 id BIGINT PRIMARY KEY CHECK (id=1),
 site_id BIGINT NOT NULL UNIQUE REFERENCES wb_site(id) ON DELETE RESTRICT CHECK (site_id BETWEEN 1 AND 255),
 next_id BIGINT NOT NULL DEFAULT 0 CHECK (next_id>=0),
 initialized SMALLINT NOT NULL DEFAULT 0 CHECK (initialized IN (0,1))
);
CREATE TABLE wb_local_admin_bootstrap (
 id BIGINT PRIMARY KEY CHECK (id=1),
 initialized_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
-- Identity and approval history cannot be reconstructed by a schema rollback.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local-only cutover requires restoring the previous database backup; automatic rollback is unsupported'; END $$;
-- +goose StatementEnd
