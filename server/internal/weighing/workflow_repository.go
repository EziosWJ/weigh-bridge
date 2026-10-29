package weighing

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var businessPlate = regexp.MustCompile(`^[晋京津冀蒙辽吉黑沪苏浙皖闽赣鲁豫鄂湘粤桂琼渝川贵云藏陕甘青宁新][A-HJ-NP-Z]([A-HJ-NP-Z0-9]{5}|[DF][A-HJ-NP-Z0-9][0-9]{4}|[0-9]{5}[DF])$`)

func normalizePlate(v string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '·' || r == '•' {
			return -1
		}
		return r
	}, v))
}
func hash(value any) string        { data, _ := json.Marshal(value); return hashBytes(data) }
func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func actorName(tx *gorm.DB, id int64) (string, error) {
	var row struct{ Nickname, Username string }
	err := tx.Table("sys_user").Select("nickname,username").Where("id=? AND deleted=0", id).Take(&row).Error
	if row.Nickname != "" {
		return row.Nickname, err
	}
	return row.Username, err
}
func ptr(v int64) *int64 { return &v }
func (r *Repository) Session(ctx context.Context, scaleCode string) (ScaleSession, error) {
	var v ScaleSession
	identity, err := localsite.Read(ctx, r.db)
	if err != nil {
		return v, err
	}
	if scaleCode == "" || scaleCode != identity.ScaleCode {
		return v, ErrNotFound
	}
	result := r.db.WithContext(ctx).Where("local_device=TRUE").Limit(1).Find(&v)
	if result.Error != nil {
		return v, result.Error
	}
	if result.RowsAffected == 0 {
		if err := r.db.WithContext(ctx).Where("code=?", scaleCode).Take(&Scale{}).Error; err != nil {
			return v, err
		}
	}
	v.ScaleCode = scaleCode
	return v, nil
}
func lockSession(tx *gorm.DB, scaleCode string) (ScaleSession, error) {
	var v ScaleSession
	identity, err := localsite.Read(tx.Statement.Context, tx)
	if err != nil {
		return v, err
	}
	if scaleCode == "" || scaleCode != identity.ScaleCode {
		return v, ErrSession
	}
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("local_device=TRUE").Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, ErrSession
	}
	v.ScaleCode = scaleCode
	return v, err
}
func validSession(v ScaleSession, actor int64, in SessionInput) bool {
	return v.HolderUserID != nil && *v.HolderUserID == actor && v.ClientID == in.ClientID && in.SessionToken != "" && v.TokenHash == hashBytes([]byte(in.SessionToken)) && v.ExpiresAt != nil && v.ExpiresAt.After(time.Now().UTC())
}
func (r *Repository) ChangeSession(ctx context.Context, scaleCode string, action string, in SessionInput, e audit.Event) (SessionResult, error) {
	var result SessionResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWriteSite(tx, ctx); err != nil {
			return err
		}
		var scale Scale
		if err := tx.Where("code=?", scaleCode).Take(&scale).Error; err != nil {
			return err
		}
		if scale.Enabled != 1 {
			return ErrSession
		}
		initial := ScaleSession{LocalDevice: true, ScaleCode: scaleCode, Version: 1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		current, err := lockSession(tx, scaleCode)
		if err != nil {
			return err
		}
		before := current
		now := time.Now().UTC()
		name, err := actorName(tx, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		token := ""
		switch action {
		case "acquire", "takeover":
			held := current.HolderUserID != nil && current.ExpiresAt != nil && current.ExpiresAt.After(now)
			same := current.HolderUserID != nil && *current.HolderUserID == e.Metadata.ActorID && current.ClientID == in.ClientID
			if held && !same && action != "takeover" {
				return ErrOccupied
			}
			if action == "takeover" && strings.TrimSpace(in.Reason) == "" {
				return ErrInvalid
			}
			data := make([]byte, 32)
			if _, err := rand.Read(data); err != nil {
				return err
			}
			token = hex.EncodeToString(data)
			expires := now.Add(r.sessionTTL)
			current.HolderUserID = ptr(e.Metadata.ActorID)
			current.HolderName = name
			current.ClientID = in.ClientID
			current.TokenHash = hashBytes([]byte(token))
			current.ExpiresAt = &expires
			current.Generation++
		case "renew":
			if !validSession(current, e.Metadata.ActorID, in) {
				return ErrSession
			}
			expires := now.Add(r.sessionTTL)
			current.ExpiresAt = &expires
		case "release":
			if !validSession(current, e.Metadata.ActorID, in) {
				return ErrSession
			}
			current.HolderUserID = nil
			current.HolderName = ""
			current.ClientID = ""
			current.TokenHash = ""
			current.ExpiresAt = nil
			current.Generation++
		default:
			return ErrInvalid
		}
		current.Version++
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		e.Action = "session." + action
		e.Resource = "weighing.session"
		event := BusinessEvent{ResourceType: e.Resource, Action: e.Action, Reason: in.Reason, Before: snapshot(before), After: snapshot(current), ActorID: e.Metadata.ActorID, ActorName: name, OccurredAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := audit.RecordOn(ctx, tx, e); err != nil {
			return err
		}
		result = SessionResult{ScaleSession: current, SessionToken: token}
		return nil
	})
	return result, err
}
func resolveBusiness(tx *gorm.DB, in BusinessInput, old *BusinessInfo) (BusinessInfo, error) {
	in.Plate = normalizePlate(in.Plate)
	if !businessPlate.MatchString(in.Plate) || (in.Direction != "inbound" && in.Direction != "outbound") || in.MaterialID < 1 || in.PartyID < 1 || !maxLength(in.ContactName, 100) || !maxLength(in.ContactPhone, 50) || !maxLength(in.Driver, 100) || !maxLength(in.DriverPhone, 50) || !maxLength(in.Remark, 2000) {
		return BusinessInfo{}, ErrInvalid
	}
	if old != nil && old.MaterialID == in.MaterialID && old.PartyID == in.PartyID && ((old.CarrierID == nil && in.CarrierID == nil) || (old.CarrierID != nil && in.CarrierID != nil && *old.CarrierID == *in.CarrierID)) {
		info := *old
		info.BusinessInput = in
		return info, nil
	}
	var cloudState masterSyncState
	if err := tx.First(&cloudState, 1).Error; err != nil {
		return BusinessInfo{}, err
	}
	if cloudState.Bound {
		checks := []struct {
			table     string
			id        int64
			unchanged bool
		}{
			{"wb_master_material_map", in.MaterialID, old != nil && old.MaterialID == in.MaterialID},
			{"wb_master_party_map", in.PartyID, old != nil && old.PartyID == in.PartyID},
		}
		if in.CarrierID != nil {
			checks = append(checks, struct {
				table     string
				id        int64
				unchanged bool
			}{"wb_master_party_map", *in.CarrierID, old != nil && old.CarrierID != nil && *old.CarrierID == *in.CarrierID})
		}
		for _, check := range checks {
			if check.unchanged {
				continue
			}
			var n int64
			if err := tx.Table(check.table).Where("local_id = ?", check.id).Count(&n).Error; err != nil {
				return BusinessInfo{}, err
			}
			if n != 1 {
				return BusinessInfo{}, RejectionError("云端模式下新业务只能选择总部资料")
			}
		}
	}
	material, err := lockRow[Material](tx, in.MaterialID)
	if err != nil {
		return BusinessInfo{}, err
	}
	if material.Enabled != 1 && (old == nil || old.MaterialID != material.ID) {
		return BusinessInfo{}, ErrInvalid
	}
	// Stable id order avoids opposite supplier/carrier lock orders across tasks.
	ids := []int64{in.PartyID}
	if in.CarrierID != nil {
		if *in.CarrierID < 1 {
			return BusinessInfo{}, ErrInvalid
		}
		if *in.CarrierID != in.PartyID {
			ids = append(ids, *in.CarrierID)
		}
	}
	if len(ids) == 2 && ids[0] > ids[1] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	parties := map[int64]Party{}
	for _, id := range ids {
		p, err := lockRow[Party](tx, id)
		if err != nil {
			return BusinessInfo{}, err
		}
		parties[id] = p
	}
	party := parties[in.PartyID]
	if party.Enabled != 1 && (old == nil || old.PartyID != party.ID) {
		return BusinessInfo{}, ErrInvalid
	}
	if (in.Direction == "inbound" && party.IsSupplier != 1) || (in.Direction == "outbound" && party.IsCustomer != 1) {
		return BusinessInfo{}, ErrInvalid
	}
	info := BusinessInfo{BusinessInput: in, MaterialVersion: material.Version, PartyVersion: party.Version, MaterialCode: material.Code, MaterialName: material.Name, MaterialSpec: material.Spec, MaterialCategoryID: material.CategoryID, PartyCode: party.Code, PartyName: party.Name, PartyNature: party.Nature, PartyPhone: party.Phone, PartyAddress: party.Address}
	if material.CategoryID != nil {
		category, err := findOne[Category](tx, *material.CategoryID)
		if err != nil {
			return info, err
		}
		info.MaterialCategoryName = category.Name
	}
	if in.CarrierID != nil {
		carrier := parties[*in.CarrierID]
		if carrier.IsCarrier != 1 || (carrier.Enabled != 1 && (old == nil || old.CarrierID == nil || *old.CarrierID != carrier.ID)) {
			return info, ErrInvalid
		}
		info.CarrierVersion = carrier.Version
		info.CarrierCode = carrier.Code
		info.CarrierName = carrier.Name
	}
	return info, nil
}
func weighingKind(direction, stage string) string {
	if (direction == "inbound" && stage == "first") || (direction == "outbound" && stage == "second") {
		return "gross"
	}
	return "tare"
}
func (r *Repository) Capture(ctx context.Context, in CaptureInput, e audit.Event) (Sample, error) {
	var result Sample
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWriteSite(tx, ctx); err != nil {
			return err
		}
		session, err := lockSession(tx, in.ScaleCode)
		if err != nil {
			return err
		}
		if !validSession(session, e.Metadata.ActorID, in.SessionInput) {
			return ErrSession
		}
		device, ok := ctx.Value(deviceKey{}).(deviceContext)
		if !ok || device.Capture == nil {
			return ErrConditions
		}
		stage := "first"
		var business BusinessInfo
		var version *int64
		if in.TaskID != nil {
			task, err := lockRow[Task](tx, *in.TaskID)
			if err != nil {
				return err
			}
			identity, err := localsite.Read(ctx, tx)
			if err != nil {
				return err
			}
			if identity.ScaleCode == "" || identity.ScaleCode != in.ScaleCode {
				return ErrForbidden
			}
			business = task.Business
			version = ptr(task.Version)
			stage = "second"
			if in.TargetRecordID != nil {
				target, err := findOne[WeighRecord](tx, *in.TargetRecordID)
				if err != nil {
					return err
				}
				if target.TaskID != task.ID {
					return ErrInvalid
				}
				stage = target.Stage
				if task.Status == "terminated" {
					return ErrConflict
				}
				if task.Status == "completed" {
					if err := activePlateConflict(tx, task.Plate, task.ID, task.SiteCode); err != nil {
						return err
					}
					var active int64
					if err := tx.Table("wb_ticket").Where("task_id=? AND status='valid'", task.ID).Count(&active).Error; err != nil {
						return err
					}
					if active > 0 {
						return ErrConflict
					}
				}
			} else if task.Status != "awaiting_second" {
				return ErrConflict
			}
		} else {
			if in.TargetRecordID != nil {
				return ErrInvalid
			}
			in.Business.Plate = normalizePlate(in.Business.Plate)
			if !businessPlate.MatchString(in.Business.Plate) || (in.Business.Direction != "inbound" && in.Business.Direction != "outbound") {
				return ErrInvalid
			}
			if in.Business.MaterialID > 0 && in.Business.PartyID > 0 {
				business, err = resolveBusiness(tx, in.Business, nil)
				if err != nil {
					return err
				}
			} else {
				business = BusinessInfo{BusinessInput: in.Business}
			}
		}
		now := time.Now().UTC()
		captured := device.Capture
		policy, err := capturePolicyMatches(ctx, tx)
		if err != nil {
			return err
		}
		captureSource := device.Source
		if captureSource == "" {
			captureSource = "agent"
		}
		source, candidate, observedAt := "manual", "", ""
		if captureSource == "server" && captured.Plate.Candidate != "" {
			// Optional recognition reuses a Server camera image; when recognition
			// was unavailable the candidate may instead be the Agent's observation.
			source = captured.PlateSource
			if source != "agent" {
				source = "server"
			}
			candidate = captured.Plate.Candidate
			if !captured.Plate.At.IsZero() {
				observedAt = captured.Plate.At.Format(time.RFC3339Nano)
			}
		} else if captureSource != "server" && in.PlateEvidence != nil && in.PlateEvidence.Source == "agent" {
			source = "agent"
			candidate = captured.Plate.Candidate
			observedAt = captured.Plate.At.Format(time.RFC3339Nano)
		}
		plateEvidence := snapshot(map[string]any{"source": source, "candidate": candidate, "confirmedPlate": business.Plate, "at": now, "observedAt": observedAt})
		if in.PlateEvidence != nil && in.PlateEvidence.Source == "server" {
			verified, err := readPlateConfirmation(tx, in.PlateEvidence.ConfirmationID, e.Metadata.ActorID, in.ScaleCode, session.Generation, business.Plate)
			if err != nil {
				return err
			}
			plateEvidence = snapshot(verified)
		}
		capturedAt := captured.CapturedAt
		if capturedAt.IsZero() {
			capturedAt = now
		}
		evidenceFields := map[string]any{"captureRequired": policy.Required, "capturePolicyRevision": policy.Revision, "imageStatus": captured.ImageStatus, "imageError": captured.ImageError, "source": captureSource, "images": captured.Images, "at": capturedAt, "receivedAt": now, "weight": captured.Weight, "plate": business.Plate, "plateEvidence": json.RawMessage(plateEvidence)}
		if captureSource == "server" {
			verifiedAt := capturedAt
			evidenceFields["measurementGeneration"] = captured.MeasurementGeneration
			evidenceFields["measurementVerifiedAt"] = verifiedAt
			evidenceFields["measurementVerified"] = captured.MeasurementVerified
			evidenceFields["cameraConfigVersion"] = captured.CameraConfigVersion
			evidenceFields["failures"] = device.Failures
		} else {
			var primaryImage p.Image
			for _, image := range captured.Images {
				if primaryImage.ID == "" || image.View == "front" {
					primaryImage = image
				}
			}
			cameraLabel := "车头视角"
			if primaryImage.View == "rear" {
				cameraLabel = "车尾视角"
			}
			evidenceFields["agentId"] = device.State.AgentID
			evidenceFields["imageSrc"] = primaryImage.URL
			evidenceFields["sha256"] = primaryImage.SHA256
			evidenceFields["preset"] = primaryImage.View
			evidenceFields["cameraLabel"] = cameraLabel
		}
		evidence := snapshot(evidenceFields)
		verifiedAt := (*time.Time)(nil)
		if captured.MeasurementVerified && captured.MeasurementGeneration != "" {
			value := capturedAt
			verifiedAt = &value
		}
		result = Sample{ScaleCode: in.ScaleCode, OwnerUserID: e.Metadata.ActorID, ClientID: in.ClientID, SessionGeneration: session.Generation, TaskID: in.TaskID, TargetRecordID: in.TargetRecordID, TaskVersion: version, BusinessHash: hash(business), Business: business, Stage: stage, Kind: weighingKind(business.Direction, stage), Plate: business.Plate, Source: captureSource, Weight: captured.Weight, CapturedAt: capturedAt, Stable: 1, DeviceOK: 1, Evidence: evidence, PlateEvidence: plateEvidence, DeviceRevision: device.State.Revision, MeasurementGeneration: captured.MeasurementGeneration, MeasurementVerifiedAt: verifiedAt, CameraConfigVersion: captured.CameraConfigVersion}
		// Recapture invalidates earlier unconfirmed samples in this session.
		if err := tx.Model(&Sample{}).Where("session_generation=? AND invalidated_at IS NULL AND id NOT IN (SELECT sample_id FROM wb_weigh_record)", session.Generation).Update("invalidated_at", now).Error; err != nil {
			return err
		}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return nil
	})
	return result, err
}

// Sample returns one unconfirmed or historical sample for confirmation checks.
func (r *Repository) Sample(ctx context.Context, id int64) (Sample, error) {
	return findOne[Sample](r.db.WithContext(ctx), id)
}

func (r *Repository) InvalidateSample(ctx context.Context, id int64, in SessionInput, actor int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sample, err := findOne[Sample](tx, id)
		if err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		session, err := lockSession(tx, identity.ScaleCode)
		if err != nil {
			return err
		}
		if !validSession(session, actor, in) || sample.OwnerUserID != actor || sample.ClientID != in.ClientID {
			return ErrSession
		}
		return tx.Model(&sample).Update("invalidated_at", time.Now().UTC()).Error
	})
}
func validateSample(ctx context.Context, tx *gorm.DB, in ConfirmationInput, actor int64, session ScaleSession) (Sample, error) {
	sample, err := findOne[Sample](tx, in.SampleID)
	if err != nil {
		return sample, err
	}
	if !in.Checks.Valid() {
		return sample, ErrInvalid
	}
	if !validSession(session, actor, in.SessionInput) {
		return sample, ErrSession
	}
	if sample.OwnerUserID != actor || sample.ClientID != in.ClientID || sample.SessionGeneration != session.Generation || sample.InvalidatedAt != nil || sample.Stable != 1 || sample.DeviceOK != 1 || sample.Weight <= 0 {
		return sample, ErrConditions
	}
	// Validate the persisted capture evidence, never the current instrument.
	if sample.MeasurementGeneration != "" && sample.MeasurementVerifiedAt == nil {
		return sample, ErrConditions
	}
	var used int64
	if err := tx.Model(&WeighRecord{}).Where("sample_id=?", sample.ID).Count(&used).Error; err != nil {
		return sample, err
	}
	if used > 0 {
		return sample, ErrConditions
	}
	return sample, nil
}
func newRecord(sample Sample, taskID int64, actor int64, name string) WeighRecord {
	return WeighRecord{TaskID: taskID, SampleID: sample.ID, Stage: sample.Stage, Kind: sample.Kind, Weight: sample.Weight, CapturedAt: sample.CapturedAt, ConfirmedAt: time.Now().UTC(), ScaleCode: sample.ScaleCode, ActorID: actor, ActorName: name, Evidence: sample.Evidence, PlateEvidence: sample.PlateEvidence}
}

type commandOutcome struct {
	result CommandResult
	event  BusinessEvent
}

func findOperation(tx *gorm.DB, actor int64, key string) (Operation, bool, error) {
	var op Operation
	result := tx.Where("actor_id=? AND request_id=?", actor, key).Limit(1).Find(&op)
	return op, result.RowsAffected > 0, result.Error
}
func replayOperation(op Operation, requestHash string) (CommandResult, error) {
	var result CommandResult
	if op.RequestHash != requestHash {
		return result, ErrRequestKey
	}
	if err := json.Unmarshal(op.Result, &result); err != nil {
		return result, err
	}
	if op.Status == "rejected" {
		if op.ErrorSnapshot == ErrForbidden.Error() {
			return result, ErrForbidden
		}
		return result, RejectionError(op.ErrorSnapshot)
	}
	return result, nil
}
func domainError(err error) bool {
	return errors.Is(err, ErrForbidden) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) || errors.Is(err, ErrReferenced) || errors.Is(err, ErrConditions) || errors.Is(err, ErrCapturePolicyChanged) || errors.Is(err, ErrCaptureCameraRequired) || errors.Is(err, ErrMeasurementCapability) || errors.Is(err, ErrCameraConfigChanged) || errors.Is(err, ErrSession) || errors.Is(err, ErrWeight) || errors.Is(err, ErrNotFound)
}
func (r *Repository) command(ctx context.Context, key, action string, payload any, e audit.Event, lock func(*gorm.DB) error, perform func(*gorm.DB, string) (commandOutcome, error)) (CommandResult, error) {
	var result CommandResult
	var businessErr error
	requestHash := hash(struct {
		Action  string
		Payload any
	}{action, payload})
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		op, found, err := findOperation(tx, e.Metadata.ActorID, key)
		if err != nil {
			return err
		}
		if found {
			result, businessErr = replayOperation(op, requestHash)
			return nil
		}
		if err := lockWriteSite(tx, ctx); err != nil {
			return err
		}
		lockErr := lock(tx)
		// Recheck before reporting an invalid sample, expired session, or old version.
		op, found, err = findOperation(tx, e.Metadata.ActorID, key)
		if err != nil {
			return err
		}
		if found {
			result, businessErr = replayOperation(op, requestHash)
			return nil
		}
		name, err := actorName(tx, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		outcome := commandOutcome{}
		if lockErr == nil {
			outcome, err = perform(tx, name)
		} else {
			err = lockErr
		}
		if err != nil && !domainError(err) {
			return err
		}
		businessErr = err
		result = outcome.result
		op = Operation{ActorID: e.Metadata.ActorID, RequestID: key, Command: action, RequestHash: requestHash, Status: "committed", Result: snapshot(result), CommittedAt: time.Now().UTC()}
		if result.TaskID > 0 {
			op.TaskID = ptr(result.TaskID)
		}
		if result.TicketID > 0 {
			op.TicketID = ptr(result.TicketID)
		}
		if businessErr != nil {
			op.Status = "rejected"
			op.ErrorSnapshot = businessErr.Error()
		}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		event := outcome.event
		event.OperationID = ptr(op.ID)
		event.Action = action
		event.ResourceType = "weighing.task"
		event.ActorID = e.Metadata.ActorID
		event.ActorName = name
		event.OccurredAt = op.CommittedAt
		if result.TaskID > 0 {
			event.TaskID = ptr(result.TaskID)
			event.ResourceID = ptr(result.TaskID)
		}
		if result.TicketID > 0 {
			event.TicketID = ptr(result.TicketID)
		}
		if businessErr != nil {
			event.Action += ".rejected"
			event.Reason = businessErr.Error()
			event.After = redactedPayload(payload)
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if businessErr == nil {
			e.Action = action
			e.Resource = event.ResourceType
			e.ResourceID = result.TaskID
			return audit.RecordOn(ctx, tx, e)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			op, found, lookupErr := findOperation(r.db.WithContext(ctx), e.Metadata.ActorID, key)
			if lookupErr == nil && found {
				return replayOperation(op, requestHash)
			}
			return result, ErrConflict
		}
		return result, err
	}
	return result, businessErr
}
func (r *Repository) ConfirmFirst(ctx context.Context, in ConfirmationInput, e audit.Event) (CommandResult, error) {
	var session ScaleSession
	return r.command(ctx, in.RequestID, "first.confirm", in, e, func(tx *gorm.DB) error { var err error; session, err = lockSession(tx, in.ScaleCode); return err }, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{}
		sample, err := validateSample(ctx, tx, in, e.Metadata.ActorID, session)
		if err != nil {
			return out, err
		}
		if sample.TaskID != nil || sample.Stage != "first" || sample.TargetRecordID != nil {
			return out, ErrInvalid
		}
		business, err := resolveBusiness(tx, in.Business, nil)
		if err != nil {
			return out, err
		}
		// A first capture binds the vehicle and loading direction. Ordinary
		// business details may be completed or corrected before final review.
		if business.Plate != sample.Plate || business.Direction != sample.Business.Direction {
			return out, ErrConditions
		}
		if hash(business) != sample.BusinessHash {
			versions := in.ReviewedVersions
			if versions == nil || versions.Material != business.MaterialVersion || versions.Party != business.PartyVersion || versions.Carrier != business.CarrierVersion {
				return out, ErrConflict
			}
		}
		var scale Scale
		if err := tx.Where("code=?", in.ScaleCode).Take(&scale).Error; err != nil {
			return out, err
		}
		var active int64
		if err := tx.Model(&Task{}).Where("plate=? AND status='awaiting_second'", business.Plate).Count(&active).Error; err != nil {
			return out, err
		}
		if active > 0 {
			return out, ErrConflict
		}
		number, err := nextNumber(tx, "task", "RW", r.dataDir, time.Now().UTC())
		if err != nil {
			return out, err
		}
		now := time.Now().UTC()
		task := Task{SiteCode: scale.SiteCode, TaskNo: number, Mode: "normal", Plate: business.Plate, Direction: business.Direction, Status: "awaiting_second", ScaleCode: in.ScaleCode, MaterialID: business.MaterialID, PartyID: business.PartyID, CarrierID: business.CarrierID, Business: business, Version: 1, CreateTime: now, UpdateTime: now, ActorID: e.Metadata.ActorID, ActorName: name}
		if err := tx.Create(&task).Error; err != nil {
			return out, err
		}
		record := newRecord(sample, task.ID, e.Metadata.ActorID, name)
		if err := tx.Create(&record).Error; err != nil {
			return out, err
		}
		if err := tx.Model(&task).Update("current_first_record_id", record.ID).Error; err != nil {
			return out, err
		}
		task.CurrentFirstRecordID = ptr(record.ID)
		out.result = CommandResult{TaskID: task.ID, RecordID: record.ID, TaskVersion: 1, Status: task.Status}
		out.event.After = snapshot(task.Business)
		out.event.SampleID = ptr(sample.ID)
		return out, nil
	})
}
func (r *Repository) Operation(ctx context.Context, actor int64, key string) (Operation, error) {
	requested := key
	op, found, err := findOperation(r.db.WithContext(ctx), actor, key)
	if err == nil && !found {
		op = Operation{RequestID: requested, Status: "not_found"}
	}
	return op, err
}
func (r *Repository) FindTask(ctx context.Context, id int64) (TaskDetail, error) {
	detail := TaskDetail{Records: []WeighRecord{}, Events: []BusinessEvent{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := findOne[Task](tx, id)
		if err != nil {
			return err
		}
		detail.Task = task
		if err := tx.Where("task_id=?", id).Order("id").Find(&detail.Records).Error; err != nil {
			return err
		}
		return tx.Where("task_id=?", id).Order("id").Find(&detail.Events).Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return detail, err
}
func (r *Repository) ActiveTask(ctx context.Context, plate string) (*TaskDetail, error) {
	var tasks []Task
	db := siteFilter(r.db.WithContext(ctx).Model(&Task{}), PageQuery{}, "site_code")
	result := db.Where("plate=? AND status='awaiting_second'", normalizePlate(plate)).Limit(2).Find(&tasks)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	if len(tasks) > 1 {
		return nil, ErrInvalid
	}
	detail, err := r.FindTask(ctx, tasks[0].ID)
	if err == nil && detail.Status != "awaiting_second" {
		return nil, nil
	}
	return &detail, err
}

func redactedPayload(payload any) JSON {
	decoder := json.NewDecoder(bytes.NewReader(snapshot(payload)))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil
	}
	var redact func(any)
	redact = func(current any) {
		switch node := current.(type) {
		case map[string]any:
			delete(node, "sessionToken")
			for _, child := range node {
				redact(child)
			}
		case []any:
			for _, child := range node {
				redact(child)
			}
		}
	}
	redact(value)
	return snapshot(value)
}
