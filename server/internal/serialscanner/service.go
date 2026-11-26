package serialscanner

import (
	"context"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

const (
	// SourceSerialAgent labels handoffs triggered by the serial Agent.
	SourceSerialAgent = "serial_agent"
	// SourceOperator labels the legacy browser/token scan path.
	SourceOperator = "operator"
)

// ScanFunc processes one validated mobile QR through the normal factory
// weighing workflow. The handler supplies a system audit identity.
type ScanFunc func(ctx context.Context, code, source, device string) (weighing.MobileHandoffResult, error)

type Service struct {
	repo *Repository
	scan ScanFunc
	now  func() time.Time
}

func NewService(repo *Repository, scan ScanFunc) *Service {
	return &Service{repo: repo, scan: scan, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Config(ctx context.Context) (Config, error) { return s.repo.Config(ctx) }

func (s *Service) SaveConfig(ctx context.Context, in ConfigInput, actor int64) (Config, error) {
	normalized, err := ValidateInput(in)
	if err != nil {
		return Config{}, err
	}
	return s.repo.SaveConfig(ctx, normalized, actor)
}

func (s *Service) Heartbeat(ctx context.Context, in HeartbeatInput) (HeartbeatResult, error) {
	cfg, err := s.repo.Config(ctx)
	if err != nil {
		return HeartbeatResult{}, err
	}
	now := s.now()
	if err := s.repo.Heartbeat(ctx, in, now); err != nil {
		return HeartbeatResult{}, err
	}
	return HeartbeatResult{
		ServerTime: now, ConfigVersion: cfg.Version, Enabled: cfg.Enabled,
		Reload: in.ConfigVersion != cfg.Version,
	}, nil
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	cfg, err := s.repo.Config(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	status, err := s.repo.Status(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	snapshot := Snapshot{Config: cfg, Status: status, ServerTime: now}
	snapshot.Online = status.HeartbeatAt != nil && now.Sub(*status.HeartbeatAt) <= OnlineWindow
	return snapshot, nil
}

func (s *Service) Scan(ctx context.Context, code, source, device string) (weighing.MobileHandoffResult, error) {
	if s.scan == nil {
		return weighing.MobileHandoffResult{}, weighing.ErrForbidden
	}
	if source == "" {
		source = SourceSerialAgent
	}
	if device == "" {
		if cfg, err := s.repo.Config(ctx); err == nil {
			device = cfg.Device
		}
	}
	return s.scan(ctx, code, source, device)
}
