-- +goose Up
CREATE TABLE wb_camera_license_snapshot (
    id BIGSERIAL PRIMARY KEY,
    camera_id BIGINT NOT NULL REFERENCES wb_camera(id),
    serial_number VARCHAR(200) NOT NULL,
    event_id TEXT NOT NULL,
    device_time VARCHAR(200) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    plate TEXT NOT NULL,
    normalized_plate TEXT NOT NULL,
    attributes JSONB NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    capture_bytes INTEGER NOT NULL,
    background_bytes INTEGER NOT NULL,
    UNIQUE (camera_id, sha256)
);
CREATE INDEX wb_camera_license_snapshot_recent ON wb_camera_license_snapshot(camera_id, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Camera snapshot data cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
