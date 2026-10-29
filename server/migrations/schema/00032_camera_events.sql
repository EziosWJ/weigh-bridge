-- +goose Up
CREATE TABLE wb_camera_event_binding (
    camera_id BIGINT PRIMARY KEY REFERENCES wb_camera(id),
    serial_number VARCHAR(200) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    timeout_seconds INTEGER NOT NULL DEFAULT 180 CHECK (timeout_seconds BETWEEN 10 AND 86400),
    last_heartbeat_at TIMESTAMPTZ,
    device_time VARCHAR(200) NOT NULL DEFAULT '',
    device_ip VARCHAR(200) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX wb_camera_event_serial_unique ON wb_camera_event_binding(serial_number) WHERE serial_number <> '';
CREATE TABLE wb_camera_event_diagnostic (
    id BIGSERIAL PRIMARY KEY,
    camera_id BIGINT NOT NULL REFERENCES wb_camera(id),
    operator VARCHAR(200) NOT NULL,
    event_id TEXT NOT NULL,
    device_time VARCHAR(200) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    diagnostic JSONB NOT NULL
);
CREATE INDEX wb_camera_event_diagnostic_camera_id_idx ON wb_camera_event_diagnostic(camera_id, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Camera event data cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
