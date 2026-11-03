package weighing

import (
	"context"
	"errors"
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

// AutoPrintEnabled defaults to false if the setting is absent or invalid.
func (s *Service) AutoPrintEnabled(ctx context.Context) (bool, error) {
	if repo, ok := s.store.(*Repository); ok {
		var row sysconfig.Config
		err := repo.db.WithContext(ctx).Where("config_key=? AND deleted=0", sysconfig.AutoPrintKey).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return row.Status == 1 && row.ValueType == "BOOLEAN" && row.ConfigValue == "true", nil
	}
	return false, nil
}
