package weighing

import (
	"context"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
)

type deviceKey struct{}
type deviceContext struct {
	State agentbridge.State
	// Capture is the legacy Agent capture result and its measured basis.
	Capture *p.Capture
	// Source distinguishes Server-owned images from historical Agent uploads.
	Source string
	// Failures keeps every camera's actual result, including optional capture.
	Failures []camera.CaptureFailure
}

func withDevice(ctx context.Context, state agentbridge.State, capture *p.Capture) context.Context {
	return context.WithValue(ctx, deviceKey{}, deviceContext{State: state, Capture: capture, Source: "agent"})
}

// withServerCapture carries the Server camera outcome into the local commit.
// The measurement generation and verification already live on captured.
func withServerCapture(ctx context.Context, state agentbridge.State, capture *p.Capture, failures []camera.CaptureFailure) context.Context {
	return context.WithValue(ctx, deviceKey{}, deviceContext{State: state, Capture: capture, Source: "server", Failures: failures})
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

// confirmCommand keeps the maintenance gate around the Agent measurement
// verification and the local commit. The device state lock is still taken only
// after the network round trip; a historical or already committed request may
// replay without any live device.
func (s *Service) confirmCommand(ctx context.Context, actor int64, requestID string, scale, sampleID int64, fn func(context.Context) (CommandResult, error)) (CommandResult, error) {
	release, err := s.maintenance.enter()
	if err != nil {
		return CommandResult{}, err
	}
	defer release()
	if err := s.verifySample(ctx, actor, requestID, sampleID); err != nil {
		return CommandResult{}, err
	}
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
