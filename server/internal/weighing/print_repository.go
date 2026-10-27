package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"strconv"
	"time"

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
func (r *Repository) StartPrint(ctx context.Context, id int64, in PrintInput, e audit.Event) (CommandResult, error) {
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
		if lookup.RowsAffected == 0 {
			if in.Intent != "initial" {
				return out, ErrInvalid
			}
		} else {
			if in.Intent != "reprint" || !validReason(in.Reason) {
				return out, ErrInvalid
			}
			if !in.AllowReprint {
				return out, ErrForbidden
			}
		}
		now := time.Now().UTC()
		out.event.Before = snapshot(last)
		if lookup.RowsAffected > 0 && last.ClosedAt == nil {
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
		attempt := PrintAttempt{TicketID: id, RequestID: in.RequestID, AttemptNo: last.AttemptNo + 1, Intent: in.Intent, Status: "initiated", Reason: in.Reason, InitiatedAt: now, InitiatedBy: e.Metadata.ActorID, ResolutionSource: "operator"}
		if err := tx.Create(&attempt).Error; err != nil {
			return out, err
		}
		out.result.PrintAttemptID = attempt.ID
		out.event.After = snapshot(map[string]any{"previous": last, "attempt": attempt})
		out.event.Reason = in.Reason
		return out, nil
	})
}
func (r *Repository) ResolvePrint(ctx context.Context, id int64, in PrintResultInput, e audit.Event) (CommandResult, error) {
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
		attempt.Status = in.Status
		attempt.ResolvedAt = &now
		attempt.ResolvedBy = ptr(e.Metadata.ActorID)
		if in.Status != "unknown" {
			attempt.ClosedAt = &now
			attempt.CloseReason = in.Reason
		}
		if err := tx.Save(&attempt).Error; err != nil {
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
		if attempt.ClosedAt != nil {
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
		if err := tx.Create(&BusinessEvent{TaskID: ptr(ticket.TaskID), TicketID: ptr(ticket.ID), ResourceType: "weighing.print", ResourceID: ptr(attempt.ID), Action: "ticket.print.agent-result", Reason: in.Reason, Before: snapshot(before), After: snapshot(attempt), ActorID: attempt.InitiatedBy, ActorName: "Agent", OccurredAt: now}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "ticket.print.agent-result", Resource: "weighing.print", ResourceID: attempt.ID, Metadata: audit.Metadata{ActorID: attempt.InitiatedBy, RequestID: "print-" + strconv.FormatInt(attempt.ID, 10), RequestMethod: "WSS", RequestURL: "/api/agent/ws", UserAgent: "Agent"}})
	})
}
