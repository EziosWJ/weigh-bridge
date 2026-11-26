package serialscanner

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Config(ctx context.Context) (Config, error) {
	var cfg Config
	err := r.db.WithContext(ctx).Where("id = 1").Take(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SaveConfig upserts the singleton row, bumps its version, and returns the
// persisted values so the caller can immediately display the new revision.
func (r *Repository) SaveConfig(ctx context.Context, in ConfigInput, actor int64) (Config, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Exec(`
INSERT INTO wb_serial_scanner
    (id, enabled, device, baud_rate, data_bits, stop_bits, parity, delimiter,
     trim_prefix, trim_suffix, retry_count, retry_interval_ms, version, update_by, update_time)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    device = EXCLUDED.device,
    baud_rate = EXCLUDED.baud_rate,
    data_bits = EXCLUDED.data_bits,
    stop_bits = EXCLUDED.stop_bits,
    parity = EXCLUDED.parity,
    delimiter = EXCLUDED.delimiter,
    trim_prefix = EXCLUDED.trim_prefix,
    trim_suffix = EXCLUDED.trim_suffix,
    retry_count = EXCLUDED.retry_count,
    retry_interval_ms = EXCLUDED.retry_interval_ms,
    version = wb_serial_scanner.version + 1,
    update_by = EXCLUDED.update_by,
    update_time = EXCLUDED.update_time`,
		in.Enabled, in.Device, in.BaudRate, in.DataBits, in.StopBits, in.Parity, in.Delimiter,
		in.TrimPrefix, in.TrimSuffix, in.RetryCount, in.RetryIntervalMS, actor, now,
	).Error
	if err != nil {
		return Config{}, err
	}
	return r.Config(ctx)
}

func (r *Repository) Status(ctx context.Context) (Status, error) {
	var status Status
	err := r.db.WithContext(ctx).Where("id = 1").Take(&status).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	return status, nil
}

func (r *Repository) Heartbeat(ctx context.Context, in HeartbeatInput, now time.Time) error {
	var pid any
	if in.PID > 0 {
		pid = in.PID
	}
	var lastScanAt any
	if in.LastScanAt != nil {
		lastScanAt = *in.LastScanAt
	}
	return r.db.WithContext(ctx).Exec(`
INSERT INTO wb_serial_scanner_status
    (id, agent_id, pid, connected, device, config_version, scans, failures,
     last_scan_at, last_scan_result, last_error, heartbeat_at, update_time)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    agent_id = EXCLUDED.agent_id,
    pid = EXCLUDED.pid,
    connected = EXCLUDED.connected,
    device = EXCLUDED.device,
    config_version = EXCLUDED.config_version,
    scans = EXCLUDED.scans,
    failures = EXCLUDED.failures,
    last_scan_at = COALESCE(EXCLUDED.last_scan_at, wb_serial_scanner_status.last_scan_at),
    last_scan_result = CASE WHEN EXCLUDED.last_scan_result = '' THEN wb_serial_scanner_status.last_scan_result ELSE EXCLUDED.last_scan_result END,
    last_error = EXCLUDED.last_error,
    heartbeat_at = EXCLUDED.heartbeat_at,
    update_time = EXCLUDED.update_time`,
		in.AgentID, pid, in.Connected, in.Device, in.ConfigVersion, in.Scans, in.Failures,
		lastScanAt, in.LastScanResult, in.LastError, now, now,
	).Error
}
