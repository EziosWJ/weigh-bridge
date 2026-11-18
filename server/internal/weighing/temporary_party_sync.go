package weighing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

type temporaryPartyOutbox struct {
	ID            int64
	PartyID       int64
	SiteCode      string
	TemporaryCode string
	Version       int64
	Payload       string
	Attempts      int
}

func (temporaryPartyOutbox) TableName() string { return "wb_temporary_party_outbox" }

// enqueueTemporaryPartyRegistration is called in the same transaction as the
// registration or correction. Retries always send the stored version verbatim.
func enqueueTemporaryPartyRegistration(tx *gorm.DB, siteCode string, party Party) error {
	payload, err := json.Marshal(temporaryPartyRegistration{
		SchemaVersion: 1,
		SiteCode:      siteCode,
		TemporaryCode: party.Code,
		Version:       party.Version,
		Name:          party.Name,
		Nature:        party.Nature,
		IsSupplier:    party.IsSupplier == 1,
		IsCustomer:    party.IsCustomer == 1,
		IsCarrier:     party.IsCarrier == 1,
	})
	if err != nil {
		return err
	}
	return tx.Create(&temporaryPartyOutbox{
		PartyID:       party.ID,
		SiteCode:      siteCode,
		TemporaryCode: party.Code,
		Version:       party.Version,
		Payload:       string(payload),
	}).Error
}

func (s *Service) wakeTemporaryPartySync() {
	select {
	case s.temporaryPartySyncWake <- struct{}{}:
	default:
	}
}

// RunTemporaryPartySyncWorker drains committed registrations on startup and
// after saves. The ticker recovers missed wakeups and runs scheduled retries.
func (s *Service) RunTemporaryPartySyncWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := s.RunTemporaryPartySync(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("处理临时往来主体登记队列失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.temporaryPartySyncWake:
		}
	}
}

// RunTemporaryPartySync attempts due registrations independently of tickets
// and of manual or scheduled master-data pulls.
func (s *Service) RunTemporaryPartySync(ctx context.Context) error {
	if !s.masterSyncEnabled {
		return nil
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil || !ready {
		return err
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	s.temporaryPartySyncMu.Lock()
	defer s.temporaryPartySyncMu.Unlock()
	const batchSize = 25
	var rows []temporaryPartyOutbox
	db := repo.db.WithContext(ctx)
	err = db.Where("confirmed_at IS NULL AND blocked_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP").
		Order("next_attempt_at, id").Limit(batchSize).Find(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		failure := s.syncV4Client.uploadTemporaryParty(ctx, s.masterSyncAddress, row)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := persistTemporaryPartyAttempt(db, row, failure, time.Now().UTC()); err != nil {
			return err
		}
	}
	if len(rows) == batchSize {
		s.wakeTemporaryPartySync()
	}
	return nil
}

func persistTemporaryPartyAttempt(db *gorm.DB, row temporaryPartyOutbox, failure *SyncCloudError, at time.Time) error {
	updates := map[string]any{
		"attempts":        row.Attempts + 1,
		"last_attempt_at": at,
		"next_attempt_at": nil,
		"last_error_kind": "",
	}
	category, reason := "", ""
	if failure == nil {
		updates["confirmed_at"] = at
	} else {
		category = failure.Category
		reason = fmt.Sprintf("厂区 %s，第 %d 版。%s", row.SiteCode, row.Version, failure.Message)
		updates["last_error_kind"] = failure.Reason
		if failure.Retryable {
			updates["next_attempt_at"] = at.Add(syncRetryBackoff(row.Attempts + 1))
		} else {
			updates["blocked_at"] = at
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&temporaryPartyOutbox{}).
			Where("id = ? AND attempts = ? AND confirmed_at IS NULL AND blocked_at IS NULL", row.ID, row.Attempts).
			Updates(updates)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return tx.Table("wb_master_sync_log").Create(map[string]any{
			"trigger_type":   "automatic",
			"sync_type":      "temp_party",
			"temporary_code": row.TemporaryCode,
			"category":       category,
			"success":        failure == nil,
			"reason":         reason,
		}).Error
	})
}
