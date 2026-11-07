package weighing

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) dueSyncChangesV5(ctx context.Context, limit int) ([]ticketSyncChangeV5, error) {
	rows := []ticketSyncChangeV5{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.* FROM wb_ticket_sync_change_v5 c
		WHERE c.status IN ('pending','waiting_predecessor','temporary_failure')
		  AND (c.next_attempt_at IS NULL OR c.next_attempt_at <= now())
		  AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change_v5 p WHERE p.ticket_id = c.ticket_id AND p.id < c.id AND p.status <> 'synced')
		  AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_review r WHERE r.ticket_id = c.ticket_id AND r.status = 'open' AND r.blocking)
		  AND (c.previous_ticket_id IS NULL OR (
			EXISTS (SELECT 1 FROM wb_ticket_sync_change_v5 v WHERE v.ticket_id = c.previous_ticket_id AND v.change_type = 'void' AND v.status = 'synced')
			AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change_v5 p WHERE p.ticket_id = c.previous_ticket_id AND p.status <> 'synced')
		  ))
		ORDER BY c.next_attempt_at NULLS FIRST, c.id
		LIMIT ?`, limit).Scan(&rows).Error
	return rows, err
}

func (r *Repository) markSyncChangeV5Synced(ctx context.Context, id int64, receivedAt time.Time, at time.Time) error {
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("id = ?", id).Updates(map[string]any{
		"status":      SyncChangeSynced,
		"category":    "",
		"reason_code": "",
		"reason":      "",
		"synced_at":   at.UTC(),
		"received_at": receivedAt.UTC(),
	}).Error
}

func (r *Repository) markSyncChangeV5Failure(ctx context.Context, id int64, status, category, reasonCode, reason string, next *time.Time, at time.Time) error {
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("id = ?", id).Updates(map[string]any{
		"status":          status,
		"category":        category,
		"reason_code":     reasonCode,
		"reason":          reason,
		"attempts":        gorm.Expr("attempts + 1"),
		"next_attempt_at": next,
		"last_attempt_at": at.UTC(),
	}).Error
}

// RunSyncV5 uploads due v5 changes. Behaviour mirrors RunSyncV4; the payload
// and digest stay as written in the business transaction, and a cloud that
// does not accept v5 keeps the change queued with its full detail intact.
func (s *Service) RunSyncV5(ctx context.Context) error {
	if !s.masterSyncEnabled {
		return nil
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	changes, err := repo.dueSyncChangesV5(ctx, 100)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		receipt, cloudErr := s.syncV4Client.uploadChange(ctx, s.masterSyncAddress, change.Payload, change.Digest, change.TicketNo, change.ChangeType)
		now := time.Now().UTC()
		dbCtx := context.WithoutCancel(ctx)
		if cloudErr == nil {
			if err := repo.markSyncChangeV5Synced(dbCtx, change.ID, receipt.ReceivedAt, now); err != nil {
				return err
			}
			if err := repo.wakeWaitingDependentsV5(dbCtx, change.TicketID); err != nil {
				return err
			}
			if err := repo.writeSyncLog(dbCtx, "automatic", change.ChangeType, change.TicketNo, change.Digest, "", true, ""); err != nil {
				return err
			}
			continue
		}
		status, next := syncFailureState(cloudErr, change.Attempts+1)
		if err := repo.markSyncChangeV5Failure(dbCtx, change.ID, status, cloudErr.Category, cloudErr.Reason, cloudErr.Message, next, now); err != nil {
			return err
		}
		if err := repo.writeSyncLog(dbCtx, "automatic", change.ChangeType, change.TicketNo, change.Digest, cloudErr.Category, false, cloudErr.Message); err != nil {
			return err
		}
		if cloudErr.Category == SyncCategoryConflict {
			detail, _ := json.Marshal(map[string]any{"digest": change.Digest, "ticketNo": change.TicketNo, "changeType": change.ChangeType, "status": status, "category": cloudErr.Category, "reasonCode": cloudErr.Reason, "message": cloudErr.Message})
			if _, err := repo.upsertSyncReview(dbCtx, change.TicketID, change.Digest, cloudErr.Reason, string(detail), true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) wakeWaitingDependentsV5(ctx context.Context, ticketID int64) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("ticket_id = ? AND status = ?", ticketID, SyncChangeWaitingPredecessor).Update("next_attempt_at", now).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("previous_ticket_id = ? AND status = ?", ticketID, SyncChangeWaitingPredecessor).Update("next_attempt_at", now).Error
}
