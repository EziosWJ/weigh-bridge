package weighing

import (
	"context"
	"time"
)

// syncChangesV5ForTicket lists the v5 rows of one ticket in change order.
func (r *Repository) syncChangesV5ForTicket(ctx context.Context, ticketID int64) ([]ticketSyncChangeV5, error) {
	rows := []ticketSyncChangeV5{}
	err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id").Find(&rows).Error
	return rows, err
}

func (r *Repository) syncChangeV5ByID(ctx context.Context, id int64) (ticketSyncChangeV5, error) {
	var row ticketSyncChangeV5
	err := r.db.WithContext(ctx).First(&row, id).Error
	if err != nil {
		return row, ErrNotFound
	}
	return row, err
}

// ticketSyncChanges returns every digest-protocol change of a ticket across
// v4 and v5 in change order, normalized onto the v4 shape so reconcile and
// retry logic stays shared. Only one protocol exists per ticket.
func (r *Repository) ticketSyncChanges(ctx context.Context, ticketID int64) ([]ticketSyncChangeV4, bool, error) {
	v5, err := r.syncChangesV5ForTicket(ctx, ticketID)
	if err != nil {
		return nil, false, err
	}
	if len(v5) > 0 {
		rows := make([]ticketSyncChangeV4, 0, len(v5))
		for _, row := range v5 {
			rows = append(rows, ticketSyncChangeV4{
				ID: row.ID, TicketID: row.TicketID, PreviousTicketID: row.PreviousTicketID,
				Digest: row.Digest, DigestVersion: row.DigestVersion, SchemaVersion: row.SchemaVersion,
				SiteCode: row.SiteCode, TicketNo: row.TicketNo, ChangeType: row.ChangeType,
				Payload: row.Payload, Status: row.Status, Category: row.Category,
				ReasonCode: row.ReasonCode, Reason: row.Reason, Attempts: row.Attempts,
				NextAttemptAt: row.NextAttemptAt, LastAttemptAt: row.LastAttemptAt,
				SyncedAt: row.SyncedAt, ReceivedAt: row.ReceivedAt, CreatedAt: row.CreatedAt,
			})
		}
		return rows, true, nil
	}
	rows, err := r.syncChangesV4ForTicket(ctx, ticketID)
	return rows, false, err
}

// markTicketSyncChangeSynced marks the row in whichever table holds it.
func (r *Repository) markTicketSyncChangeSynced(ctx context.Context, id int64, v5 bool, receivedAt, at time.Time) error {
	if v5 {
		return r.markSyncChangeV5Synced(ctx, id, receivedAt, at)
	}
	return r.markSyncChangeV4Synced(ctx, id, receivedAt, at)
}

func (r *Repository) pauseTicketSyncChange(ctx context.Context, id int64, v5 bool, category, reasonCode, reason string) error {
	if v5 {
		return r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("id = ?", id).Updates(map[string]any{
			"status": SyncChangeConflict, "category": category, "reason_code": reasonCode, "reason": reason, "next_attempt_at": nil,
		}).Error
	}
	return r.pauseSyncChangeV4(ctx, id, category, reasonCode, reason)
}

func (r *Repository) resetTicketSyncChangesForResend(ctx context.Context, ticketID int64, v5 bool) error {
	if !v5 {
		return r.resetTicketChangesForResend(ctx, ticketID)
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&ticketSyncChangeV5{}).Where("ticket_id = ?", ticketID).Updates(map[string]any{
		"status": SyncChangePending, "category": "", "reason_code": "",
		"reason": "对账确认云端缺单，按原变化补传", "attempts": 0,
		"next_attempt_at": now, "synced_at": nil, "received_at": nil,
	}).Error
}

// changeWaitsForPredecessorAny mirrors changeWaitsForPredecessor across v4/v5.
func (r *Repository) changeWaitsForPredecessorAny(ctx context.Context, change ticketSyncChangeV4, v5 bool) (bool, error) {
	table := "wb_ticket_sync_change_v4"
	if v5 {
		table = "wb_ticket_sync_change_v5"
	}
	var count int64
	if err := r.db.WithContext(ctx).Table(table).Where("ticket_id = ? AND id < ? AND status <> 'synced'", change.TicketID, change.ID).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if change.PreviousTicketID == nil {
		return false, nil
	}
	if err := r.db.WithContext(ctx).Table(table).Where("ticket_id = ? AND change_type = 'void' AND status = 'synced'", *change.PreviousTicketID).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	if err := r.db.WithContext(ctx).Table(table).Where("ticket_id = ? AND status <> 'synced'", *change.PreviousTicketID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) latestTicketSyncChange(ctx context.Context, ticketID int64) (*ticketSyncChangeV4, bool, error) {
	rows, v5, err := r.ticketSyncChanges(ctx, ticketID)
	if err != nil || len(rows) == 0 {
		return nil, v5, err
	}
	return &rows[len(rows)-1], v5, nil
}
