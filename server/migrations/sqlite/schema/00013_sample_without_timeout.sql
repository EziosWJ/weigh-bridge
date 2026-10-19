-- +goose NO TRANSACTION
-- +goose Up
-- Preserve sample history and references while allowing samples without a deadline.
PRAGMA foreign_keys=OFF;
BEGIN IMMEDIATE;
CREATE TABLE wb_sample_no_timeout_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scale_id INTEGER NOT NULL REFERENCES wb_scale(id) ON DELETE RESTRICT,
    owner_user_id INTEGER NOT NULL,
    client_id VARCHAR(100) NOT NULL,
    session_generation INTEGER NOT NULL,
    task_id INTEGER REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    target_record_id INTEGER,
    task_version INTEGER,
    business_hash VARCHAR(64) NOT NULL,
    business_snapshot TEXT NOT NULL,
    stage VARCHAR(10) NOT NULL CHECK (stage IN ('first','second')),
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('gross','tare')),
    plate VARCHAR(20) NOT NULL,
    source VARCHAR(20) NOT NULL CHECK (source IN ('simulated','agent')),
    weight_milli_tonne INTEGER NOT NULL CHECK (weight_milli_tonne>0),
    captured_at DATETIME NOT NULL,
    expires_at DATETIME,
    stable INTEGER NOT NULL CHECK (stable IN (0,1)),
    device_ok INTEGER NOT NULL CHECK (device_ok IN (0,1)),
    evidence_snapshot TEXT NOT NULL,
    plate_evidence TEXT NOT NULL,
    conditions_version INTEGER NOT NULL,
    invalidated_at DATETIME,
    FOREIGN KEY (task_id,target_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT
);
INSERT INTO wb_sample_no_timeout_new SELECT * FROM wb_sample;
DROP TABLE wb_sample;
ALTER TABLE wb_sample_no_timeout_new RENAME TO wb_sample;
CREATE INDEX idx_wb_sample_scale_time ON wb_sample(scale_id,captured_at,id);
COMMIT;
PRAGMA foreign_keys=ON;

-- +goose Down
PRAGMA foreign_keys=OFF;
BEGIN IMMEDIATE;
UPDATE wb_sample SET expires_at=datetime(captured_at, '+30 seconds') WHERE expires_at IS NULL;
CREATE TABLE wb_sample_no_timeout_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scale_id INTEGER NOT NULL REFERENCES wb_scale(id) ON DELETE RESTRICT,
    owner_user_id INTEGER NOT NULL,
    client_id VARCHAR(100) NOT NULL,
    session_generation INTEGER NOT NULL,
    task_id INTEGER REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    target_record_id INTEGER,
    task_version INTEGER,
    business_hash VARCHAR(64) NOT NULL,
    business_snapshot TEXT NOT NULL,
    stage VARCHAR(10) NOT NULL CHECK (stage IN ('first','second')),
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('gross','tare')),
    plate VARCHAR(20) NOT NULL,
    source VARCHAR(20) NOT NULL CHECK (source IN ('simulated','agent')),
    weight_milli_tonne INTEGER NOT NULL CHECK (weight_milli_tonne>0),
    captured_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    stable INTEGER NOT NULL CHECK (stable IN (0,1)),
    device_ok INTEGER NOT NULL CHECK (device_ok IN (0,1)),
    evidence_snapshot TEXT NOT NULL,
    plate_evidence TEXT NOT NULL,
    conditions_version INTEGER NOT NULL,
    invalidated_at DATETIME,
    FOREIGN KEY (task_id,target_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT
);
INSERT INTO wb_sample_no_timeout_new SELECT * FROM wb_sample;
DROP TABLE wb_sample;
ALTER TABLE wb_sample_no_timeout_new RENAME TO wb_sample;
CREATE INDEX idx_wb_sample_scale_time ON wb_sample(scale_id,captured_at,id);
COMMIT;
PRAGMA foreign_keys=ON;
