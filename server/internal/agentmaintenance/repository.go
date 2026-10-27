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
	Name           string
	Notes          string
	CreatedBy      *int64
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
// manually created restore points. Remote maintenance depends on this interface so
// tests can substitute it.
type ConfigStore interface {
	SaveEffectiveConfig(ctx context.Context, scaleCode string, effective p.EffectiveConfig) error
	CreateConfigBackup(ctx context.Context, scaleCode string, backup p.ConfigBackup, name, notes string, actor int64) (int64, error)
	ListConfigBackups(ctx context.Context, scaleCode string, offset, limit int) ([]BackupRecord, int64, error)
	DeleteConfigBackup(ctx context.Context, scaleCode string, id int64) error
	ConfigBackupByID(ctx context.Context, scaleCode string, id int64) (BackupRecord, error)
	AgentRuntime(ctx context.Context, scaleCode string) (RuntimeRecord, bool, error)
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) SaveEffectiveConfig(ctx context.Context, scaleCode string, effective p.EffectiveConfig) error {
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
	result := r.db.WithContext(ctx).Exec(`
INSERT INTO wb_agent_runtime
    (scale_code, effective_config_version, effective_config_digest, effective_settings, effective_applied_at, device_state, update_time)
SELECT ?, ?, ?, CAST(? AS jsonb), ?, ?, CURRENT_TIMESTAMP
WHERE EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?)
ON CONFLICT (local_device) DO UPDATE SET
    scale_code = EXCLUDED.scale_code,
    effective_config_version = EXCLUDED.effective_config_version,
    effective_config_digest = EXCLUDED.effective_config_digest,
    effective_settings = EXCLUDED.effective_settings,
    effective_applied_at = EXCLUDED.effective_applied_at,
    device_state = EXCLUDED.device_state,
    update_time = CURRENT_TIMESTAMP`,
		scaleCode, effective.ConfigVersion, effective.Digest, string(settings), applied, deviceState, scaleCode)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLocalScaleChanged
	}
	return nil
}

type backupRow struct {
	ID             int64
	ConfigVersion  int64
	Digest         string
	Settings       []byte
	AppliedAt      time.Time
	Source         string
	RestoredFromID *int64
	ReceivedAt     time.Time
	Name           string
	Notes          string
	CreatedBy      *int64
}

func (row backupRow) record() (BackupRecord, error) {
	record := BackupRecord{ID: row.ID, ConfigVersion: row.ConfigVersion, Digest: row.Digest, AppliedAt: row.AppliedAt, Source: row.Source, RestoredFromID: row.RestoredFromID, ReceivedAt: row.ReceivedAt, Name: row.Name, Notes: row.Notes, CreatedBy: row.CreatedBy}
	err := json.Unmarshal(row.Settings, &record.Settings)
	return record, err
}
func (r *Repository) CreateConfigBackup(ctx context.Context, scaleCode string, backup p.ConfigBackup, name, notes string, actor int64) (int64, error) {
	settings, err := json.Marshal(backup.Settings)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.WithContext(ctx).Raw(`INSERT INTO wb_agent_config_backup
 (scale_code, config_version, digest, settings, applied_at, source, name, notes, created_by)
 SELECT ?, ?, ?, CAST(? AS jsonb), ?, 'manual', ?, ?, ? WHERE EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?) RETURNING id`,
		scaleCode, backup.ConfigVersion, backup.Digest, string(settings), backup.AppliedAt, name, notes, actor, scaleCode).Scan(&id).Error
	if err == nil && id == 0 {
		return 0, ErrLocalScaleChanged
	}
	return id, err
}
func (r *Repository) ListConfigBackups(ctx context.Context, scaleCode string, offset, limit int) ([]BackupRecord, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM wb_agent_config_backup WHERE deleted_at IS NULL AND EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?)`, scaleCode).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []backupRow
	if err := r.db.WithContext(ctx).Raw(`SELECT id, config_version, digest, settings, applied_at, source, restored_from_id, received_at, name, notes, created_by FROM wb_agent_config_backup WHERE deleted_at IS NULL AND EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?) ORDER BY received_at DESC, id DESC OFFSET ? LIMIT ?`, scaleCode, offset, limit).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	records := make([]BackupRecord, 0, len(rows))
	for _, row := range rows {
		record, err := row.record()
		if err != nil {
			return nil, 0, err
		}
		records = append(records, record)
	}
	return records, total, nil
}
func (r *Repository) ConfigBackupByID(ctx context.Context, scaleCode string, id int64) (BackupRecord, error) {
	var row backupRow
	if err := r.db.WithContext(ctx).Raw(`SELECT id, config_version, digest, settings, applied_at, source, restored_from_id, received_at, name, notes, created_by FROM wb_agent_config_backup WHERE id=? AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?)`, id, scaleCode).Scan(&row).Error; err != nil {
		return BackupRecord{}, err
	}
	if row.ID == 0 {
		return BackupRecord{}, ErrNoBackup
	}
	return row.record()
}
func (r *Repository) DeleteConfigBackup(ctx context.Context, scaleCode string, id int64) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE wb_agent_config_backup SET deleted_at=CURRENT_TIMESTAMP WHERE id=? AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?)`, id, scaleCode)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoBackup
	}
	return nil
}

func (r *Repository) AgentRuntime(ctx context.Context, scaleCode string) (RuntimeRecord, bool, error) {
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
FROM wb_agent_runtime WHERE local_device AND EXISTS (SELECT 1 FROM wb_local_identity WHERE id=1 AND scale_code=?)`, scaleCode).Scan(&row).Error
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
var ErrLocalScaleChanged = errors.New("本地地磅身份已变化，请刷新后重试")
