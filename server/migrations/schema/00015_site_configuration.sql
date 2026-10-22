-- +goose Up
ALTER TABLE wb_site ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
CREATE TABLE wb_agent_binding (
 scale_id BIGINT PRIMARY KEY REFERENCES wb_scale(id),
 agent_id VARCHAR(80) NOT NULL UNIQUE,
 agent_key TEXT NOT NULL
);
CREATE TABLE wb_agent_setting (
 id BIGINT PRIMARY KEY CHECK (id=1),
 server_url TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE wb_agent_setting;
DROP TABLE wb_agent_binding;
ALTER TABLE wb_site DROP COLUMN version;
