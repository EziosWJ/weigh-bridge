-- +goose Up
ALTER TABLE wb_print_attempt ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'factory';
ALTER TABLE wb_print_attempt ADD COLUMN actor_name VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE wb_print_attempt ADD COLUMN scan_verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE wb_print_attempt ADD COLUMN scan_token_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE wb_print_attempt ADD COLUMN cloud_request_id VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE wb_print_attempt DROP CONSTRAINT wb_print_attempt_resolution_source_check;
ALTER TABLE wb_print_attempt ADD CONSTRAINT wb_print_attempt_resolution_source_check CHECK (resolution_source IN ('operator','agent','driver'));
CREATE UNIQUE INDEX uk_wb_print_cloud_request ON wb_print_attempt(cloud_request_id) WHERE cloud_request_id <> '';

-- One dirty marker per attempt. Any create/result/verify call reopens it so
-- the factory→cloud attempt mirror eventually converges on the latest state.
CREATE TABLE wb_print_sync_outbox (
  attempt_id BIGINT PRIMARY KEY REFERENCES wb_print_attempt(id) ON DELETE CASCADE,
  synced_at TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wb_print_sync_pending ON wb_print_sync_outbox(updated_at) WHERE synced_at IS NULL;

-- +goose Down
DROP TABLE wb_print_sync_outbox;
DROP INDEX IF EXISTS uk_wb_print_cloud_request;
ALTER TABLE wb_print_attempt DROP CONSTRAINT wb_print_attempt_resolution_source_check;
ALTER TABLE wb_print_attempt ADD CONSTRAINT wb_print_attempt_resolution_source_check CHECK (resolution_source IN ('operator','agent'));
ALTER TABLE wb_print_attempt DROP COLUMN cloud_request_id;
ALTER TABLE wb_print_attempt DROP COLUMN scan_token_hash;
ALTER TABLE wb_print_attempt DROP COLUMN scan_verified;
ALTER TABLE wb_print_attempt DROP COLUMN actor_name;
ALTER TABLE wb_print_attempt DROP COLUMN source;
