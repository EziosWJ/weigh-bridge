package weighing

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PlateConfirmationInput struct {
	ScaleCode string `json:"scaleCode"`
	SessionInput
	ConfirmedPlate string           `json:"confirmedPlate"`
	Evidence       RecognitionInput `json:"evidence"`
}

type plateConfirmation struct {
	ID                int64 `gorm:"primaryKey"`
	ActorID           int64
	ScaleCode         string
	SessionGeneration int64
	ConfirmedPlate    string
	Evidence          JSON `gorm:"type:jsonb"`
	ConfirmedAt       time.Time
}

func (plateConfirmation) TableName() string { return "wb_plate_confirmation" }

func (r *Repository) ConfirmPlate(ctx context.Context, in PlateConfirmationInput, event audit.Event) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := lockWriteSite(tx, ctx); err != nil {
			return err
		}
		session, err := lockSession(tx, in.ScaleCode)
		if err != nil {
			return err
		}
		if !validSession(session, event.Metadata.ActorID, in.SessionInput) {
			return ErrSession
		}
		confirmed := normalizePlate(in.ConfirmedPlate)
		if !businessPlate.MatchString(confirmed) || in.Evidence.Source != "server" {
			return ErrInvalid
		}
		now := time.Now().UTC()
		verified, err := validatePlateEvents(tx, &in.Evidence, confirmed, now)
		if err != nil {
			return err
		}
		row := plateConfirmation{ActorID: event.Metadata.ActorID, ScaleCode: in.ScaleCode, SessionGeneration: session.Generation, ConfirmedPlate: confirmed, Evidence: snapshot(verified), ConfirmedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	return id, err
}

func readPlateConfirmation(tx *gorm.DB, id, actor int64, scale string, generation int64, confirmed string) (map[string]any, error) {
	var row plateConfirmation
	if err := tx.Where("id=? AND actor_id=? AND scale_code=? AND session_generation=? AND confirmed_plate=?", id, actor, scale, generation, normalizePlate(confirmed)).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrInvalid
		}
		return nil, err
	}
	var evidence map[string]any
	if err := json.Unmarshal(row.Evidence, &evidence); err != nil {
		return nil, err
	}
	var refs struct {
		SourceEvents []struct {
			EventRecordID int64 `json:"eventRecordId"`
		} `json:"sourceEvents"`
	}
	if err := json.Unmarshal(row.Evidence, &refs); err != nil || len(refs.SourceEvents) == 0 {
		return nil, ErrInvalid
	}
	for _, ref := range refs.SourceEvents {
		var count int64
		if err := tx.Table("wb_camera_license_snapshot AS s").Joins("JOIN wb_camera AS c ON c.id=s.camera_id AND c.enabled=1 AND c.deleted=0").Joins("JOIN wb_camera_event_binding AS b ON b.camera_id=s.camera_id AND b.enabled=true AND b.serial_number=s.serial_number").Where("s.id=?", ref.EventRecordID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, ErrInvalid
		}
	}
	return evidence, nil
}

// validatePlateEvents constructs evidence entirely from persisted events and
// current bindings, never from client-supplied names or timestamps.
func validatePlateEvents(tx *gorm.DB, input *RecognitionInput, confirmed string, now time.Time) (map[string]any, error) {
	candidate := normalizePlate(input.Candidate)
	if !businessPlate.MatchString(candidate) || (candidate != normalizePlate(confirmed)) != input.Corrected || len(input.SourceEventRecordIDs) == 0 || len(input.SourceEventRecordIDs) > 50 {
		return nil, ErrInvalid
	}
	seen := map[int64]bool{}
	var sources []map[string]any
	var received []time.Time
	for _, id := range input.SourceEventRecordIDs {
		if id <= 0 || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
		var row struct {
			ID, CameraID                                   int64
			CameraName, Plate, NormalizedPlate, DeviceTime string
			ReceivedAt                                     time.Time
		}
		err := tx.Table("wb_camera_license_snapshot AS s").Select("s.id, s.camera_id, c.name AS camera_name, s.plate, s.normalized_plate, s.device_time, s.received_at").
			Joins("JOIN wb_camera AS c ON c.id=s.camera_id AND c.enabled=1 AND c.deleted=0").
			Joins("JOIN wb_camera_event_binding AS b ON b.camera_id=s.camera_id AND b.enabled=true AND b.serial_number=s.serial_number").
			Where("s.id=?", id).Take(&row).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, ErrInvalid
			}
			return nil, err
		}
		recognized, err := time.Parse(time.RFC3339, row.DeviceTime)
		if err != nil || row.NormalizedPlate != candidate || now.Sub(row.ReceivedAt) < 0 || now.Sub(row.ReceivedAt) >= 60*time.Second || row.ReceivedAt.Sub(recognized) < 0 || row.ReceivedAt.Sub(recognized) > 60*time.Second {
			return nil, ErrInvalid
		}
		sources = append(sources, map[string]any{"eventRecordId": row.ID, "cameraId": row.CameraID, "cameraName": row.CameraName, "rawPlate": row.Plate, "receivedAt": row.ReceivedAt, "recognizedAt": recognized})
		received = append(received, row.ReceivedAt)
	}
	sort.Slice(received, func(i, j int) bool { return received[i].Before(received[j]) })
	for i := 1; i < len(received); i++ {
		if received[i].Sub(received[i-1]) > 15*time.Second {
			return nil, ErrInvalid
		}
	}
	return map[string]any{"source": "server", "candidate": candidate, "confirmedPlate": confirmed, "corrected": input.Corrected, "at": now, "sourceEvents": sources}, nil
}
