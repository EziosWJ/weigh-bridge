package weighing

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
)

func (r *Repository) PrintAttempts(ctx context.Context, id int64) ([]PrintAttempt, error) {
	if _, err := findOne[Ticket](r.db.WithContext(ctx), id); err != nil {
		return nil, err
	}
	rows := []PrintAttempt{}
	err := r.db.WithContext(ctx).Where("ticket_id=?", id).Order("attempt_no").Find(&rows).Error
	return rows, err
}

func (r *Repository) LatestPrintAttempt(ctx context.Context, ticketID int64) (*PrintAttempt, error) {
	var row PrintAttempt
	result := r.db.WithContext(ctx).Where("ticket_id=?", ticketID).Order("attempt_no DESC").Limit(1).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *Repository) PrintAttemptByCloudRequest(ctx context.Context, cloudRequestID string) (*PrintAttempt, error) {
	if strings.TrimSpace(cloudRequestID) == "" {
		return nil, nil
	}
	var row PrintAttempt
	result := r.db.WithContext(ctx).Where("cloud_request_id=?", strings.TrimSpace(cloudRequestID)).Limit(1).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func queuePrintSync(tx *gorm.DB, attemptID int64) error {
	if attemptID <= 0 {
		return nil
	}
	return tx.Exec(`INSERT INTO wb_print_sync_outbox(attempt_id, synced_at, attempts, last_error, updated_at)
		VALUES (?, NULL, 0, '', now())
		ON CONFLICT(attempt_id) DO UPDATE SET synced_at=NULL, updated_at=now()`, attemptID).Error
}

func (r *Repository) actorName(ctx context.Context, actorID int64, fallback string) string {
	if actorID <= 0 {
		return fallback
	}
	var name string
	if err := r.db.WithContext(ctx).Table("sys_user").Select("nickname").Where("id=?", actorID).Scan(&name).Error; err != nil || strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}
func (r *Repository) StartPrint(ctx context.Context, id int64, in PrintInput, e audit.Event) (CommandResult, error) {
	if strings.TrimSpace(in.Source) == "" || strings.TrimSpace(in.Source) == "factory" {
		in.ActorName = r.actorName(ctx, e.Metadata.ActorID, strings.TrimSpace(in.ActorName))
	}
	initial, err := findOne[Ticket](r.db.WithContext(ctx), id)
	if err != nil {
		return CommandResult{}, err
	}
	var ticket Ticket
	payload := struct {
		ID    int64
		Input PrintInput
	}{id, in}
	return r.command(ctx, in.RequestID, "ticket.print", payload, e, func(tx *gorm.DB) error {
		var err error
		if _, err = lockRow[Task](tx, initial.TaskID); err != nil {
			return err
		}
		ticket, err = lockRow[Ticket](tx, id)
		return err
	}, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: ticket.TaskID, TicketID: id, Status: "unknown"}}
		if ticket.Status != "valid" {
			return out, ErrConflict
		}
		var last PrintAttempt
		lookup := tx.Where("ticket_id=?", id).Order("attempt_no DESC").Limit(1).Find(&last)
		if lookup.Error != nil {
			return out, lookup.Error
		}
		source := strings.TrimSpace(in.Source)
		if source == "" {
			source = "factory"
		}
		if source != "factory" && source != "mobile" {
			return out, ErrInvalid
		}
		if lookup.RowsAffected == 0 {
			switch {
			case in.Intent == "initial":
				if source == "mobile" && in.ScanVerified && !validReason(in.Reason) {
					return out, RejectionError("请填写打印原因")
				}
			case in.Intent == "reprint" && source == "mobile" && in.ScanVerified && validReason(in.Reason):
				// A failed first print consumes one reprint when retried by scan.
			default:
				return out, ErrInvalid
			}
		} else {
			if in.Intent != "reprint" || !validReason(in.Reason) {
				return out, ErrInvalid
			}
			if source == "mobile" && !in.ScanVerified {
				return out, RejectionError("补打必须先扫码")
			}
			var reprintCount int64
			if err := tx.Model(&PrintAttempt{}).Where("ticket_id=? AND intent='reprint'", id).Count(&reprintCount).Error; err != nil {
				return out, err
			}
			if reprintCount >= 2 {
				return out, RejectionError("补打次数已达上限，最多补打2次")
			}
			if !in.AllowReprint {
				return out, ErrForbidden
			}
		}
		now := time.Now().UTC()
		out.event.Before = snapshot(last)
		if lookup.RowsAffected > 0 && last.ClosedAt == nil {
			if source == "mobile" {
				return out, RejectionError("上次打印结果未知，请先核实")
			}
			// Explicit reprint authorization closes the unknown attempt without inventing success.
			if in.Intent != "reprint" || !validReason(in.Reason) || !in.AllowReprint {
				return out, ErrForbidden
			}
			last.Status = "unknown"
			last.ClosedAt = &now
			last.CloseReason = "结果未知，操作员重新打印：" + in.Reason
			last.ResolvedAt = &now
			last.ResolvedBy = ptr(e.Metadata.ActorID)
			if err := tx.Save(&last).Error; err != nil {
				return out, err
			}
		}
		attempt := PrintAttempt{
			TicketID: id, RequestID: in.RequestID, AttemptNo: last.AttemptNo + 1,
			Intent: in.Intent, Status: "initiated", Reason: in.Reason,
			InitiatedAt: now, InitiatedBy: e.Metadata.ActorID, ResolutionSource: "operator",
			Source: source, ActorName: strings.TrimSpace(in.ActorName), ScanVerified: in.ScanVerified,
			ScanTokenHash: strings.TrimSpace(in.ScanTokenHash), CloudRequestID: strings.TrimSpace(in.CloudRequestID),
		}
		if err := tx.Create(&attempt).Error; err != nil {
			return out, err
		}
		if err := queuePrintSync(tx, attempt.ID); err != nil {
			return out, err
		}
		out.result.PrintAttemptID = attempt.ID
		out.event.After = snapshot(map[string]any{"previous": last, "attempt": attempt})
		out.event.Reason = in.Reason
		return out, nil
	})
}
func (r *Repository) ResolvePrint(ctx context.Context, id int64, in PrintResultInput, e audit.Event) (CommandResult, error) {
	if strings.TrimSpace(in.ResolutionSource) == "" || in.ResolutionSource == "operator" {
		in.ActorName = r.actorName(ctx, e.Metadata.ActorID, strings.TrimSpace(in.ActorName))
	}
	existing, err := findOne[PrintAttempt](r.db.WithContext(ctx), id)
	if err != nil {
		return CommandResult{}, err
	}
	linked, lookupErr := findOne[Ticket](r.db.WithContext(ctx), existing.TicketID)
	if lookupErr != nil {
		return CommandResult{}, lookupErr
	}
	ticketTaskID := linked.TaskID
	var ticket Ticket
	payload := struct {
		ID    int64
		Input PrintResultInput
	}{id, in}
	return r.command(ctx, in.RequestID, "ticket.print.result", payload, e, func(tx *gorm.DB) error {
		var err error
		if _, err = lockRow[Task](tx, ticketTaskID); err != nil {
			return err
		}
		ticket, err = lockRow[Ticket](tx, existing.TicketID)
		return err
	}, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: ticket.TaskID, TicketID: ticket.ID, PrintAttemptID: id, Status: in.Status}}
		attempt, err := findOne[PrintAttempt](tx, id)
		if err != nil {
			return out, err
		}
		if attempt.ClosedAt != nil {
			return out, ErrConflict
		}
		before := attempt
		now := time.Now().UTC()
		resolution := strings.TrimSpace(in.ResolutionSource)
		if resolution == "" {
			resolution = "operator"
		}
		attempt.Status = in.Status
		attempt.ResolvedAt = &now
		attempt.ResolvedBy = ptr(e.Metadata.ActorID)
		attempt.ResolutionSource = resolution
		if strings.TrimSpace(in.ActorName) != "" {
			attempt.ActorName = strings.TrimSpace(in.ActorName)
		}
		if in.Status != "unknown" {
			attempt.ClosedAt = &now
			attempt.CloseReason = in.Reason
		}
		if err := tx.Save(&attempt).Error; err != nil {
			return out, err
		}
		if err := queuePrintSync(tx, attempt.ID); err != nil {
			return out, err
		}
		out.event.Before = snapshot(before)
		out.event.After = snapshot(attempt)
		out.event.Reason = in.Reason
		return out, nil
	})
}
func fillPrintStatus(tx *gorm.DB, ticket *Ticket) error {
	var last PrintAttempt
	result := tx.Where("ticket_id=?", ticket.ID).Order("attempt_no DESC").Limit(1).Find(&last)
	if result.Error != nil {
		return result.Error
	}
	ticket.PrintStatus = "unprinted"
	if result.RowsAffected > 0 {
		ticket.PrintStatus = last.Status
		if last.Status == "initiated" {
			ticket.PrintStatus = "unknown"
		}
		ticket.LastPrintAttemptID = ptr(last.ID)
	}
	return tx.Model(&PrintAttempt{}).Where("ticket_id=? AND intent='reprint'", ticket.ID).Count(&ticket.ReprintCount).Error
}

// AgentPrintResult records only the first unresolved device result. Operator
// resolutions and closed attempts are never overwritten by a late reply.
func (r *Repository) AgentPrintResult(ctx context.Context, scaleCode string, in agentproto.PrintResult) error {
	initial, err := findOne[PrintAttempt](r.db.WithContext(ctx), in.AttemptID)
	if err != nil {
		return err
	}
	linked, err := findOne[Ticket](r.db.WithContext(ctx), initial.TicketID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRow[Task](tx, linked.TaskID); err != nil {
			return err
		}
		if _, err := lockRow[Ticket](tx, linked.ID); err != nil {
			return err
		}
		attempt, err := lockRow[PrintAttempt](tx, in.AttemptID)
		if err != nil {
			return err
		}
		if attempt.ClosedAt != nil && attempt.ResolutionSource != "driver" {
			return nil
		}
		ticket, err := findOne[Ticket](tx, attempt.TicketID)
		if err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		if identity.ScaleCode == "" || identity.ScaleCode != scaleCode {
			return ErrForbidden
		}
		before := attempt
		now := time.Now().UTC()
		attempt.Status = in.Status
		attempt.ResolutionSource = "agent"
		attempt.ResolvedAt = &now
		attempt.ClosedAt = &now
		attempt.CloseReason = in.Reason
		if err := tx.Save(&attempt).Error; err != nil {
			return err
		}
		if err := queuePrintSync(tx, attempt.ID); err != nil {
			return err
		}
		if err := tx.Create(&BusinessEvent{TaskID: ptr(ticket.TaskID), TicketID: ptr(ticket.ID), ResourceType: "weighing.print", ResourceID: ptr(attempt.ID), Action: "ticket.print.agent-result", Reason: in.Reason, Before: snapshot(before), After: snapshot(attempt), ActorID: attempt.InitiatedBy, ActorName: "Agent", OccurredAt: now}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "ticket.print.agent-result", Resource: "weighing.print", ResourceID: attempt.ID, Metadata: audit.Metadata{ActorID: attempt.InitiatedBy, RequestID: "print-" + strconv.FormatInt(attempt.ID, 10), RequestMethod: "WSS", RequestURL: "/api/agent/ws", UserAgent: "Agent"}})
	})
}

type printSyncRow struct {
	ID             int64
	TicketNo       string
	AttemptNo      int64
	Intent         string
	Status         string
	Source         string
	ActorName      string
	Reason         string
	ScanVerified   bool
	CloudRequestID string
	InitiatedAt    time.Time
	ResolvedAt     *time.Time
	Resolution     string
	CloseReason    string
}

func (r *Repository) DuePrintSyncs(ctx context.Context, limit int) ([]PrintAttemptSync, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var rows []printSyncRow
	err := r.db.WithContext(ctx).Table("wb_print_attempt p").
		Select(`p.id, k.ticket_no, p.attempt_no, p.intent, p.status, p.source, p.actor_name,
			p.reason, p.scan_verified, p.cloud_request_id, p.initiated_at, p.resolved_at,
			p.resolution_source AS resolution, p.close_reason`).
		Joins("JOIN wb_print_sync_outbox o ON o.attempt_id = p.id").
		Joins("JOIN wb_ticket k ON k.id = p.ticket_id").
		Where("o.synced_at IS NULL").
		Order("o.updated_at ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]PrintAttemptSync, 0, len(rows))
	for _, row := range rows {
		out = append(out, PrintAttemptSync{
			FactoryAttempt: row.ID, TicketNo: row.TicketNo, AttemptNo: row.AttemptNo,
			Intent: row.Intent, Status: row.Status, Source: row.Source, ActorName: row.ActorName,
			Reason: row.Reason, ScanVerified: row.ScanVerified, CloudRequestID: row.CloudRequestID,
			InitiatedAt: row.InitiatedAt, ResolvedAt: row.ResolvedAt, Resolution: row.Resolution,
			CloseReason: row.CloseReason,
		})
	}
	return out, nil
}

func (r *Repository) MarkPrintSynced(ctx context.Context, attemptID int64, failure string) error {
	values := map[string]any{"last_error": strings.TrimSpace(failure), "attempts": gorm.Expr("attempts + 1"), "updated_at": time.Now().UTC()}
	if failure == "" {
		values["synced_at"] = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Table("wb_print_sync_outbox").Where("attempt_id=?", attemptID).Updates(values).Error
}
