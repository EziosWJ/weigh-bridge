package weighing

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type SyncTicketQuery struct {
	Page            int
	PageSize        int
	Keyword         string
	SyncStatus      string
	ReconcileStatus string
	Protocol        int
	From            string
	To              string
}

type SyncReconcileSummary struct {
	CheckedAt   *time.Time `json:"checkedAt"`
	Status      string     `json:"status"`
	Checked     int        `json:"checked"`
	Consistent  int        `json:"consistent"`
	Pending     int        `json:"pending"`
	NeedsReview int        `json:"needsReview"`
	Conflict    int        `json:"conflict"`
	Unknown     int        `json:"unknown"`
	Reason      string     `json:"reason"`
}

type SyncOverview struct {
	Enabled            bool                  `json:"enabled"`
	Bound              bool                  `json:"bound"`
	PendingTickets     int64                 `json:"pendingTickets"`
	NeedsActionTickets int64                 `json:"needsActionTickets"`
	LastSuccess        *time.Time            `json:"lastSuccess"`
	MasterLastSuccess  *time.Time            `json:"masterLastSuccess"`
	LastReconcile      *SyncReconcileSummary `json:"lastReconcile"`
}

type SyncChangeView struct {
	ID            int64      `json:"id"`
	Protocol      int        `json:"protocol"`
	Digest        string     `json:"digest,omitempty"`
	DigestVersion int        `json:"digestVersion,omitempty"`
	SchemaVersion int        `json:"schemaVersion,omitempty"`
	SiteCode      string     `json:"siteCode"`
	TicketNo      string     `json:"ticketNo"`
	ChangeType    string     `json:"changeType"`
	Status        string     `json:"status"`
	Category      string     `json:"category,omitempty"`
	ReasonCode    string     `json:"reasonCode,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt *time.Time `json:"nextAttemptAt"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	SyncedAt      *time.Time `json:"syncedAt"`
	ReceivedAt    *time.Time `json:"receivedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type SyncReconcileView struct {
	Status                   string          `json:"status"`
	CloudFound               *bool           `json:"cloudFound"`
	CloudStatus              *string         `json:"cloudStatus"`
	LocalLatestDigest        *string         `json:"localLatestDigest"`
	CloudLatestDigest        *string         `json:"cloudLatestDigest"`
	CloudLatestDigestVersion *int            `json:"cloudLatestDigestVersion"`
	CloudLatestChangeType    *string         `json:"cloudLatestChangeType"`
	CloudReceivedAt          *time.Time      `json:"cloudReceivedAt"`
	Detail                   json.RawMessage `json:"detail"`
	CheckedAt                time.Time       `json:"checkedAt"`
	TriggerType              string          `json:"triggerType"`
}

type SyncReviewView struct {
	ID              int64           `json:"id"`
	Digest          string          `json:"digest"`
	ReasonCode      string          `json:"reasonCode"`
	Status          string          `json:"status"`
	Blocking        bool            `json:"blocking"`
	Detail          json.RawMessage `json:"detail"`
	Conclusion      string          `json:"conclusion"`
	ConcludedByName string          `json:"concludedByName"`
	ConcludedAt     *time.Time      `json:"concludedAt"`
	LastCheckedAt   time.Time       `json:"lastCheckedAt"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type SyncTicketLink struct {
	ID       int64  `json:"id"`
	TicketNo string `json:"ticketNo"`
	Status   string `json:"status"`
}

type SyncTicketDetail struct {
	Ticket        TicketView         `json:"ticket"`
	Changes       []SyncChangeView   `json:"changes"`
	LegacyChanges []syncChangeV3View `json:"legacyChanges"`
	Reconcile     *SyncReconcileView `json:"reconcile"`
	Reviews       []SyncReviewView   `json:"reviews"`
	Predecessor   *SyncTicketLink    `json:"predecessor"`
	Successor     *SyncTicketLink    `json:"successor"`
}

func (r *Repository) SyncOverview(ctx context.Context) (SyncOverview, error) {
	result := SyncOverview{}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT ticket_id FROM wb_ticket_sync_change_v4 WHERE status <> 'synced'
			UNION
			SELECT c.ticket_id FROM wb_ticket_sync_change c
			WHERE c.status <> 'synced'
			  AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change_v4 v WHERE v.ticket_id = c.ticket_id)
		) x`).Scan(&result.PendingTickets).Error; err != nil {
		return result, err
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT ticket_id FROM wb_ticket_sync_change_v4 WHERE status IN ('needs_admin','conflict')
			UNION
			SELECT ticket_id FROM wb_ticket_sync_review WHERE status = 'open'
		) x`).Scan(&result.NeedsActionTickets).Error; err != nil {
		return result, err
	}
	var lastSuccess sql.NullTime
	if err := r.db.WithContext(ctx).Raw(`
		SELECT MAX(value) FROM (
			SELECT MAX(synced_at) AS value FROM wb_ticket_sync_change_v4 WHERE status = 'synced'
			UNION ALL
			SELECT MAX(synced_at) FROM wb_ticket_sync_change WHERE status = 'synced'
		) x`).Row().Scan(&lastSuccess); err != nil {
		return result, err
	}
	if lastSuccess.Valid {
		result.LastSuccess = &lastSuccess.Time
	}
	var masterLast sql.NullTime
	if err := r.db.WithContext(ctx).Raw("SELECT last_success FROM wb_master_sync_state WHERE id = 1").Row().Scan(&masterLast); err != nil {
		return result, err
	}
	if masterLast.Valid {
		result.MasterLastSuccess = &masterLast.Time
	}
	state, err := r.readSyncReconcileState(ctx)
	if err != nil {
		return result, err
	}
	if state.LastRunAt != nil {
		result.LastReconcile = &SyncReconcileSummary{CheckedAt: state.LastRunAt, Status: state.LastStatus, Checked: state.LastChecked, Consistent: state.LastConsistent, Pending: state.LastPending, NeedsReview: state.LastNeedsReview, Conflict: state.LastConflict, Unknown: state.LastUnknown, Reason: state.LastReason}
	}
	return result, nil
}

func (r *Repository) SyncTickets(ctx context.Context, q SyncTicketQuery) (Page[TicketView], error) {
	base := r.db.WithContext(ctx).Table("wb_ticket AS k").
		Joins("JOIN wb_transport_task t ON t.id=k.task_id").
		Joins("LEFT JOIN wb_ticket_reconcile r ON r.ticket_id=k.id").
		Select(`k.*,
			CASE WHEN EXISTS (SELECT 1 FROM wb_ticket_sync_change_v4 v WHERE v.ticket_id=k.id) THEN 4
			     WHEN EXISTS (SELECT 1 FROM wb_ticket_sync_change c WHERE c.ticket_id=k.id) THEN 3 ELSE 0 END AS sync_protocol,
			COALESCE(
				(SELECT v.status FROM wb_ticket_sync_change_v4 v WHERE v.ticket_id=k.id ORDER BY v.id DESC LIMIT 1),
				(SELECT c.status FROM wb_ticket_sync_change c WHERE c.ticket_id=k.id ORDER BY c.id DESC LIMIT 1),
				'untracked') AS sync_status,
			COALESCE(
				(SELECT v.reason FROM wb_ticket_sync_change_v4 v WHERE v.ticket_id=k.id ORDER BY v.id DESC LIMIT 1),
				(SELECT c.reason FROM wb_ticket_sync_change c WHERE c.ticket_id=k.id ORDER BY c.id DESC LIMIT 1),
				'') AS sync_reason,
			COALESCE(r.status,'') AS reconcile_status,
			r.checked_at AS reconcile_at`)
	if q.Keyword != "" {
		value := "%" + q.Keyword + "%"
		base = base.Where("k.ticket_no LIKE ? OR t.plate LIKE ?", value, value)
	}
	if q.From != "" || q.To != "" {
		from, to, err := dateBounds(PageQuery{From: q.From, To: q.To})
		if err != nil {
			return Page[TicketView]{}, err
		}
		if from != nil {
			base = base.Where("k.issued_at >= ?", *from)
		}
		if to != nil {
			base = base.Where("k.issued_at < ?", *to)
		}
	}
	wrapped := r.db.WithContext(ctx).Table("(?) AS x", base)
	if q.SyncStatus != "" {
		if q.SyncStatus == "untracked" {
			wrapped = wrapped.Where("x.sync_status = 'untracked'")
		} else {
			wrapped = wrapped.Where("x.sync_status = ?", q.SyncStatus)
		}
	}
	if q.ReconcileStatus != "" {
		wrapped = wrapped.Where("x.reconcile_status = ?", q.ReconcileStatus)
	}
	if q.Protocol > 0 {
		wrapped = wrapped.Where("x.sync_protocol = ?", q.Protocol)
	}
	result := Page[TicketView]{Records: []TicketView{}, Page: q.Page, PageSize: q.PageSize}
	if err := wrapped.Count(&result.Total).Error; err != nil {
		return result, err
	}
	rows := []Ticket{}
	if err := wrapped.Order("x.issued_at DESC,x.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result.Records = append(result.Records, ticketView(row))
	}
	return result, nil
}

func (r *Repository) SyncTicketDetail(ctx context.Context, ticketID int64) (SyncTicketDetail, error) {
	view, err := r.FindTicket(ctx, ticketID)
	if err != nil {
		return SyncTicketDetail{}, err
	}
	result := SyncTicketDetail{Ticket: view, Changes: []SyncChangeView{}, LegacyChanges: []syncChangeV3View{}, Reviews: []SyncReviewView{}}
	rows, err := r.syncChangesV4ForTicket(ctx, ticketID)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.Changes = append(result.Changes, syncChangeView(row))
	}
	legacy, err := r.syncChangesV3ForTicket(ctx, ticketID)
	if err != nil {
		return result, err
	}
	result.LegacyChanges = legacy
	reconcile, err := r.reconcileForTicket(ctx, ticketID)
	if err != nil {
		return result, err
	}
	if reconcile != nil {
		result.Reconcile = &SyncReconcileView{Status: reconcile.Status, CloudFound: reconcile.CloudFound, CloudStatus: reconcile.CloudStatus, LocalLatestDigest: reconcile.LocalLatestDigest, CloudLatestDigest: reconcile.CloudLatestDigest, CloudLatestDigestVersion: reconcile.CloudLatestDigestVersion, CloudLatestChangeType: reconcile.CloudLatestChangeType, CloudReceivedAt: reconcile.CloudReceivedAt, Detail: json.RawMessage(reconcile.Detail), CheckedAt: reconcile.CheckedAt, TriggerType: reconcile.TriggerType}
	}
	reviews := []ticketSyncReview{}
	if err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id DESC").Limit(50).Find(&reviews).Error; err != nil {
		return result, err
	}
	for _, review := range reviews {
		result.Reviews = append(result.Reviews, SyncReviewView{ID: review.ID, Digest: review.Digest, ReasonCode: review.ReasonCode, Status: review.Status, Blocking: review.Blocking, Detail: json.RawMessage(review.Detail), Conclusion: review.Conclusion, ConcludedByName: review.ConcludedByName, ConcludedAt: review.ConcludedAt, LastCheckedAt: review.LastCheckedAt, CreatedAt: review.CreatedAt})
	}
	if view.PreviousTicketID != nil {
		var previous Ticket
		if err := r.db.WithContext(ctx).First(&previous, *view.PreviousTicketID).Error; err == nil {
			result.Predecessor = &SyncTicketLink{ID: previous.ID, TicketNo: previous.TicketNo, Status: previous.Status}
		}
	}
	var successor Ticket
	if err := r.db.WithContext(ctx).Where("previous_ticket_id = ?", ticketID).Order("id DESC").Take(&successor).Error; err == nil {
		result.Successor = &SyncTicketLink{ID: successor.ID, TicketNo: successor.TicketNo, Status: successor.Status}
	}
	return result, nil
}

func syncChangeView(row ticketSyncChangeV4) SyncChangeView {
	return SyncChangeView{ID: row.ID, Protocol: SyncSchemaVersionV4, Digest: row.Digest, DigestVersion: row.DigestVersion, SchemaVersion: row.SchemaVersion, SiteCode: row.SiteCode, TicketNo: row.TicketNo, ChangeType: row.ChangeType, Status: row.Status, Category: row.Category, ReasonCode: row.ReasonCode, Reason: row.Reason, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt, LastAttemptAt: row.LastAttemptAt, SyncedAt: row.SyncedAt, ReceivedAt: row.ReceivedAt, CreatedAt: row.CreatedAt}
}

// ticketIDsForReconcile returns v4-tracked tickets in scope. Automatic runs
// cover changes from the last seven days plus any older unconfirmed change and
// open review. Manual runs may additionally narrow the range by issue date.
func (r *Repository) ticketIDsForReconcile(ctx context.Context, from, to *time.Time, automatic bool) ([]int64, error) {
	scope := `SELECT DISTINCT c.ticket_id
		FROM wb_ticket_sync_change_v4 c JOIN wb_ticket k ON k.id = c.ticket_id WHERE TRUE`
	args := []any{}
	if automatic {
		scope += ` AND (c.created_at >= now() - interval '7 days' OR c.status <> 'synced'
			OR c.ticket_id IN (SELECT ticket_id FROM wb_ticket_sync_review WHERE status = 'open'))`
	} else {
		if from != nil {
			scope += " AND k.issued_at >= ?"
			args = append(args, *from)
		}
		if to != nil {
			scope += " AND k.issued_at < ?"
			args = append(args, *to)
		}
	}
	query := r.db.WithContext(ctx).Raw(scope, args...)
	var rows []struct{ TicketID int64 }
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TicketID)
	}
	return ids, nil
}

func (r *Repository) syncTicketRef(ctx context.Context, ticketID int64) (syncTicketRef, error) {
	var row struct {
		TicketNo string
		SiteCode string
	}
	err := r.db.WithContext(ctx).Table("wb_ticket").Select("ticket_no, site_code").Where("id = ?", ticketID).Take(&row).Error
	return syncTicketRef{SiteCode: row.SiteCode, TicketNo: row.TicketNo}, err
}

func (r *Repository) syncTicketRefsForBatch(ctx context.Context, ids []int64) ([]syncTicketRef, error) {
	refs := make([]syncTicketRef, 0, len(ids))
	for _, id := range ids {
		ref, err := r.syncTicketRef(ctx, id)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
