-- +goose Up
-- Serial QR scanner configuration and the heartbeat/status reported by the
-- local serial-agent process. Both tables are singletons: the factory runs one
-- scanner on the deployment host.
CREATE TABLE wb_serial_scanner (
  id BIGINT PRIMARY KEY CHECK (id = 1),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  device TEXT NOT NULL DEFAULT '/dev/ttyUSB0',
  baud_rate INTEGER NOT NULL DEFAULT 9600,
  data_bits INTEGER NOT NULL DEFAULT 8 CHECK (data_bits BETWEEN 5 AND 8),
  stop_bits INTEGER NOT NULL DEFAULT 1 CHECK (stop_bits IN (1, 2)),
  parity TEXT NOT NULL DEFAULT 'none' CHECK (parity IN ('none', 'even', 'odd')),
  delimiter TEXT NOT NULL DEFAULT '\r\n',
  trim_prefix TEXT NOT NULL DEFAULT '',
  trim_suffix TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 2 CHECK (retry_count BETWEEN 0 AND 10),
  retry_interval_ms INTEGER NOT NULL DEFAULT 500 CHECK (retry_interval_ms BETWEEN 100 AND 10000),
  version BIGINT NOT NULL DEFAULT 1,
  update_by BIGINT NOT NULL DEFAULT 0,
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE wb_serial_scanner_status (
  id BIGINT PRIMARY KEY CHECK (id = 1),
  agent_id TEXT NOT NULL DEFAULT '',
  pid INTEGER,
  connected BOOLEAN NOT NULL DEFAULT FALSE,
  device TEXT NOT NULL DEFAULT '',
  config_version BIGINT NOT NULL DEFAULT 0,
  scans BIGINT NOT NULL DEFAULT 0,
  failures BIGINT NOT NULL DEFAULT 0,
  last_scan_at TIMESTAMPTZ,
  last_scan_result TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  heartbeat_at TIMESTAMPTZ,
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE wb_mobile_handoff ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'operator';
ALTER TABLE wb_mobile_handoff ADD COLUMN device VARCHAR(120) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE wb_mobile_handoff DROP COLUMN IF EXISTS device;
ALTER TABLE wb_mobile_handoff DROP COLUMN IF EXISTS source;
DROP TABLE IF EXISTS wb_serial_scanner_status;
DROP TABLE IF EXISTS wb_serial_scanner;
