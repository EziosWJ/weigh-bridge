package camera

import (
	"context"
	"encoding/json"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventBinding struct {
	CameraID        int64      `json:"cameraId" gorm:"primaryKey"`
	SerialNumber    string     `json:"serialNumber"`
	Enabled         bool       `json:"enabled"`
	TimeoutSeconds  int        `json:"timeoutSeconds"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt"`
	DeviceTime      string     `json:"deviceTime"`
	DeviceIP        string     `json:"deviceIp"`
}

func (EventBinding) TableName() string { return "wb_camera_event_binding" }

type EventDiagnostic struct {
	ID         int64           `json:"id" gorm:"primaryKey"`
	CameraID   int64           `json:"cameraId"`
	Operator   string          `json:"operator"`
	EventID    string          `json:"eventId"`
	DeviceTime string          `json:"deviceTime"`
	ReceivedAt time.Time       `json:"receivedAt"`
	SHA256     string          `json:"sha256"`
	Diagnostic json.RawMessage `json:"diagnostic" gorm:"type:jsonb"`
}

func (EventDiagnostic) TableName() string { return "wb_camera_event_diagnostic" }

type eventStore interface {
	EventBinding(context.Context, int64) (EventBinding, error)
	SaveEventBinding(context.Context, EventBinding, audit.Event) error
	ReceiveEvent(context.Context, string, string, string, string, string, time.Time, string, json.RawMessage) error
	EventDiagnostics(context.Context, int64) ([]EventDiagnostic, error)
}

func (r *Repository) EventBinding(ctx context.Context, id int64) (EventBinding, error) {
	if _, err := r.Camera(ctx, id); err != nil {
		return EventBinding{}, err
	}
	var row EventBinding
	err := r.db.WithContext(ctx).Where("camera_id=?", id).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return EventBinding{CameraID: id, TimeoutSeconds: 180}, nil
	}
	return row, err
}
func (r *Repository) SaveEventBinding(ctx context.Context, row EventBinding, event audit.Event) error {
	return normalizeDBError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var camera cameraRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0", row.CameraID).First(&camera).Error; err != nil {
			return normalizeDBError(err)
		}
		var old EventBinding
		err := tx.Where("camera_id=?", row.CameraID).First(&old).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if old.SerialNumber == row.SerialNumber && old.Enabled == row.Enabled {
			row.LastHeartbeatAt = old.LastHeartbeatAt
			row.DeviceTime = old.DeviceTime
			row.DeviceIP = old.DeviceIP
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "camera_id"}}, UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
		event.Action = "camera.event_binding.update"
		event.Resource = "weighing.camera"
		event.ResourceID = row.CameraID
		return audit.RecordOn(ctx, tx, event)
	}))
}
func (r *Repository) ReceiveEvent(ctx context.Context, serial, operator, eventID, deviceTime, deviceIP string, now time.Time, hash string, diagnostic json.RawMessage) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row EventBinding
		if err := tx.Where("serial_number=? AND enabled=true", serial).First(&row).Error; err != nil {
			return normalizeDBError(err)
		}
		var camera cameraRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0 AND enabled=1", row.CameraID).First(&camera).Error; err != nil {
			return normalizeDBError(err)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("camera_id=? AND serial_number=? AND enabled=true", row.CameraID, serial).First(&row).Error; err != nil {
			return normalizeDBError(err)
		}
		if operator == "heartbeat" {
			return tx.Model(&row).Updates(map[string]any{"last_heartbeat_at": now, "device_time": deviceTime, "device_ip": deviceIP}).Error
		}
		d := EventDiagnostic{CameraID: row.CameraID, Operator: operator, EventID: eventID, DeviceTime: deviceTime, ReceivedAt: now, SHA256: hash, Diagnostic: diagnostic}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM wb_camera_event_diagnostic WHERE camera_id = ? AND id NOT IN (SELECT id FROM wb_camera_event_diagnostic WHERE camera_id = ? ORDER BY id DESC LIMIT 100)", row.CameraID, row.CameraID).Error
	})
}
func (r *Repository) EventDiagnostics(ctx context.Context, id int64) ([]EventDiagnostic, error) {
	if _, err := r.Camera(ctx, id); err != nil {
		return nil, err
	}
	var rows []EventDiagnostic
	err := r.db.WithContext(ctx).Where("camera_id=?", id).Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}
