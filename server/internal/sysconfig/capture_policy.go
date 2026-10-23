package sysconfig

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// CapturePolicy is a snapshot of the local business rule, not a device setting.
type CapturePolicy struct {
	Required bool   `json:"captureRequired"`
	Revision string `json:"capturePolicyRevision"`
}

func DefaultCapturePolicy() CapturePolicy { return CapturePolicy{Required: true, Revision: "default"} }

// ReadCapturePolicy optionally holds a shared row lock until the business transaction commits.
func ReadCapturePolicy(ctx context.Context, db *gorm.DB, lock bool) (CapturePolicy, error) {
	var row Config
	q := db.WithContext(ctx).Where("config_key=? AND deleted=0", CaptureRequiredKey)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	err := q.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultCapturePolicy(), nil
	}
	if err != nil {
		return CapturePolicy{}, err
	}
	policy := DefaultCapturePolicy()
	policy.Revision = fmt.Sprintf("%d:%s:%d:%s:%s", row.ID, row.UpdateTime.UTC().Format(time.RFC3339Nano), row.Status, row.ValueType, row.ConfigValue)
	policy.Required = row.Status != 1 || row.ValueType != "BOOLEAN" || row.ConfigValue != "false"
	return policy, nil
}
