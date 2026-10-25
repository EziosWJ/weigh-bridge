package weighing

import (
	"context"
	"sync"
)

// maintenanceGate rejects new device work before waiting for admitted work to
// finish. Unlike a bare RWMutex, waiting respects request cancellation.
type maintenanceGate struct {
	mu      sync.Mutex
	paused  bool
	active  int
	drained chan struct{}
}

func (g *maintenanceGate) enter() (func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return nil, ErrConditions
	}
	g.active++
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.active--
		if g.paused && g.active == 0 {
			close(g.drained)
		}
	}, nil
}
func (s *Service) BeginMaintenance(ctx context.Context) (func(), error) {
	g := &s.maintenance
	g.mu.Lock()
	if g.paused {
		g.mu.Unlock()
		return nil, ErrConditions
	}
	g.paused = true
	g.drained = make(chan struct{})
	if g.active == 0 {
		close(g.drained)
	}
	done := g.drained
	g.mu.Unlock()
	release := func() { g.mu.Lock(); g.paused = false; g.mu.Unlock() }
	select {
	case <-done:
		return release, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}

// The availability callback is supplied by the maintenance owner. Legacy
// configurations continue using the existing device evidence checks.
func (s *Service) SetCameraAvailability(fn func(int64) bool) { s.cameraAvailable = fn }

func (s *Service) captureBlockedReason(scale int64) string {
	return s.captureBlockedReasonWithPolicy(scale, true)
}
func (s *Service) captureBlockedReasonWithPolicy(scale int64, required bool) string {
	s.maintenance.mu.Lock()
	paused := s.maintenance.paused
	s.maintenance.mu.Unlock()
	if paused {
		return "设备维护中，暂停采集和确认"
	}
	// Server-owned cameras are validated by the frozen capture plan. The old
	// Agent camera availability callback must not block a Server capture.
	return ""
}
