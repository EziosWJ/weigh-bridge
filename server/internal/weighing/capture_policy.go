package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"gorm.io/gorm"
)

type capturePolicyKey struct{}

func (s *Service) CapturePolicy(ctx context.Context) (sysconfig.CapturePolicy, error) {
	if repo, ok := s.store.(*Repository); ok {
		return sysconfig.ReadCapturePolicy(ctx, repo.db, false)
	}
	return sysconfig.DefaultCapturePolicy(), nil
}
func withCapturePolicy(ctx context.Context, policy sysconfig.CapturePolicy) context.Context {
	return context.WithValue(ctx, capturePolicyKey{}, policy)
}
func capturePolicyMatches(ctx context.Context, tx *gorm.DB) (sysconfig.CapturePolicy, error) {
	current, err := sysconfig.ReadCapturePolicy(ctx, tx, true)
	if err != nil {
		return current, err
	}
	expected, ok := ctx.Value(capturePolicyKey{}).(sysconfig.CapturePolicy)
	if !ok || current != expected {
		return current, ErrCapturePolicyChanged
	}
	return current, nil
}
