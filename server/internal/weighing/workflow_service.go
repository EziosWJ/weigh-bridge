package weighing

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
)

// Measurement commands are short correlated round trips. The unified camera
// capture deadline is owned by camera.CapturePlan.TimeoutMS and is deliberately
// not held under the device state lock.
const (
	measurementCommandTimeout = 10 * time.Second
	measurementCancelTimeout  = 2 * time.Second
)

func (s *Service) Session(ctx context.Context, scaleCode string) (ScaleSession, error) {
	return s.store.Session(ctx, scaleCode)
}
func (s *Service) ChangeSession(ctx context.Context, scaleCode string, action string, in SessionInput, e audit.Event) (SessionResult, error) {
	if strings.TrimSpace(scaleCode) == "" || strings.TrimSpace(in.ClientID) == "" || !maxLength(in.ClientID, 100) || !maxLength(in.Reason, 2000) {
		return SessionResult{}, ErrInvalid
	}
	return s.store.ChangeSession(ctx, scaleCode, action, in, e)
}

// Capture orchestrates one formal acquisition:
//
//	Agent begin (freeze the instrument basis) -> parallel Server camera capture
//	under one deadline -> Agent end verification -> persist the sample.
//
// The camera set and config version are frozen before the window opens. The
// Agent generation records the verified capture window. A successful sample is
// frozen; operator confirmation does not depend on subsequent device state.
func (s *Service) Capture(ctx context.Context, in CaptureInput, e audit.Event) (Sample, error) {
	release, gateErr := s.maintenance.enter()
	if gateErr != nil {
		return Sample{}, gateErr
	}
	defer release()
	policy, err := s.CapturePolicy(ctx)
	if err != nil {
		return Sample{}, err
	}
	if in.ScaleCode == "" || in.ClientID == "" || s.devices == nil || s.camera == nil {
		return Sample{}, ErrConditions
	}
	if in.PlateEvidence != nil {
		value := in.PlateEvidence
		if (value.Source != "manual" && value.Source != "agent") || !maxLength(value.ScenePlate, 20) || !maxLength(value.Candidate, 20) || !maxLength(value.At, 100) {
			return Sample{}, ErrInvalid
		}
	}
	current, err := s.store.Session(ctx, in.ScaleCode)
	if err != nil {
		return Sample{}, err
	}
	if !validSession(current, e.Metadata.ActorID, in.SessionInput) {
		return Sample{}, ErrSession
	}
	if err := s.devices.RequireCapability(in.ScaleCode, p.CapabilityMeasurementWindow); err != nil {
		return Sample{}, ErrMeasurementCapability
	}
	// A new begin supersedes an unconfirmed window, so two captures on one
	// scale must not overlap. This lock spans network work but never the device
	// state lock (Hub.Guard is taken only for the local commit).
	unlock := s.lockCapture(in.ScaleCode)
	defer unlock()

	plan, err := s.camera.Plan(ctx, in.ScaleCode, agentbridge.ID(), policy.Required)
	if err != nil {
		return Sample{}, err
	}
	if policy.Required && len(plan.Cameras) == 0 {
		return Sample{}, ErrCaptureCameraRequired
	}
	begin, err := s.beginMeasurement(ctx, in.ScaleCode, policy.Revision, plan)
	if err != nil {
		return Sample{}, err
	}
	outcome, captureErr := s.camera.Capture(ctx, plan)
	if captureErr != nil || !outcome.Success {
		s.cancelMeasurement(in.ScaleCode, begin.Generation, "抓拍未满足策略")
		return Sample{}, ErrConditions
	}
	end, err := s.endMeasurement(ctx, in.ScaleCode, begin.Generation)
	if err != nil {
		s.cancelMeasurement(in.ScaleCode, begin.Generation, "测量结束核实失败")
		return Sample{}, err
	}
	// Recognition is optional and reuses an already saved image; a failure must
	// not discard a valid instrument and image basis.
	candidate, recognizedAt, plateSource := "", time.Time{}, ""
	if platform, recognizeErr := s.camera.RecognizeOutcome(ctx, plan, outcome); recognizeErr == nil && platform != "" {
		candidate, recognizedAt, plateSource = platform, time.Now().UTC(), "server"
	} else if in.PlateEvidence != nil && in.PlateEvidence.Source == "agent" {
		// The Agent still observes the scene plate for operator cross-check.
		// Recognition failure must not discard that observation, and the
		// evidence must keep its actual source instead of claiming Server work.
		observed := s.devices.State(in.ScaleCode).Plate
		candidate, recognizedAt, plateSource = observed.Candidate, observed.At, "agent"
	}
	// Take the device state lock only for the local commit, never across the
	// capture interval. Conditions are checked through sample persistence only;
	// later device changes do not revoke a successful sample.
	state, unlockState, err := s.devices.Guard(in.ScaleCode, 0)
	if err != nil {
		return Sample{}, ErrConditions
	}
	locked := true
	defer func() {
		if locked {
			unlockState()
		}
	}()
	if state.Weight != end.State.Weight || !state.Stable || !state.DeviceOK {
		return Sample{}, ErrConditions
	}
	captured := p.Capture{
		Success:     true,
		DeviceState: end.State.DeviceState,
		ImageStatus: captureImageStatus(plan, outcome),
		CapturedAt:  end.EndedAt.UTC(),
		Images:      outcome.Images,
		Plate:       p.Plate{Candidate: candidate, At: recognizedAt},
		PlateSource: plateSource,
		// The generation and its verification are the evidence that the basis
		// survived the whole image window.
		MeasurementGeneration: begin.Generation,
		MeasurementVerified:   true,
		CameraConfigVersion:   plan.ConfigVersion,
	}
	if len(outcome.Failures) > 0 {
		captured.ImageError = outcome.Failures[0].Message
	}
	sample, err := s.store.Capture(withServerCapture(withCapturePolicy(ctx, policy), state, &captured, outcome.Failures), in, e)
	// Release the device state lock before sending any further Agent command;
	// Command takes the same hub mutex.
	locked = false
	unlockState()
	if err != nil {
		// The measurement window has no persisted sample to confirm. Releasing
		// it is best effort; it is equally unusable after any later session or
		// policy change.
		s.cancelMeasurement(in.ScaleCode, begin.Generation, "采样保存失败")
		return Sample{}, err
	}
	return sample, nil
}

func captureImageStatus(plan camera.CapturePlan, outcome camera.CaptureOutcome) string {
	if plan.Required {
		return "complete"
	}
	if len(plan.Cameras) == 0 {
		return "disabled"
	}
	if len(outcome.Images) > 0 {
		return "complete"
	}
	return "capture_failed"
}

func (s *Service) beginMeasurement(ctx context.Context, scaleCode string, policyRevision string, plan camera.CapturePlan) (p.MeasurementBeginResult, error) {
	var result p.MeasurementBeginResult
	message, err := s.devices.Command(ctx, scaleCode, p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{
		CaptureID: plan.CaptureID, CapturePolicyRevision: policyRevision, CameraConfigVersion: plan.ConfigVersion,
	}, measurementCommandTimeout)
	if err != nil || message.Type != p.TypeMeasurementBeginResult || json.Unmarshal(message.Payload, &result) != nil {
		return result, ErrConditions
	}
	if !result.Accepted || result.Generation == "" || result.State.Unit != "t" || result.State.Weight <= 0 || !result.State.Stable || !result.State.DeviceOK {
		return result, ErrConditions
	}
	return result, nil
}

func (s *Service) endMeasurement(ctx context.Context, scaleCode string, generation string) (p.MeasurementEndResult, error) {
	var result p.MeasurementEndResult
	message, err := s.devices.Command(ctx, scaleCode, p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: generation}, measurementCommandTimeout)
	if err != nil || message.Type != p.TypeMeasurementEndResult || json.Unmarshal(message.Payload, &result) != nil {
		return result, ErrConditions
	}
	if !result.Verified || result.Generation != generation || result.EndedAt.IsZero() || result.State.Unit != "t" || result.State.Weight <= 0 || !result.State.Stable || !result.State.DeviceOK {
		return result, ErrConditions
	}
	return result, nil
}

// cancelMeasurement releases a window that will never be confirmed. Failure is
// harmless: an already superseded or disconnected window is equally unusable.
func (s *Service) cancelMeasurement(scaleCode string, generation, reason string) {
	if s.devices == nil || generation == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), measurementCancelTimeout)
	defer cancel()
	_, _ = s.devices.Command(ctx, scaleCode, p.TypeMeasurementCancelRequest, p.MeasurementCancelRequest{Generation: generation, Reason: reason}, measurementCancelTimeout)
}

func (s *Service) InvalidateSample(ctx context.Context, id int64, in SessionInput, actor int64) error {
	sample, lookupErr := s.store.Sample(ctx, id)
	if err := s.store.InvalidateSample(ctx, id, in, actor); err != nil {
		return err
	}
	if lookupErr == nil && sample.MeasurementGeneration != "" {
		if current, err := s.LocalStatus(ctx); err == nil && current.ScaleCode != "" {
			s.cancelMeasurement(current.ScaleCode, sample.MeasurementGeneration, "操作员废弃采样")
		}
	}
	return nil
}

func (s *Service) ConfirmFirst(ctx context.Context, in ConfirmationInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.SampleID < 1 || in.ScaleCode == "" || in.TaskID != nil {
		return CommandResult{}, ErrInvalid
	}
	return s.store.ConfirmFirst(ctx, in, e)
}
func (s *Service) Operation(ctx context.Context, actor int64, key string) (Operation, error) {
	if !validRequestID(key) {
		return Operation{}, ErrInvalid
	}
	return s.store.Operation(ctx, actor, key)
}
func (s *Service) FindTask(ctx context.Context, id int64) (TaskDetail, error) {
	return s.store.FindTask(ctx, id)
}
func (s *Service) ActiveTask(ctx context.Context, plate string) (*TaskDetail, error) {
	if !businessPlate.MatchString(normalizePlate(plate)) {
		return nil, ErrInvalid
	}
	return s.store.ActiveTask(ctx, plate)
}
func validRequestID(v string) bool { return len(v) >= 8 && len(v) <= 128 && strings.TrimSpace(v) == v }

func (s *Service) ConfirmSecond(ctx context.Context, id int64, in ConfirmationInput, e audit.Event) (CommandResult, error) {
	if id < 1 || !validRequestID(in.RequestID) || in.SampleID < 1 || in.ScaleCode == "" || in.Version < 1 {
		return CommandResult{}, ErrInvalid
	}
	in.TaskID = ptr(id)
	return s.store.ConfirmSecond(ctx, id, in, e)
}
func (s *Service) FindTicket(ctx context.Context, id int64) (TicketView, error) {
	return s.store.FindTicket(ctx, id)
}
func (s *Service) Tasks(ctx context.Context, q PageQuery) (Page[Task], error) {
	return s.store.Tasks(ctx, q)
}
func (s *Service) Tickets(ctx context.Context, q PageQuery) (Page[TicketView], error) {
	return s.store.Tickets(ctx, q)
}

// captureLocks serializes measurement windows per scale. A new begin replaces
// an unconfirmed window, so overlapping Server captures would invalidate each
// other. The map is tiny (one scale per site) and entries live for the process.
var captureLocks sync.Map

func (s *Service) lockCapture(scaleCode string) func() {
	value, _ := captureLocks.LoadOrStore(scaleCode, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}
