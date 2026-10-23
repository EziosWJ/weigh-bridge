package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

type deviceKey struct{}
type deviceContext struct {
	State   agentbridge.State
	Capture *p.Capture
}

func withDevice(ctx context.Context, state agentbridge.State, capture *p.Capture) context.Context {
	return context.WithValue(ctx, deviceKey{}, deviceContext{state, capture})
}
func (s *Service) deviceCommand(ctx context.Context, scale int64, fn func(context.Context) (CommandResult, error)) (CommandResult, error) {
	release, err := s.maintenance.enter()
	if err != nil {
		return CommandResult{}, err
	}
	defer release()
	// Idempotent command replay is still resolved by the repository before validating a new sample.
	// A missing/offline device context rejects fresh samples, while stored operations can replay.
	if s.devices == nil {
		return fn(ctx)
	}
	state, unlock, err := s.devices.Guard(scale, 0)
	if err != nil {
		return fn(ctx)
	}
	defer unlock()
	return fn(withDevice(ctx, state, nil))
}
