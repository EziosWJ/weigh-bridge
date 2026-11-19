package weighing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type taskSyncItem struct {
	TaskNo         string               `json:"taskNo"`
	Version        int64                `json:"version"`
	Status         string               `json:"status"`
	Plate          string               `json:"plate"`
	Direction      string               `json:"direction"`
	MaterialCode   string               `json:"materialCode"`
	MaterialName   string               `json:"materialName"`
	PartyCode      string               `json:"partyCode"`
	PartyName      string               `json:"partyName"`
	FirstWeighedAt time.Time            `json:"firstWeighedAt"`
	Termination    *taskSyncTermination `json:"termination"`
	Ticket         *taskSyncTicket      `json:"ticket"`
}
type taskSyncTermination struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Reason string    `json:"reason"`
}
type taskSyncTicket struct {
	SiteCode string `json:"siteCode"`
	TicketNo string `json:"ticketNo"`
}
type taskSyncOutbox struct {
	ID            int64
	TaskID        int64
	SiteCode      string
	TaskNo        string
	Version       int64
	Payload       string
	Attempts      int
	NextAttemptAt time.Time
	ConfirmedAt   *time.Time
	LastError     string
}

func (taskSyncOutbox) TableName() string { return "wb_task_sync_outbox" }

type taskSyncRejection struct {
	status int
	code   int
}

func (e *taskSyncRejection) Error() string {
	return fmt.Sprintf("task sync rejected: HTTP %d, code %d", e.status, e.code)
}

// The factory identity can be changed without rewriting outbox snapshots.
const oldTaskSyncSite = "site_code <> (SELECT site_code FROM wb_local_identity WHERE id = 1 AND site_code <> '')"

// enqueueTaskSync captures the business snapshot in the same transaction as the
// task change. Replays always use these bytes, including the original version.
func enqueueTaskSync(tx *gorm.DB, task Task, actor string) error {
	if task.CurrentFirstRecordID == nil {
		return fmt.Errorf("task %s has no first record", task.TaskNo)
	}
	first, err := findOne[WeighRecord](tx, *task.CurrentFirstRecordID)
	if err != nil {
		return err
	}
	item := taskSyncItem{TaskNo: task.TaskNo, Version: task.Version, Status: task.Status, Plate: task.Plate, Direction: task.Direction, MaterialCode: task.Business.MaterialCode, MaterialName: task.Business.MaterialName, PartyCode: task.Business.PartyCode, PartyName: task.Business.PartyName, FirstWeighedAt: first.CapturedAt}
	if task.Status == "terminated" {
		if task.TerminatedAt == nil {
			return ErrInvalid
		}
		item.Termination = &taskSyncTermination{At: *task.TerminatedAt, Actor: actor, Reason: task.TerminateReason}
	}
	if task.Status == "completed" {
		var ticket Ticket
		if err := tx.Where("task_id=?", task.ID).Order("revision ASC").First(&ticket).Error; err != nil {
			return err
		}
		item.Ticket = &taskSyncTicket{SiteCode: ticket.SiteCode, TicketNo: ticket.TicketNo}
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "task_id"}, {Name: "version"}}, DoNothing: true}).Create(&taskSyncOutbox{TaskID: task.ID, SiteCode: task.SiteCode, TaskNo: task.TaskNo, Version: task.Version, Payload: string(data), NextAttemptAt: time.Now().UTC()}).Error
}

// Bootstrap and business commands serialize on task row locks; the outbox's
// unique version key handles a task already captured by an intervening command.
func (r *Repository) bootstrapTaskSync(ctx context.Context) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("wb_task_sync_bootstrap").Count(&count).Error; err != nil || count != 0 {
			return err
		}
		var tasks []Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status = ?", "awaiting_second").Order("id").Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if err := enqueueTaskSync(tx, task, task.ActorName); err != nil {
				return err
			}
		}
		return tx.Exec("INSERT INTO wb_task_sync_bootstrap (id, initialized_at) VALUES (1, ?) ON CONFLICT DO NOTHING", time.Now().UTC()).Error
	})
}

func (s *Service) RunTaskSync(ctx context.Context) error {
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
	s.taskSyncMu.Lock()
	defer s.taskSyncMu.Unlock()

	if err = repo.bootstrapTaskSync(ctx); err != nil {
		return err
	}
	db := repo.db.WithContext(ctx)
	// Earlier workers persisted only the error text. Retire that known
	// rejection even when the row is still waiting for its next retry.
	if err = db.Model(&taskSyncOutbox{}).
		Where("confirmed_at IS NULL AND blocked_at IS NULL").
		Where(oldTaskSyncSite).
		Where("last_error ~ ?", `^task sync rejected: HTTP 401, code [0-9]+$`).
		Update("blocked_at", time.Now().UTC()).Error; err != nil {
		return err
	}

	var rows []taskSyncOutbox
	if err = db.Raw(`
		SELECT c.* FROM wb_task_sync_outbox c
		WHERE c.confirmed_at IS NULL AND c.blocked_at IS NULL AND c.next_attempt_at <= CURRENT_TIMESTAMP
		  AND NOT EXISTS (
			SELECT 1 FROM wb_task_sync_outbox p
			WHERE p.task_id = c.task_id AND p.version < c.version AND p.confirmed_at IS NULL
		  )
		ORDER BY c.id LIMIT 200`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var item taskSyncItem
		if err = json.Unmarshal([]byte(row.Payload), &item); err != nil {
			return err
		}
		pending := db.Model(&taskSyncOutbox{}).Where("id = ? AND confirmed_at IS NULL AND blocked_at IS NULL", row.ID)
		err = s.sendTaskSync(ctx, row.SiteCode, []taskSyncItem{item})
		if err != nil {
			canContinue, updateErr := recordTaskSyncFailure(pending, row, err)
			if updateErr != nil || !canContinue {
				return updateErr
			}
			continue
		}
		if err = pending.Updates(map[string]any{"confirmed_at": time.Now().UTC(), "last_error": ""}).Error; err != nil {
			return err
		}
	}
	return s.sendTaskSyncHeartbeat(ctx, db)
}

func recordTaskSyncFailure(pending *gorm.DB, row taskSyncOutbox, failure error) (bool, error) {
	now := time.Now().UTC()
	updates := map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_error": failure.Error()}
	var rejection *taskSyncRejection
	if errors.As(failure, &rejection) && rejection.status == http.StatusUnauthorized {
		// Check the current code when persisting the rejection, since it could
		// have changed while the HTTP request was in flight.
		blocked := pending.Session(&gorm.Session{}).Where(oldTaskSyncSite).
			Updates(map[string]any{"attempts": updates["attempts"], "last_error": updates["last_error"], "blocked_at": now})
		if blocked.Error != nil || blocked.RowsAffected != 0 {
			return true, blocked.Error
		}
	}
	updates["next_attempt_at"] = now.Add(syncRetryBackoff(row.Attempts + 1))
	err := pending.Updates(updates).Error
	// Validation and conflicts apply to the submitted task. Connection errors,
	// unknown results and server failures end the pass to avoid repeated timeouts.
	canContinue := rejection != nil && (rejection.status == http.StatusBadRequest || rejection.status == http.StatusConflict)
	return canContinue, err
}

func (s *Service) sendTaskSyncHeartbeat(ctx context.Context, db *gorm.DB) error {
	site, err := s.LocalSiteCode(ctx)
	if err != nil {
		return err
	}
	if site == "" {
		return ErrInvalid
	}
	var remaining int64
	if err = db.Model(&taskSyncOutbox{}).Where("site_code = ? AND confirmed_at IS NULL", site).Count(&remaining).Error; err != nil || remaining != 0 {
		return err
	}
	var heartbeat struct{ LastHeartbeatAt *time.Time }
	if err = db.Table("wb_task_sync_bootstrap").Where("id=1").Take(&heartbeat).Error; err != nil {
		return err
	}
	if heartbeat.LastHeartbeatAt != nil && time.Since(*heartbeat.LastHeartbeatAt) < time.Minute {
		return nil
	}
	if err = s.sendTaskSync(ctx, site, []taskSyncItem{}); err != nil {
		return err
	}
	return db.Table("wb_task_sync_bootstrap").Where("id=1").Update("last_heartbeat_at", time.Now().UTC()).Error
}

func (s *Service) sendTaskSync(ctx context.Context, site string, items []taskSyncItem) error {
	target, err := syncV4URL(s.masterSyncAddress, "/api/v1/weighbridge/sync/tasks", nil)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		SchemaVersion int            `json:"schemaVersion"`
		SiteCode      string         `json:"siteCode"`
		Items         []taskSyncItem `json:"items"`
	}{1, site, items})
	if err != nil {
		return err
	}
	envelope, status, err := s.syncV4Client.do(ctx, http.MethodPost, target, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK || envelope.Code != http.StatusOK {
		return &taskSyncRejection{status: status, code: envelope.Code}
	}
	var receipt struct {
		SchemaVersion int       `json:"schemaVersion"`
		SiteCode      string    `json:"siteCode"`
		ReceivedAt    time.Time `json:"receivedAt"`
		Items         []struct {
			TaskNo  string `json:"taskNo"`
			Version int64  `json:"version"`
		} `json:"items"`
	}
	if json.Unmarshal(envelope.Data, &receipt) != nil || receipt.SchemaVersion != 1 || receipt.SiteCode != site || receipt.ReceivedAt.IsZero() || receipt.Items == nil || len(receipt.Items) != len(items) {
		return fmt.Errorf("invalid task sync receipt")
	}
	for i, item := range items {
		if receipt.Items[i].TaskNo != item.TaskNo || receipt.Items[i].Version < item.Version {
			return fmt.Errorf("invalid task sync item receipt")
		}
	}
	return nil
}

// RunTaskSyncWorker retries committed snapshots and reports freshness even when idle.
func (s *Service) RunTaskSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		_ = s.RunTaskSync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
