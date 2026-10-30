package weighing

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("无权执行此操作")

const deductionCategoryDictCode = "WEIGHING_DEDUCTION_CATEGORY"

// resolveNetAdjustmentCategory validates the operator-facing reason against the
// active dictionary and returns the label snapshot stored on the ticket.
func resolveNetAdjustmentCategory(tx *gorm.DB, in *NetAdjustmentInput) (string, error) {
	if in == nil || !in.Requested() {
		return "", nil
	}
	code := strings.TrimSpace(in.Category)
	if code == "" || len(code) > 40 {
		return "", ErrInvalid
	}
	var label string
	row := tx.Table("sys_dict_data AS d").
		Select("d.dict_label").
		Joins("JOIN sys_dict_type AS t ON t.id=d.dict_type_id").
		Where("t.dict_code=? AND t.status=1 AND t.deleted=0 AND d.deleted=0 AND d.dict_value=?", deductionCategoryDictCode, code).
		Limit(1).Row()
	if err := row.Scan(&label); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalid
		}
		return "", err
	}
	if strings.TrimSpace(label) == "" {
		return "", ErrInvalid
	}
	return label, nil
}

// resolveNetAdjustment turns the operator input into a signed absolute amount.
// Percentage mode uses the adjusted net basis and rounds to 0.001 吨, half away
// from zero. Negative amounts are accepted only when reopening a void ticket.
func resolveNetAdjustment(in *NetAdjustmentInput, base Tonnes, allowUpward bool) (WeightDifference, string, *int, error) {
	if in == nil {
		return 0, "amount", nil, nil
	}
	mode := in.Mode
	if mode == "" {
		mode = "amount"
	}
	switch mode {
	case "amount":
		if in.Amount == nil || in.RateBP != nil {
			return 0, "", nil, ErrInvalid
		}
		amount := *in.Amount
		if amount < 0 && !allowUpward {
			return 0, "", nil, ErrInvalid
		}
		if amount == 0 {
			return 0, "amount", nil, nil
		}
		return amount, "amount", nil, nil
	case "percent":
		if in.RateBP == nil {
			return 0, "", nil, ErrInvalid
		}
		rate := *in.RateBP
		if rate < -10000 || rate > 10000 || (rate < 0 && !allowUpward) {
			return 0, "", nil, ErrInvalid
		}
		if rate == 0 {
			return 0, "amount", nil, nil
		}
		if int64(base) > math.MaxInt64/10000 {
			return 0, "", nil, ErrWeight
		}
		amount := WeightDifference(roundRatio(int64(base)*int64(rate), 10000))
		if amount == 0 || (amount < 0 && !allowUpward) {
			return 0, "", nil, ErrInvalid
		}
		return amount, "percent", &rate, nil
	default:
		return 0, "", nil, ErrInvalid
	}
}

func roundRatio(numerator, denominator int64) int64 {
	if numerator >= 0 {
		return (numerator + denominator/2) / denominator
	}
	return -((-numerator + denominator/2) / denominator)
}

func makeTicket(task Task, first, second WeighRecord, final *FinalWeights, adjustment *NetAdjustmentInput, categoryName string, allowUpward, canAdjust bool, actor int64, name string, previous *Ticket) (Ticket, error) {
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
	if adjustment != nil && !maxLength(adjustment.Remark, 2000) {
		return Ticket{}, ErrWeight
	}
	amount, mode, rateBP, err := resolveNetAdjustment(adjustment, values.Gross-values.Tare, allowUpward)
	if err != nil {
		return Ticket{}, err
	}
	if amount != 0 && strings.TrimSpace(categoryName) == "" {
		return Ticket{}, ErrInvalid
	}
	if amount == 0 {
		mode, rateBP, categoryName = "amount", nil, ""
	}
	net := finalNet(values.Gross, values.Tare, amount)
	if net <= 0 || net >= values.Gross {
		return Ticket{}, ErrWeight
	}
	changed := values.Gross != gross || values.Tare != tare || amount != 0
	if changed && !canAdjust {
		return Ticket{}, ErrForbidden
	}
	remark := ""
	if adjustment != nil {
		remark = strings.TrimSpace(adjustment.Remark)
	}
	if remark == "" && final != nil {
		remark = strings.TrimSpace(final.Remark)
	}
	now := time.Now().UTC()
	ticket := Ticket{SiteCode: task.SiteCode, TaskID: task.ID, Revision: 1, Mode: "normal", Status: "valid", FirstRecordID: first.ID, SecondRecordID: second.ID, Business: task.Business, Basis: []WeighRecord{first, second}, SnapshotSchemaVersion: 1, OriginalGross: gross, OriginalTare: tare, FinalGross: values.Gross, FinalTare: values.Tare, GrossSource: "collected", TareSource: "collected", NetAdjustment: amount, NetAdjustmentMode: mode, NetAdjustmentRateBP: rateBP, IssuedAt: now, IssuedBy: actor, IssuerName: name}
	if amount != 0 {
		ticket.NetAdjustmentCategory = strings.TrimSpace(adjustment.Category)
		ticket.NetAdjustmentCategoryName = strings.TrimSpace(categoryName)
	}
	if changed {
		ticket.AdjustedBy = ptr(actor)
		ticket.AdjustedAt = &now
		ticket.AdjustmentRemark = remark
		if ticket.AdjustmentRemark == "" && amount == 0 {
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

func insertTicket(ctx context.Context, tx *gorm.DB, ticket *Ticket, dataDir string) error {
	identity, err := localsite.Read(ctx, tx)
	if err != nil {
		return err
	}
	if identity.SiteCode == "" {
		return localsite.ErrNotConfigured
	}
	// The task retains its original identity snapshot; a new ticket is issued
	// under the identity effective at the time of issue.
	ticket.SiteCode = identity.SiteCode
	number, err := nextNumber(tx, "ticket", "GB", dataDir, ticket.IssuedAt)
	if err != nil {
		return err
	}
	var site Site
	if err := tx.Where("site_code=?", ticket.SiteCode).Take(&site).Error; err != nil {
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
		session, err = lockSession(tx, in.ScaleCode)
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
		categoryName, err := resolveNetAdjustmentCategory(tx, in.NetAdjustment)
		if err != nil {
			return out, err
		}
		ticket, err := makeTicket(task, first, second, nil, in.NetAdjustment, categoryName, false, in.AllowAdjust, e.Metadata.ActorID, name, nil)
		if err != nil {
			return out, err
		}
		if err := tx.Create(&second).Error; err != nil {
			return out, err
		}
		ticket.SecondRecordID = second.ID
		ticket.Basis[1] = second
		if err := insertTicket(ctx, tx, &ticket, r.dataDir); err != nil {
			return out, err
		}
		if err := enqueueTicketChange(tx, ticket, nil, "issue", name); err != nil {
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
			var site Site
			identity, err := localsite.Read(ctx, tx)
			if err != nil {
				return err
			}
			if err := tx.Where("site_code=?", identity.SiteCode).Take(&site).Error; err != nil {
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
			view.PreviousFinal = &PriorFinalWeights{Gross: previous.FinalGross, Tare: previous.FinalTare, Net: finalNet(previous.FinalGross, previous.FinalTare, previous.NetAdjustment), NetAdjustment: previous.NetAdjustment, CategoryName: previous.NetAdjustmentCategoryName}
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
	query = query.Select("k.*, COALESCE((SELECT c.status FROM wb_ticket_sync_change c WHERE c.ticket_id=k.id ORDER BY c.id DESC LIMIT 1),'untracked') AS sync_status, COALESCE((SELECT CASE WHEN p.status='initiated' THEN 'unknown' ELSE p.status END FROM wb_print_attempt p WHERE p.ticket_id=k.id ORDER BY p.attempt_no DESC LIMIT 1),'unprinted') AS print_status, (SELECT COUNT(*) FROM wb_print_attempt p WHERE p.ticket_id=k.id AND p.intent='reprint') AS reprint_count, (SELECT p.id FROM wb_print_attempt p WHERE p.ticket_id=k.id ORDER BY p.attempt_no DESC LIMIT 1) AS last_print_attempt_id")
	raw, err := workflowPage[Ticket](query, q, "k.issued_at DESC,k.id DESC")
	result := Page[TicketView]{Records: []TicketView{}, Total: raw.Total, Page: raw.Page, PageSize: raw.PageSize}
	for _, row := range raw.Records {
		result.Records = append(result.Records, ticketView(row))
	}
	return result, err
}
