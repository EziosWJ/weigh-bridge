package weighing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) dueSyncChangesV4(ctx context.Context, limit int) ([]ticketSyncChangeV4, error) {
	rows := []ticketSyncChangeV4{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.* FROM wb_ticket_sync_change_v4 c
		WHERE c.status IN ('pending','waiting_predecessor','temporary_failure')
		  AND (c.next_attempt_at IS NULL OR c.next_attempt_at <= now())
		  AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change_v4 p WHERE p.ticket_id = c.ticket_id AND p.id < c.id AND p.status <> 'synced')
		  AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_review r WHERE r.ticket_id = c.ticket_id AND r.status = 'open' AND r.blocking)
		  AND (c.previous_ticket_id IS NULL OR (
			EXISTS (SELECT 1 FROM wb_ticket_sync_change_v4 v WHERE v.ticket_id = c.previous_ticket_id AND v.change_type = 'void' AND v.status = 'synced')
			AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change_v4 p WHERE p.ticket_id = c.previous_ticket_id AND p.status <> 'synced')
		  ))
		ORDER BY c.next_attempt_at NULLS FIRST, c.id
		LIMIT ?`, limit).Scan(&rows).Error
	return rows, err
}

func (r *Repository) syncChangeV4ByID(ctx context.Context, id int64) (ticketSyncChangeV4, error) {
	var row ticketSyncChangeV4
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

func (r *Repository) syncChangesV4ForTicket(ctx context.Context, ticketID int64) ([]ticketSyncChangeV4, error) {
	rows := []ticketSyncChangeV4{}
	err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id").Find(&rows).Error
	return rows, err
}

// syncChangeV3View exposes legacy change state for the detail page without
// changing or migrating the v3 outbox.
type syncChangeV3View struct {
	ID            int64      `json:"id"`
	ChangeID      string     `json:"changeId"`
	TicketNo      string     `json:"ticketNo"`
	ChangeType    string     `json:"changeType"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	SyncedAt      *time.Time `json:"syncedAt"`
}

func (r *Repository) syncChangesV3ForTicket(ctx context.Context, ticketID int64) ([]syncChangeV3View, error) {
	rows := []syncChangeV3View{}
	err := r.db.WithContext(ctx).Table("wb_ticket_sync_change").Select("id, change_id, ticket_no, change_type, status, reason, created_at, last_attempt_at, synced_at").Where("ticket_id = ?", ticketID).Order("id").Scan(&rows).Error
	return rows, err
}

func (r *Repository) markSyncChangeV4Synced(ctx context.Context, id int64, receivedAt time.Time, at time.Time) error {
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("id = ?", id).Updates(map[string]any{
		"status":      SyncChangeSynced,
		"category":    "",
		"reason_code": "",
		"reason":      "",
		"synced_at":   at.UTC(),
		"received_at": receivedAt.UTC(),
	}).Error
}

// wakeWaitingDependents makes the next change of the same ticket and any
// reopen waiting on this ticket eligible immediately.
func (r *Repository) wakeWaitingDependents(ctx context.Context, ticketID int64) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("ticket_id = ? AND status = ?", ticketID, SyncChangeWaitingPredecessor).Update("next_attempt_at", now).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("previous_ticket_id = ? AND status = ?", ticketID, SyncChangeWaitingPredecessor).Update("next_attempt_at", now).Error
}

func (r *Repository) markSyncChangeV4Failure(ctx context.Context, id int64, status, category, reasonCode, reason string, nextAttempt *time.Time, at time.Time) error {
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("id = ?", id).Updates(map[string]any{
		"status":          status,
		"category":        category,
		"reason_code":     reasonCode,
		"reason":          reason,
		"attempts":        gorm.Expr("attempts + 1"),
		"last_attempt_at": at.UTC(),
		"next_attempt_at": nextAttempt,
	}).Error
}

func (r *Repository) pauseSyncChangeV4(ctx context.Context, id int64, category, reasonCode, reason string) error {
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("id = ?", id).Updates(map[string]any{
		"status":          SyncChangeConflict,
		"category":        category,
		"reason_code":     reasonCode,
		"reason":          reason,
		"next_attempt_at": nil,
	}).Error
}

// resetTicketChangesForResend clears local confirmation for every change of a
// ticket that the cloud no longer has. Payload and digest stay untouched so
// the original changes are replayed in order.
func (r *Repository) resetTicketChangesForResend(ctx context.Context, ticketID int64) error {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("ticket_id = ?", ticketID).Updates(map[string]any{
		"status":          SyncChangePending,
		"category":        "",
		"reason_code":     "",
		"reason":          "对账确认云端缺单，按原变化补传",
		"attempts":        0,
		"next_attempt_at": now,
		"synced_at":       nil,
		"received_at":     nil,
	}).Error
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) ticketHasBlockingReview(ctx context.Context, ticketID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("wb_ticket_sync_review").Where("ticket_id = ? AND status = 'open' AND blocking", ticketID).Count(&count).Error
	return count > 0, err
}

// changeWaitsForPredecessor reports whether an earlier change of the same
// ticket, or the original ticket's void for a reopen, is still unconfirmed.
func (r *Repository) changeWaitsForPredecessor(ctx context.Context, change ticketSyncChangeV4) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("wb_ticket_sync_change_v4").Where("ticket_id = ? AND id < ? AND status <> 'synced'", change.TicketID, change.ID).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if change.PreviousTicketID == nil {
		return false, nil
	}
	if err := r.db.WithContext(ctx).Table("wb_ticket_sync_change_v4").Where("ticket_id = ? AND change_type = 'void' AND status = 'synced'", *change.PreviousTicketID).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	if err := r.db.WithContext(ctx).Table("wb_ticket_sync_change_v4").Where("ticket_id = ? AND status <> 'synced'", *change.PreviousTicketID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) latestSyncChangeV4(ctx context.Context, ticketID int64) (*ticketSyncChangeV4, error) {
	var row ticketSyncChangeV4
	err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) upsertTicketReconcile(ctx context.Context, row ticketReconcile) error {
	detail := row.Detail
	if detail == "" {
		detail = "{}"
	}
	values := map[string]any{
		"ticket_id":                   row.TicketID,
		"site_code":                   row.SiteCode,
		"ticket_no":                   row.TicketNo,
		"status":                      row.Status,
		"cloud_found":                 row.CloudFound,
		"cloud_status":                row.CloudStatus,
		"local_latest_digest":         row.LocalLatestDigest,
		"cloud_latest_digest":         row.CloudLatestDigest,
		"cloud_latest_digest_version": row.CloudLatestDigestVersion,
		"cloud_latest_change_type":    row.CloudLatestChangeType,
		"cloud_received_at":           row.CloudReceivedAt,
		"detail":                      gorm.Expr("?::jsonb", detail),
		"checked_at":                  row.CheckedAt.UTC(),
		"trigger_type":                row.TriggerType,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ticket_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"site_code", "ticket_no", "status", "cloud_found", "cloud_status", "local_latest_digest", "cloud_latest_digest", "cloud_latest_digest_version", "cloud_latest_change_type", "cloud_received_at", "detail", "checked_at", "trigger_type"}),
	}).Table("wb_ticket_reconcile").Create(values).Error
}

func (r *Repository) reconcileForTicket(ctx context.Context, ticketID int64) (*ticketReconcile, error) {
	var row ticketReconcile
	err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) upsertSyncReview(ctx context.Context, ticketID int64, digest, reasonCode, detail string, blocking bool) (ticketSyncReview, error) {
	now := time.Now().UTC()
	if detail == "" {
		detail = "{}"
	}
	var existing ticketSyncReview
	err := r.db.WithContext(ctx).Where("ticket_id = ? AND digest = ? AND reason_code = ? AND status = ?", ticketID, digest, reasonCode, SyncReviewOpen).Take(&existing).Error
	if err == nil && existing.Status == SyncReviewOpen {
		// A repeated automatic check must not erase an administrator's
		// conclusion for the same unresolved difference.
		if err := r.db.WithContext(ctx).Model(&ticketSyncReview{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"blocking":        blocking,
			"detail":          gorm.Expr("?::jsonb", detail),
			"last_checked_at": now,
			"updated_at":      now,
		}).Error; err != nil {
			return existing, err
		}
		return r.syncReviewByID(ctx, existing.ID)
	}
	row := ticketSyncReview{TicketID: ticketID, Digest: digest, ReasonCode: reasonCode, Status: SyncReviewOpen, Blocking: blocking, Detail: detail, LastCheckedAt: now, CreatedAt: now, UpdatedAt: now}
	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "ticket_id"}, {Name: "digest"}, {Name: "reason_code"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "status = 'open'"}}},
		DoUpdates: clause.Assignments(map[string]any{
			"blocking":        blocking,
			"detail":          gorm.Expr("?::jsonb", detail),
			"last_checked_at": now,
			"updated_at":      now,
		}),
	}).Create(&row).Error
	if err != nil {
		return row, err
	}
	return r.syncReviewByID(ctx, row.ID)
}

func (r *Repository) syncReviewByID(ctx context.Context, id int64) (ticketSyncReview, error) {
	var row ticketSyncReview
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

func (r *Repository) openSyncReviewsForTicket(ctx context.Context, ticketID int64) ([]ticketSyncReview, error) {
	rows := []ticketSyncReview{}
	err := r.db.WithContext(ctx).Where("ticket_id = ? AND status = 'open'", ticketID).Order("id").Find(&rows).Error
	return rows, err
}

func (r *Repository) resolveReviewsForTicket(ctx context.Context, ticketID int64, status, reason, detail string) error {
	updates := map[string]any{"status": status, "blocking": false, "updated_at": time.Now().UTC(), "last_checked_at": time.Now().UTC()}
	if detail != "" {
		updates["detail"] = gorm.Expr("?::jsonb", detail)
	}
	return r.db.WithContext(ctx).Model(&ticketSyncReview{}).Where("ticket_id = ? AND status = 'open'", ticketID).Updates(updates).Error
}

func (r *Repository) resolveSyncReview(ctx context.Context, id int64, detail string) error {
	updates := map[string]any{"status": SyncReviewResolved, "blocking": false, "updated_at": time.Now().UTC(), "last_checked_at": time.Now().UTC()}
	if detail != "" {
		updates["detail"] = gorm.Expr("?::jsonb", detail)
	}
	return r.db.WithContext(ctx).Model(&ticketSyncReview{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) recordSyncReviewConclusion(ctx context.Context, id, actorID int64, actorName, conclusion string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&ticketSyncReview{}).Where("id = ? AND status = 'open'", id).Updates(map[string]any{
		"conclusion":        conclusion,
		"concluded_by":      actorID,
		"concluded_by_name": actorName,
		"concluded_at":      now,
		"updated_at":        now,
		"last_checked_at":   now,
	}).Error
}

func (r *Repository) resetConflictChangesForTicket(ctx context.Context, ticketID int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV4{}).Where("ticket_id = ? AND status = ?", ticketID, SyncChangeConflict).Updates(map[string]any{
		"status":          SyncChangePending,
		"category":        "",
		"reason_code":     "",
		"reason":          "重新核查通过，等待上传",
		"attempts":        0,
		"next_attempt_at": now,
	}).Error
}

func (r *Repository) readSyncReconcileState(ctx context.Context) (syncReconcileState, error) {
	var state syncReconcileState
	err := r.db.WithContext(ctx).First(&state, 1).Error
	return state, err
}

func (r *Repository) writeSyncReconcileState(ctx context.Context, state syncReconcileState) error {
	values := map[string]any{
		"last_auto_date":    state.LastAutoDate,
		"last_auto_success": state.LastAutoSuccess,
		"last_run_at":       state.LastRunAt,
		"last_status":       state.LastStatus,
		"last_checked":      state.LastChecked,
		"last_consistent":   state.LastConsistent,
		"last_pending":      state.LastPending,
		"last_needs_review": state.LastNeedsReview,
		"last_conflict":     state.LastConflict,
		"last_unknown":      state.LastUnknown,
		"last_reason":       state.LastReason,
	}
	return r.db.WithContext(ctx).Model(&syncReconcileState{}).Where("id = 1").Updates(values).Error
}

func (r *Repository) writeSyncLog(ctx context.Context, trigger, syncType, ticketNo, digest, category string, success bool, reason string) error {
	return r.db.WithContext(ctx).Table("wb_master_sync_log").Create(map[string]any{
		"trigger_type": trigger,
		"sync_type":    syncType,
		"ticket_no":    ticketNo,
		"digest":       digest,
		"category":     category,
		"success":      success,
		"reason":       reason,
	}).Error
}
