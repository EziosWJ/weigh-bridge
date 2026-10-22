package weighing

import (
	"context"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Session(ctx context.Context, id int64) (ScaleSession, error) {
	return s.store.Session(ctx, id)
}
func (s *Service) ChangeSession(ctx context.Context, id int64, action string, in SessionInput, e audit.Event) (SessionResult, error) {
	if id < 1 || strings.TrimSpace(in.ClientID) == "" || !maxLength(in.ClientID, 100) || !maxLength(in.Reason, 2000) {
		return SessionResult{}, ErrInvalid
	}
	return s.store.ChangeSession(ctx, id, action, in, e)
}
func (s *Service) Capture(ctx context.Context, in CaptureInput, e audit.Event) (Sample, error) {
	if s.devices == nil {
		return Sample{}, ErrConditions
	}
	if in.ScaleID < 1 || in.ClientID == "" {
		return Sample{}, ErrInvalid
	}
	if in.PlateEvidence != nil {
		value := in.PlateEvidence
		if (value.Source != "manual" && value.Source != "agent") || !maxLength(value.ScenePlate, 20) || !maxLength(value.Candidate, 20) || !maxLength(value.At, 100) {
			return Sample{}, ErrInvalid
		}
	}

	current, err := s.store.Session(ctx, in.ScaleID)
	if err != nil {
		return Sample{}, err
	}
	if !validSession(current, e.Metadata.ActorID, in.SessionInput) {
		return Sample{}, ErrSession
	}
	captured, revision, err := s.devices.Capture(ctx, in.ScaleID)
	if err != nil {
		return Sample{}, ErrConditions
	}
	state, unlock, err := s.devices.Guard(in.ScaleID, revision)
	if err != nil {
		return Sample{}, ErrConditions
	}
	defer unlock()
	return s.store.Capture(withDevice(ctx, state, &captured), in, e)
}
func (s *Service) InvalidateSample(ctx context.Context, id int64, in SessionInput, actor int64) error {
	return s.store.InvalidateSample(ctx, id, in, actor)
}
func (s *Service) ConfirmFirst(ctx context.Context, in ConfirmationInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.SampleID < 1 || in.ScaleID < 1 || in.TaskID != nil {
		return CommandResult{}, ErrInvalid
	}
	return s.deviceCommand(ctx, in.ScaleID, func(ctx context.Context) (CommandResult, error) { return s.store.ConfirmFirst(ctx, in, e) })
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
	if id < 1 || !validRequestID(in.RequestID) || in.SampleID < 1 || in.ScaleID < 1 || in.Version < 1 {
		return CommandResult{}, ErrInvalid
	}
	in.TaskID = ptr(id)
	return s.deviceCommand(ctx, in.ScaleID, func(ctx context.Context) (CommandResult, error) { return s.store.ConfirmSecond(ctx, id, in, e) })
}
func (s *Service) FindTicket(ctx context.Context, id int64) (TicketView, error) {
	v, err := s.store.FindTicket(ctx, id)
	if err == nil {
		s.decorateTicketImages(&v)
	}
	return v, err
}
func (s *Service) Tasks(ctx context.Context, q PageQuery) (Page[Task], error) {
	return s.store.Tasks(ctx, q)
}
func (s *Service) Tickets(ctx context.Context, q PageQuery) (Page[TicketView], error) {
	v, err := s.store.Tickets(ctx, q)
	if err == nil {
		for i := range v.Records {
			s.decorateTicketImages(&v.Records[i])
		}
	}
	return v, err
}
