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
