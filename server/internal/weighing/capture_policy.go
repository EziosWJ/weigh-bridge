package weighing

import (
	"context"
	"encoding/json"
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
func validateSamplePolicy(ctx context.Context, tx *gorm.DB, sample Sample) error {
	current, err := sysconfig.ReadCapturePolicy(ctx, tx, true)
	if err != nil {
		return err
	}
	var saved struct {
		Required *bool  `json:"captureRequired"`
		Revision string `json:"capturePolicyRevision"`
	}
	if json.Unmarshal(sample.Evidence, &saved) != nil || saved.Required == nil || *saved.Required != current.Required || saved.Revision != current.Revision {
		return ErrCapturePolicyChanged
	}
	return nil
}
