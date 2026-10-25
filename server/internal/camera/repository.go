package camera

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cameraRow struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	PublicID       string     `gorm:"column:public_id"`
	Name           string     `gorm:"column:name"`
	SourceURL      string     `gorm:"column:source_url"`
	Username       string     `gorm:"column:username"`
	Password       string     `gorm:"column:password"`
	CaptureEnabled int        `gorm:"column:capture_enabled"`
	Enabled        int        `gorm:"column:enabled"`
	Deleted        int        `gorm:"column:deleted"`
	Version        int64      `gorm:"column:version"`
	CreateTime     time.Time  `gorm:"column:create_time"`
	UpdateTime     time.Time  `gorm:"column:update_time"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

func (cameraRow) TableName() string { return "wb_camera" }

type streamRow struct {
	CameraID       int64      `gorm:"column:camera_id;primaryKey"`
	MediaPath      string     `gorm:"column:media_path"`
	DesiredVersion int64      `gorm:"column:desired_version"`
	SyncedVersion  int64      `gorm:"column:synced_version"`
	State          string     `gorm:"column:state"`
	LastError      string     `gorm:"column:last_error"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	SyncedAt       *time.Time `gorm:"column:synced_at"`
	RemovedAt      *time.Time `gorm:"column:removed_at"`
}

func (streamRow) TableName() string { return "wb_camera_stream" }

type settingsRow struct {
	ID                  int64     `gorm:"column:id;primaryKey"`
	ConfigVersion       int64     `gorm:"column:config_version"`
	CaptureTimeoutMS    int       `gorm:"column:capture_timeout_ms"`
	RecognitionURL      string    `gorm:"column:recognition_url"`
	RecognitionCameraID *int64    `gorm:"column:recognition_camera_id"`
	MediaMTXAPIURL      string    `gorm:"column:mediamtx_api_url"`
	MediaMTXAPIUsername string    `gorm:"column:mediamtx_api_username"`
	MediaMTXAPIPassword string    `gorm:"column:mediamtx_api_password"`
	MediaMTXWHEPBaseURL string    `gorm:"column:mediamtx_whep_base_url"`
	VideoVersion        int64     `gorm:"column:video_version"`
	VideoSyncState      string    `gorm:"column:video_sync_state"`
	VideoSyncError      string    `gorm:"column:video_sync_error"`
	VideoSyncedVersion  int64     `gorm:"column:video_synced_version"`
	UpdateTime          time.Time `gorm:"column:update_time"`
}

func (settingsRow) TableName() string { return "wb_camera_settings" }

type imageRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	ScaleID        int64     `gorm:"column:scale_id"`
	CameraID       *int64    `gorm:"column:camera_id"`
	CameraPublicID string    `gorm:"column:camera_public_id"`
	CameraName     string    `gorm:"column:camera_name"`
	CameraSource   string    `gorm:"column:camera_source"`
	Source         string    `gorm:"column:source"`
	View           string    `gorm:"column:view"`
	Status         string    `gorm:"column:status"`
	Error          string    `gorm:"column:error"`
	FileName       string    `gorm:"column:file_name"`
	SHA256         string    `gorm:"column:sha256"`
	Width          int       `gorm:"column:width"`
	Height         int       `gorm:"column:height"`
	Bytes          int64     `gorm:"column:bytes"`
	CapturedAt     time.Time `gorm:"column:captured_at"`
	CreateTime     time.Time `gorm:"column:create_time"`
}

func (imageRow) TableName() string { return "wb_weighing_image" }

// Repository persists the camera module in PostgreSQL. It does not start
// migrations; schema 00018 owns these tables.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func toCameraRecord(row cameraRow) cameraRecord {
	return cameraRecord{
		ID: row.ID, PublicID: row.PublicID, Name: row.Name, SourceURL: row.SourceURL,
		Username: row.Username, Password: row.Password,
		CaptureEnabled: row.CaptureEnabled == 1, Enabled: row.Enabled == 1,
		Deleted: row.Deleted == 1, Version: row.Version,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
}

func toStreamRecord(row streamRow) streamRecord {
	return streamRecord{
		CameraID: row.CameraID, MediaPath: row.MediaPath, DesiredVersion: row.DesiredVersion,
		SyncedVersion: row.SyncedVersion, State: row.State, LastError: row.LastError,
		UpdatedAt: row.UpdatedAt, SyncedAt: row.SyncedAt, RemovedAt: row.RemovedAt,
	}
}

func toSettingsRecord(row settingsRow) settingsRecord {
	return settingsRecord{
		ConfigVersion: row.ConfigVersion, CaptureTimeoutMS: row.CaptureTimeoutMS,
		RecognitionURL: row.RecognitionURL, RecognitionCameraID: row.RecognitionCameraID,
		MediaMTXAPIURL: row.MediaMTXAPIURL, MediaMTXAPIUsername: row.MediaMTXAPIUsername,
		MediaMTXAPIPassword: row.MediaMTXAPIPassword, MediaMTXWHEPBaseURL: row.MediaMTXWHEPBaseURL,
		VideoVersion: row.VideoVersion, VideoSyncState: row.VideoSyncState,
		VideoSyncError: row.VideoSyncError, VideoSyncedVersion: row.VideoSyncedVersion,
		UpdateTime: row.UpdateTime,
	}
}

func toImageRecord(row imageRow) imageRecord {
	return imageRecord{
		ID: row.ID, ScaleID: row.ScaleID, CameraID: row.CameraID, CameraPublicID: row.CameraPublicID,
		CameraName: row.CameraName, CameraSource: row.CameraSource, Source: row.Source,
		View: row.View, Status: row.Status, Error: row.Error, FileName: row.FileName,
		SHA256: row.SHA256, Width: row.Width, Height: row.Height, Bytes: row.Bytes,
		CapturedAt: row.CapturedAt, CreateTime: row.CreateTime,
	}
}

func normalizeDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

// repeatableRead gives a single coherent read snapshot for the camera set and
// the site config version. It does not hold locks across network capture.
func repeatableRead(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ").Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

func readSettings(tx *gorm.DB) (settingsRow, error) {
	var row settingsRow
	err := tx.Where("id=1").First(&row).Error
	return row, normalizeDBError(err)
}

func ensureCameraNameFree(tx *gorm.DB, name string, excludeID int64) error {
	var count int64
	query := tx.Model(&cameraRow{}).Where("deleted=0 AND LOWER(name)=LOWER(?)", name)
	if excludeID > 0 {
		query = query.Where("id<>?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConflict
	}
	return nil
}

func bumpCameraConfig(tx *gorm.DB, now time.Time) error {
	return tx.Model(&settingsRow{}).Where("id=1").Updates(map[string]any{
		"config_version": gorm.Expr("config_version+1"),
		"update_time":    now,
	}).Error
}

func clearRecognitionCamera(tx *gorm.DB, cameraID int64) error {
	return tx.Model(&settingsRow{}).Where("id=1 AND recognition_camera_id=?", cameraID).
		UpdateColumn("recognition_camera_id", nil).Error
}

// ListState reads settings, cameras and streams from one repeatable-read
// snapshot. A capture plan therefore freezes a coherent camera set and config
// version even while an administrator saves another camera.
func (r *Repository) ListState(ctx context.Context) (settingsRecord, []cameraRecord, []streamRecord, error) {
	var setting settingsRow
	var cameras []cameraRow
	var streams []streamRow
	err := repeatableRead(ctx, r.db, func(tx *gorm.DB) error {
		var err error
		setting, err = readSettings(tx)
		if err != nil {
			return err
		}
		if err := tx.Where("deleted=0").Order("id").Find(&cameras).Error; err != nil {
			return err
		}
		return tx.Order("camera_id").Find(&streams).Error
	})
	if err != nil {
		return settingsRecord{}, nil, nil, err
	}
	cameraRecords := make([]cameraRecord, 0, len(cameras))
	for _, row := range cameras {
		cameraRecords = append(cameraRecords, toCameraRecord(row))
	}
	streamRecords := make([]streamRecord, 0, len(streams))
	for _, row := range streams {
		streamRecords = append(streamRecords, toStreamRecord(row))
	}
	return toSettingsRecord(setting), cameraRecords, streamRecords, nil
}

func (r *Repository) Camera(ctx context.Context, id int64) (cameraRecord, error) {
	var row cameraRow
	err := r.db.WithContext(ctx).Where("id=? AND deleted=0", id).First(&row).Error
	if err != nil {
		return cameraRecord{}, normalizeDBError(err)
	}
	return toCameraRecord(row), nil
}

func (r *Repository) CreateCamera(ctx context.Context, record cameraRecord, stream streamRecord, event audit.Event) (int64, settingsRecord, error) {
	now := time.Now().UTC()
	var setting settingsRow
	var id int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureCameraNameFree(tx, record.Name, 0); err != nil {
			return err
		}
		row := cameraRow{
			PublicID: record.PublicID, Name: record.Name, SourceURL: record.SourceURL,
			Username: record.Username, Password: record.Password,
			CaptureEnabled: boolInt(record.CaptureEnabled), Enabled: boolInt(record.Enabled),
			Version: 1, CreateTime: now, UpdateTime: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return normalizeDBError(err)
		}
		id = row.ID
		stream.CameraID = row.ID
		if stream.DesiredVersion < 1 {
			stream.DesiredVersion = 1
		}
		if stream.State == "" {
			stream.State = StreamPending
		}
		stream.UpdatedAt = now
		if err := tx.Create(&streamRow{
			CameraID: stream.CameraID, MediaPath: stream.MediaPath,
			DesiredVersion: stream.DesiredVersion, SyncedVersion: stream.SyncedVersion,
			State: stream.State, LastError: stream.LastError, UpdatedAt: stream.UpdatedAt,
		}).Error; err != nil {
			return normalizeDBError(err)
		}
		if err := bumpCameraConfig(tx, now); err != nil {
			return err
		}
		event.ResourceID = row.ID
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		var err error
		setting, err = readSettings(tx)
		return err
	})
	if err != nil {
		return 0, settingsRecord{}, err
	}
	return id, toSettingsRecord(setting), nil
}

func (r *Repository) UpdateCamera(ctx context.Context, id, expectedVersion int64, record cameraRecord, contentChanged, streamChanged bool, event audit.Event) (settingsRecord, error) {
	now := time.Now().UTC()
	var setting settingsRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current cameraRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0", id).First(&current).Error; err != nil {
			return normalizeDBError(err)
		}
		if current.Version != expectedVersion {
			return ErrConflict
		}
		if err := ensureCameraNameFree(tx, record.Name, id); err != nil {
			return err
		}
		nextVersion := current.Version + 1
		result := tx.Model(&cameraRow{}).Where("id=? AND version=? AND deleted=0", id, expectedVersion).Updates(map[string]any{
			"name":            record.Name,
			"source_url":      record.SourceURL,
			"username":        record.Username,
			"password":        record.Password,
			"capture_enabled": boolInt(record.CaptureEnabled),
			"enabled":         boolInt(record.Enabled),
			"version":         nextVersion,
			"update_time":     now,
		})
		if result.Error != nil {
			return normalizeDBError(result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrConflict
		}
		if streamChanged {
			state := StreamPending
			if !record.Enabled {
				state = StreamRemoving
			}
			result := tx.Model(&streamRow{}).Where("camera_id=?", id).Updates(map[string]any{
				"desired_version": nextVersion,
				"state":           state,
				"last_error":      "",
				"updated_at":      now,
				"removed_at":      nil,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := tx.Create(&streamRow{
					CameraID: id, MediaPath: managedPath(current.PublicID), DesiredVersion: nextVersion,
					State: state, UpdatedAt: now,
				}).Error; err != nil {
					return normalizeDBError(err)
				}
			}
		}
		if contentChanged {
			if err := bumpCameraConfig(tx, now); err != nil {
				return err
			}
		}
		if !record.CaptureEnabled || !record.Enabled {
			if err := clearRecognitionCamera(tx, id); err != nil {
				return err
			}
		}
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		var err error
		setting, err = readSettings(tx)
		return err
	})
	if err != nil {
		return settingsRecord{}, err
	}
	return toSettingsRecord(setting), nil
}

func (r *Repository) DeleteCamera(ctx context.Context, id, expectedVersion int64, event audit.Event) (settingsRecord, error) {
	now := time.Now().UTC()
	var setting settingsRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current cameraRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0", id).First(&current).Error; err != nil {
			return normalizeDBError(err)
		}
		if current.Version != expectedVersion {
			return ErrConflict
		}
		nextVersion := current.Version + 1
		result := tx.Model(&cameraRow{}).Where("id=? AND version=? AND deleted=0", id, expectedVersion).Updates(map[string]any{
			"deleted":     1,
			"deleted_at":  now,
			"version":     nextVersion,
			"update_time": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConflict
		}
		result = tx.Model(&streamRow{}).Where("camera_id=?", id).Updates(map[string]any{
			"desired_version": nextVersion,
			"state":           StreamRemoving,
			"last_error":      "",
			"updated_at":      now,
			"removed_at":      nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Create(&streamRow{
				CameraID: id, MediaPath: managedPath(current.PublicID), DesiredVersion: nextVersion,
				State: StreamRemoving, UpdatedAt: now,
			}).Error; err != nil {
				return normalizeDBError(err)
			}
		}
		if err := clearRecognitionCamera(tx, id); err != nil {
			return err
		}
		if err := bumpCameraConfig(tx, now); err != nil {
			return err
		}
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		var err error
		setting, err = readSettings(tx)
		return err
	})
	if err != nil {
		return settingsRecord{}, err
	}
	return toSettingsRecord(setting), nil
}

func (r *Repository) UpdateCaptureSettings(ctx context.Context, expectedVersion int64, timeoutMS int, recognitionCameraID *int64, recognitionURL string, contentChanged bool, event audit.Event) (settingsRecord, error) {
	now := time.Now().UTC()
	var setting settingsRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current settingsRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").First(&current).Error; err != nil {
			return normalizeDBError(err)
		}
		if current.ConfigVersion != expectedVersion {
			return ErrConflict
		}
		if recognitionCameraID != nil {
			var camera cameraRow
			if err := tx.Where("id=? AND deleted=0 AND enabled=1 AND capture_enabled=1", *recognitionCameraID).First(&camera).Error; err != nil {
				return ErrInvalid
			}
		}
		updates := map[string]any{
			"capture_timeout_ms":    timeoutMS,
			"recognition_url":       recognitionURL,
			"recognition_camera_id": recognitionCameraID,
			"update_time":           now,
		}
		if contentChanged {
			updates["config_version"] = gorm.Expr("config_version+1")
		}
		result := tx.Model(&settingsRow{}).Where("id=1 AND config_version=?", expectedVersion).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConflict
		}
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		var err error
		setting, err = readSettings(tx)
		return err
	})
	if err != nil {
		return settingsRecord{}, err
	}
	return toSettingsRecord(setting), nil
}

func (r *Repository) UpdateVideoSettings(ctx context.Context, expectedVersion int64, apiURL, apiUsername, apiPassword, whepBaseURL string, event audit.Event) (settingsRecord, error) {
	now := time.Now().UTC()
	var setting settingsRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current settingsRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").First(&current).Error; err != nil {
			return normalizeDBError(err)
		}
		if current.VideoVersion != expectedVersion {
			return ErrConflict
		}
		result := tx.Model(&settingsRow{}).Where("id=1 AND video_version=?", expectedVersion).Updates(map[string]any{
			"mediamtx_api_url":       apiURL,
			"mediamtx_api_username":  apiUsername,
			"mediamtx_api_password":  apiPassword,
			"mediamtx_whep_base_url": whepBaseURL,
			"video_version":          gorm.Expr("video_version+1"),
			"video_sync_state":       VideoSyncPending,
			"video_sync_error":       "",
			"update_time":            now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConflict
		}
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		var err error
		setting, err = readSettings(tx)
		return err
	})
	if err != nil {
		return settingsRecord{}, err
	}
	return toSettingsRecord(setting), nil
}

func (r *Repository) SaveImage(ctx context.Context, record imageRecord) error {
	row := imageRow{
		ID: record.ID, ScaleID: record.ScaleID, CameraID: record.CameraID,
		CameraPublicID: record.CameraPublicID, CameraName: record.CameraName,
		CameraSource: record.CameraSource, Source: record.Source, View: record.View,
		Status: record.Status, Error: record.Error, FileName: record.FileName,
		SHA256: record.SHA256, Width: record.Width, Height: record.Height,
		Bytes: record.Bytes, CapturedAt: record.CapturedAt, CreateTime: time.Now().UTC(),
	}
	return normalizeDBError(r.db.WithContext(ctx).Create(&row).Error)
}

func (r *Repository) Image(ctx context.Context, id string) (imageRecord, error) {
	var row imageRow
	if err := r.db.WithContext(ctx).Where("id=?", id).First(&row).Error; err != nil {
		return imageRecord{}, normalizeDBError(err)
	}
	return toImageRecord(row), nil
}

func (r *Repository) SyncSnapshot(ctx context.Context) (settingsRecord, []streamRecord, map[int64]cameraRecord, error) {
	var setting settingsRow
	var streams []streamRow
	cameras := map[int64]cameraRecord{}
	err := repeatableRead(ctx, r.db, func(tx *gorm.DB) error {
		var err error
		setting, err = readSettings(tx)
		if err != nil {
			return err
		}
		if err := tx.Where("state<>?", StreamRemoved).Order("camera_id").Find(&streams).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(streams))
		for _, row := range streams {
			ids = append(ids, row.CameraID)
		}
		if len(ids) == 0 {
			return nil
		}
		var rows []cameraRow
		if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			cameras[row.ID] = toCameraRecord(row)
		}
		return nil
	})
	if err != nil {
		return settingsRecord{}, nil, nil, err
	}
	streamRecords := make([]streamRecord, 0, len(streams))
	for _, row := range streams {
		streamRecords = append(streamRecords, toStreamRecord(row))
	}
	return toSettingsRecord(setting), streamRecords, cameras, nil
}

func (r *Repository) MarkStreamSynced(ctx context.Context, cameraID, desiredVersion int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&streamRow{}).
		Where("camera_id=? AND desired_version=? AND state<>?", cameraID, desiredVersion, StreamRemoved).
		Updates(map[string]any{
			"state":          StreamSynced,
			"synced_version": desiredVersion,
			"last_error":     "",
			"updated_at":     now,
			"synced_at":      now,
		}).Error
}

func (r *Repository) MarkStreamError(ctx context.Context, cameraID, desiredVersion int64, message string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&streamRow{}).
		Where("camera_id=? AND desired_version=? AND state<>?", cameraID, desiredVersion, StreamRemoved).
		Updates(map[string]any{
			"state":      StreamError,
			"last_error": message,
			"updated_at": now,
		}).Error
}

func (r *Repository) MarkStreamRemoved(ctx context.Context, cameraID, desiredVersion int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&streamRow{}).
		Where("camera_id=? AND desired_version=? AND state<>?", cameraID, desiredVersion, StreamRemoved).
		Updates(map[string]any{
			"state":          StreamRemoved,
			"synced_version": desiredVersion,
			"last_error":     "",
			"updated_at":     now,
			"removed_at":     now,
		}).Error
}

func (r *Repository) UpdateVideoSync(ctx context.Context, videoVersion int64, state, message string, success bool) error {
	updates := map[string]any{
		"video_sync_state": state,
		"video_sync_error": message,
		"update_time":      time.Now().UTC(),
	}
	if success {
		updates["video_synced_version"] = videoVersion
	}
	return r.db.WithContext(ctx).Model(&settingsRow{}).Where("id=1 AND video_version=?", videoVersion).Updates(updates).Error
}

func (r *Repository) RecordAudit(ctx context.Context, event audit.Event) error {
	return audit.RecordOn(ctx, r.db, event)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
