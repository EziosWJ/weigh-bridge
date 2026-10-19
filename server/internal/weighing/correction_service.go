package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"strings"
)

func validReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && maxLength(reason, 2000)
}
func (s *Service) CorrectTask(ctx context.Context, id int64, in CorrectionInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.Version < 1 || !validReason(in.Reason) {
		return CommandResult{}, ErrInvalid
	}
	return s.store.CorrectTask(ctx, id, in, e)
}
func (s *Service) TerminateTask(ctx context.Context, id int64, in TaskActionInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.Version < 1 || !validReason(in.Reason) {
		return CommandResult{}, ErrInvalid
	}
	return s.store.TerminateTask(ctx, id, in, e)
}
func (s *Service) VoidTicket(ctx context.Context, id int64, in VoidInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || !validReason(in.Reason) {
		return CommandResult{}, ErrInvalid
	}
	return s.store.VoidTicket(ctx, id, in, e)
}
func (s *Service) Reweigh(ctx context.Context, id int64, in ReweighInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.Version < 1 || !validReason(in.Reason) {
		return CommandResult{}, ErrInvalid
	}
	return s.deviceCommand(ctx, in.ScaleID, func(ctx context.Context) (CommandResult, error) { return s.store.Reweigh(ctx, id, in, e) })
}
func (s *Service) ReopenTicket(ctx context.Context, id int64, in ReopenInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || in.TaskVersion < 1 || !validReason(in.Reason) {
		return CommandResult{}, ErrInvalid
	}
	return s.store.ReopenTicket(ctx, id, in, e)
}
