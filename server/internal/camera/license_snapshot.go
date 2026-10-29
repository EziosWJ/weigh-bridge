package camera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Keep the manufacturer event ID as raw JSON; it can exceed JavaScript or
// float64 precision. Images in the event are deliberately not decoded or saved.
type snapshotRequest struct {
	Info struct {
		EventID json.RawMessage `json:"eventId"`
		Time    string          `json:"time"`
		License struct {
			Number string `json:"Number"`
		} `json:"carLicenseAttriInfo"`
	} `json:"info"`
}
type LicenseSnapshot struct {
	ID              int64           `json:"id" gorm:"primaryKey"`
	CameraID        int64           `json:"cameraId"`
	SerialNumber    string          `json:"serialNumber"`
	EventID         string          `json:"eventId"`
	DeviceTime      string          `json:"deviceTime"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	Plate           string          `json:"plate"`
	NormalizedPlate string          `json:"normalizedPlate"`
	Attributes      json.RawMessage `json:"attributes" gorm:"type:jsonb"`
	SHA256          string          `json:"-"`
	CaptureBytes    int             `json:"-"`
	BackgroundBytes int             `json:"-"`
}

func (LicenseSnapshot) TableName() string { return "wb_camera_license_snapshot" }

const snapshotMaxAge = 5 * time.Minute

var errExpiredSnapshot = errors.New("expired snapshot")

func snapshotExpired(deviceTime, receivedAt time.Time, maxAge time.Duration) bool {
	return receivedAt.Sub(deviceTime) > maxAge
}

type snapshotStore interface {
	ReceiveSnapshot(context.Context, string, string, func(int64, time.Duration) (LicenseSnapshot, error)) (bool, error)
	Snapshots(context.Context, int64) ([]LicenseSnapshot, error)
}

func (s *Service) receiveSnapshot(ctx context.Context, serial, hash string, body []byte, now time.Time) (bool, error) {
	store, ok := s.store.(snapshotStore)
	if !ok {
		return false, errors.New("snapshot store unavailable")
	}
	return store.ReceiveSnapshot(ctx, serial, hash, func(cameraID int64, maxAge time.Duration) (LicenseSnapshot, error) {
		var request snapshotRequest
		if err := json.Unmarshal(body, &request); err != nil {
			return LicenseSnapshot{}, fmt.Errorf("%w: snapshot fields", ErrInvalid)
		}
		deviceTime, err := time.Parse(time.RFC3339, request.Info.Time)
		if err != nil {
			return LicenseSnapshot{}, fmt.Errorf("%w: device time", ErrInvalid)
		}
		if snapshotExpired(deviceTime, now, maxAge) {
			return LicenseSnapshot{}, errExpiredSnapshot
		}
		plate := request.Info.License.Number
		if len(plate) > 200 || strings.ContainsAny(plate, "\x00\r\n") {
			return LicenseSnapshot{}, fmt.Errorf("%w: plate", ErrInvalid)
		}
		var values struct {
			Info struct {
				License json.RawMessage `json:"carLicenseAttriInfo"`
				Car     json.RawMessage `json:"carAttriInfo"`
			} `json:"info"`
		}
		if json.Unmarshal(body, &values) != nil {
			return LicenseSnapshot{}, ErrInvalid
		}
		attributes := map[string]map[string]json.RawMessage{"carLicenseAttriInfo": {}, "carAttriInfo": {}}
		for group, keys := range map[string][]string{"carLicenseAttriInfo": {"CarLicenseAttriColor", "CarLicenseRowNum", "CarLicenseRect"}, "carAttriInfo": {"DriveEntryDirection", "Match", "Score", "CarRect"}} {
			raw := values.Info.License
			if group == "carAttriInfo" {
				raw = values.Info.Car
			}
			var source map[string]json.RawMessage
			_ = json.Unmarshal(raw, &source)
			for _, key := range keys {
				if value, ok := source[key]; ok && len(value) <= 1024 {
					attributes[group][key] = value
				}
			}
		}
		attrs, err := json.Marshal(attributes)
		if err != nil {
			return LicenseSnapshot{}, ErrInvalid
		}
		normalized := strings.ToUpper(strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == '·' || r == '•' {
				return -1
			}
			return r
		}, plate))
		return LicenseSnapshot{CameraID: cameraID, SerialNumber: serial, EventID: string(request.Info.EventID), DeviceTime: request.Info.Time, ReceivedAt: now, Plate: plate, NormalizedPlate: normalized, Attributes: attrs, SHA256: hash}, nil
	})
}
func (r *Repository) ReceiveSnapshot(ctx context.Context, serial, hash string, prepare func(int64, time.Duration) (LicenseSnapshot, error)) (bool, error) {
	// SQL errors must not print the plate or attributes to the server log.
	accepted := true
	err := r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		var binding EventBinding
		if err := tx.Where("serial_number=? AND enabled=true", serial).First(&binding).Error; err != nil {
			return normalizeDBError(err)
		}
		var cam cameraRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0 AND enabled=1", binding.CameraID).First(&cam).Error; err != nil {
			return normalizeDBError(err)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("camera_id=? AND serial_number=? AND enabled=true", binding.CameraID, serial).First(&binding).Error; err != nil {
			return normalizeDBError(err)
		}
		var existing LicenseSnapshot
		err := tx.Where("camera_id=? AND sha256=?", binding.CameraID, hash).First(&existing).Error
		if err == nil {
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		maxAge := snapshotMaxAge
		var config sysconfig.Config
		configErr := tx.Where("config_key=? AND deleted=0 AND status=1", sysconfig.CameraEventMaxAgeKey).First(&config).Error
		if configErr != nil && !errors.Is(configErr, gorm.ErrRecordNotFound) {
			return configErr
		}
		if configErr == nil {
			minutes, parseErr := strconv.Atoi(strings.TrimSpace(config.ConfigValue))
			if parseErr == nil && minutes >= 1 && minutes <= 10080 {
				maxAge = time.Duration(minutes) * time.Minute
			}
		}
		row, err := prepare(binding.CameraID, maxAge)
		if errors.Is(err, errExpiredSnapshot) {
			accepted = false
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	return accepted, err
}
func (r *Repository) Snapshots(ctx context.Context, cameraID int64) ([]LicenseSnapshot, error) {
	var rows []LicenseSnapshot
	err := r.db.WithContext(ctx).Where("camera_id=?", cameraID).Order("id DESC").Limit(50).Find(&rows).Error
	return rows, err
}
