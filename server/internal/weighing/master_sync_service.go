package weighing

import (
	"context"
	"errors"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/mastersync"
)

// SyncMaster serializes automatic and manual runs through the same path.
// The configuration is supplied by the caller and is never written to logs.
func (s *Service) SyncMaster(ctx context.Context, address, credential, trigger string) error {
	if trigger != "manual" && trigger != "automatic" {
		return ErrInvalid
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	s.masterSyncMu.Lock()
	defer s.masterSyncMu.Unlock()
	identity, err := localsite.Read(ctx, repo.db)
	fetchReason := ""
	if err == nil {
		var snapshot mastersync.Snapshot
		snapshot, err = mastersync.Fetch(ctx, address, credential, identity.SiteCode, identity.ScaleCode)
		if err != nil {
			fetchReason = err.Error()
		}
		if err == nil {
			err = repo.applyMasterSnapshot(ctx, snapshot)
		}
	}
	// Store only controlled messages: database errors can include SQL arguments.
	reason := ""
	if err != nil {
		reason = "资料获取、校验或应用失败"
		if fetchReason != "" {
			reason = fetchReason
		}
		var rejection RejectionError
		if errors.As(err, &rejection) {
			reason = rejection.Error()
		}
	}
	logErr := repo.db.WithContext(context.WithoutCancel(ctx)).Table("wb_master_sync_log").Create(map[string]any{"trigger_type": trigger, "success": err == nil, "reason": reason}).Error
	if err != nil {
		return RejectionError(reason)
	}
	return logErr
}

// ConfigureMasterSync applies the deployment switch before the HTTP server starts.
// A new enabled period requires manual synchronization; ordinary restarts do not.
func (s *Service) ConfigureMasterSync(ctx context.Context, enabled bool, address, credential string) error {
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		var state masterSyncState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, 1).Error; err != nil {
			return err
		}
		values := map[string]any{"config_enabled": enabled}
		if !enabled || !state.ConfigEnabled {
			values["bound"] = false
		}
		return tx.Model(&state).Updates(values).Error
	})
	if err == nil {
		s.masterSyncEnabled = enabled
		s.masterSyncAddress = address
		s.masterSyncCredential = credential
	}
	return err
}

type MasterSyncStatus struct {
	Enabled     bool       `json:"enabled"`
	Bound       bool       `json:"bound"`
	LastSuccess *time.Time `json:"lastSuccess"`
	IsAdmin     bool       `json:"isAdmin"`
}

func (s *Service) MasterSyncStatus(ctx context.Context) (MasterSyncStatus, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return MasterSyncStatus{}, ErrForbidden
	}
	var st masterSyncState
	if err := repo.db.WithContext(ctx).First(&st, 1).Error; err != nil {
		return MasterSyncStatus{}, err
	}
	return MasterSyncStatus{Enabled: s.masterSyncEnabled, Bound: st.Bound, LastSuccess: st.LastSuccess}, nil
}
func (s *Service) RunMasterSync(ctx context.Context, trigger string) error {
	if !s.masterSyncEnabled {
		return RejectionError("同步开关未开启")
	}
	if trigger == "automatic" {
		status, err := s.MasterSyncStatus(ctx)
		if err != nil {
			return err
		}
		if !status.Bound {
			return nil
		}
	}
	return s.SyncMaster(ctx, s.masterSyncAddress, s.masterSyncCredential, trigger)
}
func (s *Service) masterSyncReady(ctx context.Context) error {
	if !s.masterSyncEnabled {
		return nil
	}
	st, err := s.MasterSyncStatus(ctx)
	if err != nil {
		return err
	}
	if st.LastSuccess == nil {
		return RejectionError("请先由管理员完成首次总部资料同步")
	}
	return nil
}

func (s *Service) masterSyncAdmin(ctx context.Context, userID int64) (bool, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return false, ErrForbidden
	}
	var n int64
	err := repo.db.WithContext(ctx).Table("sys_user_role ur").Joins("JOIN sys_role r ON r.id = ur.role_id").Where("ur.user_id = ? AND r.role_code = ? AND r.status = 1 AND r.deleted = 0", userID, "ADMIN").Count(&n).Error
	return n > 0, err
}

type MasterSyncLog struct {
	ID          int64     `json:"id"`
	AttemptedAt time.Time `json:"attemptedAt"`
	TriggerType string    `json:"triggerType"`
	Success     bool      `json:"success"`
	Reason      string    `json:"reason"`
}

func (s *Service) masterSyncLogs(ctx context.Context) ([]MasterSyncLog, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrForbidden
	}
	rows := []MasterSyncLog{}
	err := repo.db.WithContext(ctx).Table("wb_master_sync_log").Order("attempted_at DESC, id DESC").Find(&rows).Error
	return rows, err
}
func (s *Service) clearMasterSyncLogs(ctx context.Context, event audit.Event) error {
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	event.Action = "master-sync.logs.clear"
	event.Resource = "weighing.master-sync"
	return repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM wb_master_sync_log").Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	})
}
