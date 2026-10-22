-- +goose Up
ALTER TABLE wb_site ADD COLUMN server_url TEXT NOT NULL DEFAULT '';
ALTER TABLE wb_operation ADD COLUMN origin_request_id VARCHAR(128) NOT NULL DEFAULT '';
CREATE TABLE wb_node_identity (
 id BIGINT PRIMARY KEY CHECK (id=1), role VARCHAR(10) NOT NULL,
 site_id BIGINT NOT NULL, next_id BIGINT NOT NULL DEFAULT 0,
 initialized INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE wb_sync_digest (
 site_id BIGINT NOT NULL, table_name VARCHAR(60) NOT NULL,
 row_id BIGINT NOT NULL, digest VARCHAR(64) NOT NULL,
 PRIMARY KEY (site_id,table_name,row_id)
);
CREATE TABLE wb_sync_state (
 site_id BIGINT PRIMARY KEY, received_seq BIGINT NOT NULL DEFAULT 0,
 sent_seq BIGINT NOT NULL DEFAULT 0, last_success TEXT NOT NULL DEFAULT '',
 last_error TEXT NOT NULL DEFAULT '', pending TEXT NOT NULL DEFAULT '',
 acknowledged_hash VARCHAR(64) NOT NULL DEFAULT '', pending_images INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE wb_temporary_party (
 party_id BIGINT PRIMARY KEY REFERENCES wb_party(id), site_id BIGINT NOT NULL,
 state VARCHAR(20) NOT NULL CHECK (state IN ('pending','returned','resubmitted','approved','linked')),
 revision BIGINT NOT NULL DEFAULT 1, reviewed_revision BIGINT NOT NULL DEFAULT 0,
 formal_party_id BIGINT REFERENCES wb_party(id), reason TEXT NOT NULL DEFAULT ''
);
CREATE TABLE wb_site_proposal (
 site_id BIGINT PRIMARY KEY, revision BIGINT NOT NULL, applied_revision BIGINT NOT NULL DEFAULT 0,
 configuration TEXT NOT NULL
);
-- +goose Down
DROP TABLE wb_site_proposal;
DROP TABLE wb_temporary_party;
DROP TABLE wb_sync_state;
DROP TABLE wb_sync_digest;
DROP TABLE wb_node_identity;
ALTER TABLE wb_operation DROP COLUMN origin_request_id;
ALTER TABLE wb_site DROP COLUMN server_url;
