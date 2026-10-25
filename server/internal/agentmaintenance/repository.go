package agentmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"gorm.io/gorm"
)

// BackupRecord is the Server-held recovery copy of one successfully applied
// Agent configuration. It never contains connection identity or runtime state.
type BackupRecord struct {
	ID             int64
	ConfigVersion  int64
	Digest         string
	Settings       p.DeviceSettings
	AppliedAt      time.Time
	Source         string
	RestoredFromID *int64
	ReceivedAt     time.Time
}

// RuntimeRecord is the persisted last-known Agent runtime report. Online state
// still comes from the live hub; these fields support diagnosis and offline
// read-only display.
type RuntimeRecord struct {
	PublicID               string
	ProtocolVersion        int
	AgentVersion           string
	Hostname               string
	Connected              bool
	Capabilities           []string
	EffectiveConfigVersion int64
	EffectiveConfigDigest  string
	EffectiveSettings      p.DeviceSettings
	EffectiveAppliedAt     *time.Time
	DeviceState            string
	UpdateTime             time.Time
}

// ConfigStore persists the Agent-reported effective configuration and the
// success backup. Remote maintenance depends only on this small interface so
// tests can substitute it.
type ConfigStore interface {
	SaveEffectiveConfig(ctx context.Context, scale int64, effective p.EffectiveConfig) error
	SaveConfigBackup(ctx context.Context, scale int64, backup p.ConfigBackup, source string, restoredFrom int64) (int64, error)
	LatestConfigBackup(ctx context.Context, scale int64) (BackupRecord, error)
	ConfigBackupByID(ctx context.Context, scale, id int64) (BackupRecord, error)
	AgentRuntime(ctx context.Context, scale int64) (RuntimeRecord, bool, error)
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) SaveEffectiveConfig(ctx context.Context, scale int64, effective p.EffectiveConfig) error {
	settings, err := json.Marshal(p.DeviceSettings{Serial: effective.Serial, Print: effective.Print})
	if err != nil {
		return err
	}
	var applied any
	if !effective.AppliedAt.IsZero() {
		applied = effective.AppliedAt
	}
	deviceState := effective.DeviceState
	if deviceState == "" {
		deviceState = "unknown"
	}
	return r.db.WithContext(ctx).Exec(`
INSERT INTO wb_agent_runtime
    (scale_id, effective_config_version, effective_config_digest, effective_settings, effective_applied_at, device_state, update_time)
VALUES (?, ?, ?, CAST(? AS jsonb), ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (scale_id) DO UPDATE SET
    effective_config_version = EXCLUDED.effective_config_version,
    effective_config_digest = EXCLUDED.effective_config_digest,
    effective_settings = EXCLUDED.effective_settings,
    effective_applied_at = EXCLUDED.effective_applied_at,
    device_state = EXCLUDED.device_state,
    update_time = CURRENT_TIMESTAMP`,
		scale, effective.ConfigVersion, effective.Digest, string(settings), applied, deviceState).Error
}

func (r *Repository) SaveConfigBackup(ctx context.Context, scale int64, backup p.ConfigBackup, source string, restoredFrom int64) (int64, error) {
	if source != "restore" {
		source = "auto"
	}
	settings, err := json.Marshal(backup.Settings)
	if err != nil {
		return 0, err
	}
	appliedAt := backup.AppliedAt
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	var restored, requested any
	if restoredFrom > 0 {
		restored, requested = appliedAt, time.Now().UTC()
	}
	if err := r.db.WithContext(ctx).Exec(`
INSERT INTO wb_agent_config_backup
    (scale_id, config_version, digest, settings, applied_at, source, restored_from_id, restore_requested_at, restored_at)
VALUES (?, ?, ?, CAST(? AS jsonb), ?, ?, NULLIF(?, 0), ?, ?)
ON CONFLICT (scale_id, config_version, digest) DO UPDATE SET
    source = CASE WHEN EXCLUDED.source='restore' THEN 'restore' ELSE wb_agent_config_backup.source END,
    restored_from_id = COALESCE(EXCLUDED.restored_from_id, wb_agent_config_backup.restored_from_id),
    restore_requested_at = COALESCE(EXCLUDED.restore_requested_at, wb_agent_config_backup.restore_requested_at),
    restored_at = COALESCE(EXCLUDED.restored_at, wb_agent_config_backup.restored_at)`,
		scale, backup.ConfigVersion, backup.Digest, string(settings), appliedAt, source, restoredFrom, requested, restored).Error; err != nil {
		return 0, err
	}
	var row struct{ ID int64 }
	err = r.db.WithContext(ctx).Raw(`
SELECT id FROM wb_agent_config_backup
WHERE scale_id=? AND config_version=? AND digest=?
ORDER BY id DESC LIMIT 1`, scale, backup.ConfigVersion, backup.Digest).Scan(&row).Error
	return row.ID, err
}

func (r *Repository) LatestConfigBackup(ctx context.Context, scale int64) (BackupRecord, error) {
	var row struct {
		ID             int64
		ConfigVersion  int64
		Digest         string
		Settings       []byte
		AppliedAt      time.Time
		Source         string
		RestoredFromID *int64
		ReceivedAt     time.Time
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT id, config_version, digest, settings, applied_at, source, restored_from_id, received_at
FROM wb_agent_config_backup WHERE scale_id=?
ORDER BY applied_at DESC, id DESC LIMIT 1`, scale).Scan(&row).Error
	if err != nil {
		return BackupRecord{}, err
	}
	if row.ID == 0 {
		return BackupRecord{}, ErrNoBackup
	}
	return backupFromRow(row.ID, row.ConfigVersion, row.Digest, row.Settings, row.AppliedAt, row.Source, row.RestoredFromID, row.ReceivedAt)
}

func (r *Repository) ConfigBackupByID(ctx context.Context, scale, id int64) (BackupRecord, error) {
	var row struct {
		ID             int64
		ConfigVersion  int64
		Digest         string
		Settings       []byte
		AppliedAt      time.Time
		Source         string
		RestoredFromID *int64
		ReceivedAt     time.Time
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT id, config_version, digest, settings, applied_at, source, restored_from_id, received_at
FROM wb_agent_config_backup WHERE scale_id=? AND id=?`, scale, id).Scan(&row).Error
	if err != nil {
		return BackupRecord{}, err
	}
	if row.ID == 0 {
		return BackupRecord{}, ErrNoBackup
	}
	return backupFromRow(row.ID, row.ConfigVersion, row.Digest, row.Settings, row.AppliedAt, row.Source, row.RestoredFromID, row.ReceivedAt)
}

func backupFromRow(id, version int64, digest string, raw []byte, appliedAt time.Time, source string, restoredFrom *int64, receivedAt time.Time) (BackupRecord, error) {
	record := BackupRecord{ID: id, ConfigVersion: version, Digest: digest, AppliedAt: appliedAt, Source: source, RestoredFromID: restoredFrom, ReceivedAt: receivedAt}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &record.Settings); err != nil {
			return BackupRecord{}, err
		}
	}
	return record, nil
}

func (r *Repository) AgentRuntime(ctx context.Context, scale int64) (RuntimeRecord, bool, error) {
	var row struct {
		PublicID               string
		ProtocolVersion        int
		AgentVersion           string
		Hostname               string
		Connected              int
		Capabilities           []byte
		EffectiveConfigVersion int64
		EffectiveConfigDigest  string
		EffectiveSettings      []byte
		EffectiveAppliedAt     *time.Time
		DeviceState            string
		UpdateTime             time.Time
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT public_id, protocol_version, agent_version, hostname, connected, capabilities,
       effective_config_version, effective_config_digest, effective_settings,
       effective_applied_at, device_state, update_time
FROM wb_agent_runtime WHERE scale_id=?`, scale).Scan(&row).Error
	if err != nil {
		return RuntimeRecord{}, false, err
	}
	if row.UpdateTime.IsZero() {
		return RuntimeRecord{}, false, nil
	}
	out := RuntimeRecord{
		PublicID: row.PublicID, ProtocolVersion: row.ProtocolVersion, AgentVersion: row.AgentVersion,
		Hostname: row.Hostname, Connected: row.Connected == 1, EffectiveConfigVersion: row.EffectiveConfigVersion,
		EffectiveConfigDigest: row.EffectiveConfigDigest, EffectiveAppliedAt: row.EffectiveAppliedAt,
		DeviceState: row.DeviceState, UpdateTime: row.UpdateTime,
	}
	if len(row.Capabilities) > 0 {
		_ = json.Unmarshal(row.Capabilities, &out.Capabilities)
	}
	if len(row.EffectiveSettings) > 0 {
		_ = json.Unmarshal(row.EffectiveSettings, &out.EffectiveSettings)
	}
	return out, true, nil
}

var ErrNoBackup = errors.New("尚无可恢复的成功配置备份")
