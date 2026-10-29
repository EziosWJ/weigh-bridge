package camera

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
)

func (s *Service) CandidateWindow(ctx context.Context) ([]CandidateEvent, int64, time.Time, error) {
	store, ok := s.store.(interface {
		CandidateWindow(context.Context) ([]CandidateEvent, int64, time.Time, error)
	})
	if !ok {
		return nil, 0, time.Time{}, errors.New("candidate store unavailable")
	}
	return store.CandidateWindow(ctx)
}

// plateEventLock is shared by snapshot writers and watermark readers.
const plateEventLock int64 = 0x5742504c415445

func LockPlateEvents(ctx context.Context, tx *gorm.DB) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", plateEventLock).Error
}

// CandidateEvent is an already authorized, persisted recognition event.
type CandidateEvent struct {
	ID           int64     `json:"eventRecordId"`
	CameraID     int64     `json:"cameraId"`
	CameraName   string    `json:"cameraName"`
	Plate        string    `json:"-"`
	ReceivedAt   time.Time `json:"receivedAt"`
	RecognizedAt time.Time `json:"recognizedAt"`
}

type PlateCandidate struct {
	ID              int64            `json:"id"`
	Plate           string           `json:"plate"`
	SourceEvents    []CandidateEvent `json:"sourceEvents"`
	FirstReceivedAt time.Time        `json:"firstReceivedAt"`
	LastReceivedAt  time.Time        `json:"lastReceivedAt"`
	ExpiresAt       time.Time        `json:"expiresAt"`
}

// CandidateWindow reads the recent events and watermark under the same lock
// as inserts. The result is internal; callers must apply business authorization
// and plate validation before exposing it.
func (r *Repository) CandidateWindow(ctx context.Context) ([]CandidateEvent, int64, time.Time, error) {
	var events []CandidateEvent
	var watermark int64
	var now time.Time
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockPlateEvents(ctx, tx); err != nil {
			return err
		}
		now = time.Now().UTC()
		if err := tx.Raw("SELECT COALESCE(MAX(id), 0) FROM wb_camera_license_snapshot").Scan(&watermark).Error; err != nil {
			return err
		}
		var rows []struct {
			ID              int64
			CameraID        int64
			CameraName      string
			NormalizedPlate string
			ReceivedAt      time.Time
			DeviceTime      string
		}
		if err := tx.Table("wb_camera_license_snapshot AS s").
			Select("s.id, s.camera_id, c.name AS camera_name, s.normalized_plate, s.received_at, s.device_time").
			Joins("JOIN wb_camera AS c ON c.id = s.camera_id AND c.enabled = 1 AND c.deleted = 0").
			Joins("JOIN wb_camera_event_binding AS b ON b.camera_id = s.camera_id AND b.enabled = true AND b.serial_number = s.serial_number").
			Where("s.received_at > ? AND s.id <= ?", now.Add(-60*time.Second), watermark).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			recognized, err := time.Parse(time.RFC3339, row.DeviceTime)
			if err != nil {
				continue
			}
			events = append(events, CandidateEvent{ID: row.ID, CameraID: row.CameraID, CameraName: row.CameraName, Plate: row.NormalizedPlate, ReceivedAt: row.ReceivedAt, RecognizedAt: recognized})
		}
		return nil
	})
	return events, watermark, now, err
}

// AggregatePlateCandidates operates on server-filtered events. The caller is
// responsible for authorization, camera binding and plate validation.
func AggregatePlateCandidates(events []CandidateEvent, now time.Time) []PlateCandidate {
	rows := append([]CandidateEvent(nil), events...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ReceivedAt.Equal(rows[j].ReceivedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].ReceivedAt.Before(rows[j].ReceivedAt)
	})
	groups := []PlateCandidate{}
	seen := map[int64]bool{}
	for _, e := range rows {
		age := now.Sub(e.ReceivedAt)
		delay := e.ReceivedAt.Sub(e.RecognizedAt)
		if seen[e.ID] || e.ID <= 0 || e.Plate == "" || age < 0 || age >= 60*time.Second || delay < 0 || delay > 60*time.Second {
			continue
		}
		seen[e.ID] = true
		index := -1
		for i := len(groups) - 1; i >= 0; i-- {
			if groups[i].Plate == e.Plate && e.ReceivedAt.Sub(groups[i].LastReceivedAt) <= 15*time.Second {
				index = i
				break
			}
		}
		if index < 0 {
			groups = append(groups, PlateCandidate{ID: e.ID, Plate: e.Plate, FirstReceivedAt: e.ReceivedAt})
			index = len(groups) - 1
		}
		g := &groups[index]
		g.SourceEvents = append(g.SourceEvents, e)
		g.LastReceivedAt = e.ReceivedAt
		g.ExpiresAt = e.ReceivedAt.Add(60 * time.Second)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].LastReceivedAt.Equal(groups[j].LastReceivedAt) {
			return groups[i].ID > groups[j].ID
		}
		return groups[i].LastReceivedAt.After(groups[j].LastReceivedAt)
	})
	// One candidate per plate: a later batch of the same plate replaces the
	// earlier groups instead of occupying another display slot. The retained
	// group keeps a valid <=15s source chain for server-side confirmation.
	unique := make([]PlateCandidate, 0, len(groups))
	plates := map[string]bool{}
	for _, group := range groups {
		if plates[group.Plate] {
			continue
		}
		plates[group.Plate] = true
		unique = append(unique, group)
	}
	return unique
}
