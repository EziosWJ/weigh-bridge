package weighing

import (
	"context"
	"database/sql"
	"errors"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("无权执行此操作")

func makeTicket(task Task, first, second WeighRecord, final *FinalWeights, canAdjust bool, actor int64, name string, previous *Ticket) (Ticket, error) {
	gross, tare := first.Weight, second.Weight
	if task.Direction == "outbound" {
		gross, tare = second.Weight, first.Weight
	}
	if first.TaskID != task.ID || second.TaskID != task.ID || first.Stage != "first" || second.Stage != "second" || first.Kind != weighingKind(task.Direction, "first") || second.Kind != weighingKind(task.Direction, "second") || gross <= 0 || tare <= 0 || gross <= tare {
		return Ticket{}, ErrWeight
	}
	values := FinalWeights{Gross: gross, Tare: tare}
	if final != nil {
		values = *final
	}
	if values.Gross <= 0 || values.Tare <= 0 || values.Gross <= values.Tare || !maxLength(values.Remark, 2000) {
		return Ticket{}, ErrWeight
	}
	changed := values.Gross != gross || values.Tare != tare
	if changed && !canAdjust {
		return Ticket{}, ErrForbidden
	}
	now := time.Now().UTC()
	ticket := Ticket{SiteID: task.SiteID, TaskID: task.ID, Revision: 1, Mode: "normal", Status: "valid", FirstRecordID: first.ID, SecondRecordID: second.ID, Business: task.Business, Basis: []WeighRecord{first, second}, SnapshotSchemaVersion: 1, OriginalGross: gross, OriginalTare: tare, FinalGross: values.Gross, FinalTare: values.Tare, GrossSource: "collected", TareSource: "collected", IssuedAt: now, IssuedBy: actor, IssuerName: name}
	if changed {
		ticket.AdjustedBy = ptr(actor)
		ticket.AdjustedAt = &now
		ticket.AdjustmentRemark = strings.TrimSpace(values.Remark)
		if ticket.AdjustmentRemark == "" {
			ticket.AdjustmentRemark = "磅精度调整"
		}
	}
	if values.Gross != gross {
		ticket.GrossSource = "manual"
	}
	if values.Tare != tare {
		ticket.TareSource = "manual"
	}
	if previous != nil {
		ticket.PreviousTicketID = ptr(previous.ID)
		ticket.Revision = previous.Revision + 1
	}
	// Validation completes before any fact or number is written.
	return ticket, nil
}
func insertTicket(ctx context.Context, tx *gorm.DB, ticket *Ticket) error {
	number, err := nextNumber(tx, "ticket", "GB")
	if err != nil {
		return err
	}
	site, err := findOne[Site](tx, ticket.SiteID)
	if err != nil {
		return err
	}
	// The title is frozen in the same transaction that creates the ticket, so
	// later configuration edits never change this ticket's view or reprint.
	title, err := sysconfig.ReadTicketTitle(ctx, tx, true)
	if err != nil {
		return err
	}
	ticket.SiteName = site.Name
	ticket.TicketTitle = title
	ticket.TicketNo = number
	return tx.Create(ticket).Error
}
func (r *Repository) ConfirmSecond(ctx context.Context, id int64, in ConfirmationInput, e audit.Event) (CommandResult, error) {
	var session ScaleSession
	var task Task
	return r.command(ctx, in.RequestID, "second.confirm", in, e, func(tx *gorm.DB) error {
		var err error
		session, err = lockSession(tx, in.ScaleID)
		if err != nil {
			return err
		}
		task, err = lockRow[Task](tx, id)
		return err
	}, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: id, TaskVersion: task.Version, Status: task.Status}}
		sample, err := validateSample(ctx, tx, in, e.Metadata.ActorID, session)
		if err != nil {
			return out, err
		}
		out.event.SampleID = ptr(sample.ID)
		if task.Version != in.Version || task.Status != "awaiting_second" || task.CurrentFirstRecordID == nil {
			return out, ErrConflict
		}
		if sample.TaskID == nil || *sample.TaskID != id || sample.Stage != "second" || sample.TargetRecordID != nil || sample.TaskVersion == nil || *sample.TaskVersion != task.Version || sample.BusinessHash != hash(task.Business) {
			return out, ErrConditions
		}
		first, err := findOne[WeighRecord](tx, *task.CurrentFirstRecordID)
		if err != nil {
			return out, err
		}
		second := newRecord(sample, task.ID, e.Metadata.ActorID, name)
		ticket, err := makeTicket(task, first, second, in.FinalWeights, in.AllowAdjust, e.Metadata.ActorID, name, nil)
		if err != nil {
			return out, err
		}
		if err := tx.Create(&second).Error; err != nil {
			return out, err
		}
		ticket.SecondRecordID = second.ID
		ticket.Basis[1] = second
		if err := insertTicket(ctx, tx, &ticket); err != nil {
			return out, err
		}
		now := time.Now().UTC()
		task.CurrentSecondRecordID = ptr(second.ID)
		task.Status = "completed"
		task.CompletedAt = &now
		task.UpdateTime = now
		task.Version++
		if err := tx.Save(&task).Error; err != nil {
			return out, err
		}
		out.result = CommandResult{TaskID: id, RecordID: second.ID, TicketID: ticket.ID, TaskVersion: task.Version, Status: task.Status}
		out.event.Before = snapshot(task.Business)
		out.event.After = snapshot(ticketView(ticket))
		return out, nil
	})
}
func (r *Repository) FindTicket(ctx context.Context, id int64) (TicketView, error) {
	var view TicketView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ticket, err := findOne[Ticket](tx, id)
		if err != nil {
			return err
		}
		if err := fillPrintStatus(tx, &ticket); err != nil {
			return err
		}
		// Historical tickets predate the site-name snapshot. Resolve their added
		// ownership label without rewriting any stored ticket or weight evidence.
		if ticket.SiteName == "" {
			site, err := findOne[Site](tx, ticket.SiteID)
			if err != nil {
				return err
			}
			ticket.SiteName = site.Name
		}
		view = ticketView(ticket)
		// ADR 0030: each ticket keeps the title frozen when it was generated.
		// Tickets created before snapshots existed fall back to the current
		// effective title until the API start freezes their upgrade baseline.
		title := ticket.TicketTitle
		if title == "" {
			title, err = sysconfig.ReadTicketTitle(ctx, tx, false)
			if err != nil {
				return err
			}
		}
		view.PrintSheet = compactPrintSheet(view, title)
		if ticket.PreviousTicketID != nil {
			previous, err := findOne[Ticket](tx, *ticket.PreviousTicketID)
			if err != nil {
				return err
			}
			view.PreviousFinal = &PriorFinalWeights{Gross: previous.FinalGross, Tare: previous.FinalTare, Net: previous.FinalGross - previous.FinalTare}
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return view, err
}

// FreezeLegacyTicketTitles writes the upgrade-time effective title onto tickets
// that predate title snapshots. It is safe to call on every start: only rows
// still carrying the empty marker are changed.
func FreezeLegacyTicketTitles(ctx context.Context, db *gorm.DB, title string) error {
	if !db.Migrator().HasTable("wb_ticket") {
		return nil
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = sysconfig.DefaultTicketTitle
	}
	return db.WithContext(ctx).Exec("UPDATE wb_ticket SET ticket_title=? WHERE ticket_title=''", title).Error
}

func (r *Repository) Tasks(ctx context.Context, q PageQuery) (Page[Task], error) {
	query, err := taskQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return Page[Task]{}, err
	}
	return workflowPage[Task](query, q, "t.create_time DESC,t.id DESC")
}
func (r *Repository) Tickets(ctx context.Context, q PageQuery) (Page[TicketView], error) {
	query, err := ticketQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return Page[TicketView]{}, err
	}
	query = query.Select("k.*, COALESCE((SELECT CASE WHEN p.status='initiated' THEN 'unknown' ELSE p.status END FROM wb_print_attempt p WHERE p.ticket_id=k.id ORDER BY p.attempt_no DESC LIMIT 1),'unprinted') AS print_status, (SELECT COUNT(*) FROM wb_print_attempt p WHERE p.ticket_id=k.id AND p.intent='reprint') AS reprint_count, (SELECT p.id FROM wb_print_attempt p WHERE p.ticket_id=k.id ORDER BY p.attempt_no DESC LIMIT 1) AS last_print_attempt_id")
	raw, err := workflowPage[Ticket](query, q, "k.issued_at DESC,k.id DESC")
	result := Page[TicketView]{Records: []TicketView{}, Total: raw.Total, Page: raw.Page, PageSize: raw.PageSize}
	for _, row := range raw.Records {
		result.Records = append(result.Records, ticketView(row))
	}
	return result, err
}
