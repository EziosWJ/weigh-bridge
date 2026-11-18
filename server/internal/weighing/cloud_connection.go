package weighing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gorm.io/gorm"
)

type CloudConnection struct {
	Enabled     bool               `json:"enabled"`
	Address     string             `json:"address"`
	SiteCode    string             `json:"siteCode"`
	Bound       bool               `json:"bound"`
	LastSuccess *time.Time         `json:"lastSuccess"`
	LastAttempt *MasterSyncAttempt `json:"lastAttempt"`
	Readiness   CloudReadiness     `json:"readiness"`
}

type MasterSyncAttempt struct {
	AttemptedAt time.Time `json:"attemptedAt"`
	Success     bool      `json:"success"`
	Reason      string    `json:"reason"`
}

type CloudReadiness struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checkedAt"`
}

// CloudConnection reports the runtime settings and probes the configured cloud
// without applying master data or recording any synchronization activity.
func (s *Service) CloudConnection(ctx context.Context) (CloudConnection, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return CloudConnection{}, ErrForbidden
	}
	status, err := s.MasterSyncStatus(ctx)
	if err != nil {
		return CloudConnection{}, err
	}
	local, err := s.LocalStatus(ctx)
	if err != nil {
		return CloudConnection{}, err
	}
	out := CloudConnection{Enabled: status.Enabled, SiteCode: local.SiteCode, Bound: status.Bound, LastSuccess: status.LastSuccess}
	var attempt MasterSyncAttempt
	err = repo.db.WithContext(ctx).Table("wb_master_sync_log").
		Select("attempted_at", "success", "reason").
		Where("sync_type = ?", "master").
		Order("attempted_at DESC, id DESC").Take(&attempt).Error
	switch {
	case err == nil:
		out.LastAttempt = &attempt
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return CloudConnection{}, err
	}
	target, err := syncV4URL(s.masterSyncAddress, "/ready", nil)
	if err == nil {
		out.Address = s.masterSyncAddress
	}
	switch {
	case !status.Enabled:
		out.Readiness.Status = "disabled"
	case s.masterSyncAddress == "":
		out.Readiness.Status = "unconfigured"
	case err != nil:
		out.Readiness.Status = "invalid_address"
	default:
		out.Readiness = s.checkCloudReadiness(ctx, target)
	}
	return out, nil
}

func (s *Service) checkCloudReadiness(ctx context.Context, target string) CloudReadiness {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	envelope, httpStatus, err := s.syncV4Client.do(ctx, http.MethodGet, target, nil)
	checkedAt := time.Now().UTC()
	result := CloudReadiness{Status: "unexpected_response", CheckedAt: &checkedAt}
	if err != nil {
		if httpStatus == 0 {
			result.Status = "unreachable"
		}
		return result
	}
	if httpStatus == http.StatusServiceUnavailable {
		result.Status = "not_ready"
		return result
	}
	if httpStatus == http.StatusOK && envelope.Code == http.StatusOK {
		var data struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(envelope.Data, &data) == nil && data.Status == "ok" {
			result.Status = "ready"
		}
	}
	return result
}
