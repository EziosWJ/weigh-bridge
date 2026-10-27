package weighing

import (
	"context"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func activePlateConflict(tx *gorm.DB, plate string, exclude int64, _ string) error {
	var count int64
	if err := tx.Model(&Task{}).Where("plate=? AND id<>? AND status='awaiting_second'", plate, exclude).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConflict
	}
	return nil
}
func (r *Repository) CorrectTask(ctx context.Context, id int64, in CorrectionInput, e audit.Event) (CommandResult, error) {
	var task Task
	payload := struct {
		ID    int64
		Input CorrectionInput
	}{id, in}
	return r.command(ctx, in.RequestID, "task.correct", payload, e, func(tx *gorm.DB) error { var err error; task, err = lockRow[Task](tx, id); return err }, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: id, TaskVersion: task.Version, Status: task.Status}}
		if task.Status != "awaiting_second" || task.Version != in.Version || in.Business.Direction != task.Direction {
			return out, ErrConflict
		}
		business, err := resolveBusiness(tx, in.Business, &task.Business)
		if err != nil {
			return out, err
		}
		if err := activePlateConflict(tx, business.Plate, id, task.SiteCode); err != nil {
			return out, err
		}
		out.event.Before = snapshot(task.Business)
		out.event.After = snapshot(business)
		out.event.Reason = in.Reason
		task.Business = business
		task.Plate = business.Plate
		task.MaterialID = business.MaterialID
		task.PartyID = business.PartyID
		task.CarrierID = business.CarrierID
		task.Version++
		task.UpdateTime = time.Now().UTC()
		if err := tx.Save(&task).Error; err != nil {
			return out, err
		}
		out.result.TaskVersion = task.Version
		return out, nil
	})
}
func (r *Repository) TerminateTask(ctx context.Context, id int64, in TaskActionInput, e audit.Event) (CommandResult, error) {
	var task Task
	payload := struct {
		ID    int64
		Input TaskActionInput
	}{id, in}
	return r.command(ctx, in.RequestID, "task.terminate", payload, e, func(tx *gorm.DB) error { var err error; task, err = lockRow[Task](tx, id); return err }, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: id, TaskVersion: task.Version, Status: task.Status}}
		if task.Status != "awaiting_second" || task.Version != in.Version {
			return out, ErrConflict
		}
		out.event.Before = snapshot(task.Business)
		out.event.Reason = in.Reason
		now := time.Now().UTC()
		task.Status = "terminated"
		task.TerminatedAt = &now
		task.TerminateReason = in.Reason
		task.UpdateTime = now
		task.Version++
		if err := tx.Save(&task).Error; err != nil {
			return out, err
		}
		out.event.After = snapshot(task.Business)
		out.result.TaskVersion = task.Version
		out.result.Status = task.Status
		return out, nil
	})
}
func (r *Repository) VoidTicket(ctx context.Context, id int64, in VoidInput, e audit.Event) (CommandResult, error) {
	existing, err := findOne[Ticket](r.db.WithContext(ctx), id)
	if err != nil {
		return CommandResult{}, err
	}
	var task Task
	payload := struct {
		ID    int64
		Input VoidInput
	}{id, in}
	return r.command(ctx, in.RequestID, "ticket.void", payload, e, func(tx *gorm.DB) error { var err error; task, err = lockRow[Task](tx, existing.TaskID); return err }, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: task.ID, TicketID: id, TaskVersion: task.Version, Status: task.Status}}
		ticket, err := lockRow[Ticket](tx, id)
		if err != nil {
			return out, err
		}
		if ticket.Status != "valid" || task.Status != "completed" {
			return out, ErrConflict
		}
		before := ticket
		now := time.Now().UTC()
		ticket.Status = "void"
		ticket.VoidedAt = &now
		ticket.VoidedBy = ptr(e.Metadata.ActorID)
		ticket.VoidReason = in.Reason
		if err := tx.Model(&Ticket{}).Where("id=? AND status='valid'", id).Updates(map[string]any{"status": "void", "voided_at": now, "voided_by": e.Metadata.ActorID, "void_reason": in.Reason}).Error; err != nil {
			return out, err
		}
		task.Version++
		task.UpdateTime = now
		if err := tx.Model(&task).Updates(map[string]any{"version": task.Version, "update_time": now}).Error; err != nil {
			return out, err
		}
		out.event.Before = snapshot(ticketView(before))
		out.event.After = snapshot(ticketView(ticket))
		out.event.Reason = in.Reason
		out.result.TaskVersion = task.Version
		return out, nil
	})
}
func (r *Repository) Reweigh(ctx context.Context, id int64, in ReweighInput, e audit.Event) (CommandResult, error) {
	var session ScaleSession
	var task Task
	payload := struct {
		ID    int64
		Input ReweighInput
	}{id, in}
	return r.command(ctx, in.RequestID, "task.reweigh", payload, e, func(tx *gorm.DB) error {
		var err error
		session, err = lockSession(tx, in.ScaleCode)
		if err != nil {
			return err
		}
		task, err = lockRow[Task](tx, id)
		return err
	}, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: id, TaskVersion: task.Version, Status: task.Status}}
		if task.Version != in.Version || task.Status == "terminated" || in.TargetRecordID == nil || !in.SameLoadConfirmed {
			return out, ErrConflict
		}
		if task.Status == "completed" {
			var active int64
			if err := tx.Model(&Ticket{}).Where("task_id=? AND status='valid'", id).Count(&active).Error; err != nil {
				return out, err
			}
			if active > 0 {
				return out, ErrConflict
			}
			if err := activePlateConflict(tx, task.Plate, id, task.SiteCode); err != nil {
				return out, err
			}
		}
		sample, err := validateSample(ctx, tx, in.ConfirmationInput, e.Metadata.ActorID, session)
		if err != nil {
			return out, err
		}
		if sample.TaskID == nil || *sample.TaskID != id || sample.TargetRecordID == nil || *sample.TargetRecordID != *in.TargetRecordID || sample.TaskVersion == nil || *sample.TaskVersion != task.Version || sample.BusinessHash != hash(task.Business) {
			return out, ErrConditions
		}
		target, err := findOne[WeighRecord](tx, *in.TargetRecordID)
		if err != nil {
			return out, err
		}
		if target.TaskID != id || sample.Stage != target.Stage || sample.Kind != target.Kind {
			return out, ErrInvalid
		}
		current, retained := task.CurrentFirstRecordID, task.CurrentSecondRecordID
		if target.Stage == "second" {
			current, retained = task.CurrentSecondRecordID, task.CurrentFirstRecordID
		}
		if current == nil || *current != target.ID {
			return out, ErrConflict
		}
		var other *WeighRecord
		if retained != nil {
			if !in.OtherBasisValid {
				return out, ErrInvalid
			}
			record, err := findOne[WeighRecord](tx, *retained)
			if err != nil {
				return out, err
			}
			other = &record
			gross, tare := sample.Weight, record.Weight
			if sample.Kind == "tare" {
				gross, tare = record.Weight, sample.Weight
			}
			if gross <= tare {
				return out, ErrWeight
			}
		}
		record := newRecord(sample, id, e.Metadata.ActorID, name)
		record.ReplacesRecordID = ptr(target.ID)
		record.RetainedRecordID = retained
		record.Reason = in.Reason
		record.SameLoadConfirmed = 1
		record.OtherBasisConfirmed = flag(in.OtherBasisValid)
		if err := tx.Create(&record).Error; err != nil {
			return out, err
		}
		if target.Stage == "first" {
			task.CurrentFirstRecordID = ptr(record.ID)
		} else {
			task.CurrentSecondRecordID = ptr(record.ID)
		}
		task.Version++
		task.UpdateTime = time.Now().UTC()
		if err := tx.Save(&task).Error; err != nil {
			return out, err
		}
		out.result.RecordID = record.ID
		out.result.TaskVersion = task.Version
		out.event.SampleID = ptr(sample.ID)
		out.event.Before = snapshot(target)
		out.event.After = snapshot(map[string]any{"replacement": record, "retained": other})
		out.event.Reason = in.Reason
		return out, nil
	})
}
func (r *Repository) ReopenTicket(ctx context.Context, id int64, in ReopenInput, e audit.Event) (CommandResult, error) {
	previous, err := findOne[Ticket](r.db.WithContext(ctx), id)
	if err != nil {
		return CommandResult{}, err
	}
	var task Task
	payload := struct {
		ID    int64
		Input ReopenInput
	}{id, in}
	return r.command(ctx, in.RequestID, "ticket.reopen", payload, e, func(tx *gorm.DB) error { var err error; task, err = lockRow[Task](tx, previous.TaskID); return err }, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{result: CommandResult{TaskID: task.ID, TaskVersion: task.Version, Status: task.Status}}
		if task.Status != "completed" || task.Version != in.TaskVersion || !in.BasisValid || task.CurrentFirstRecordID == nil || task.CurrentSecondRecordID == nil || in.Business.Direction != task.Direction {
			return out, ErrConflict
		}
		var latest Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id=?", task.ID).Order("revision DESC").Take(&latest).Error; err != nil {
			return out, err
		}
		if latest.ID != id || latest.Status != "void" {
			return out, ErrConflict
		}
		input := in.Business
		input.Plate = normalizePlate(input.Plate)
		business := task.Business
		if hash(input) != hash(task.Business.BusinessInput) {
			if !in.AllowCorrect {
				return out, ErrForbidden
			}
			business, err = resolveBusiness(tx, input, &task.Business)
			if err != nil {
				return out, err
			}
		}
		if business.Plate != task.Plate {
			if err := activePlateConflict(tx, business.Plate, task.ID, task.SiteCode); err != nil {
				return out, err
			}
		}
		first, err := findOne[WeighRecord](tx, *task.CurrentFirstRecordID)
		if err != nil {
			return out, err
		}
		second, err := findOne[WeighRecord](tx, *task.CurrentSecondRecordID)
		if err != nil {
			return out, err
		}
		before := task.Business
		task.Business = business
		task.Plate = business.Plate
		task.MaterialID = business.MaterialID
		task.PartyID = business.PartyID
		task.CarrierID = business.CarrierID
		ticket, err := makeTicket(task, first, second, in.FinalWeights, in.AllowAdjust, e.Metadata.ActorID, name, &latest)
		if err != nil {
			return out, err
		}
		if err := insertTicket(ctx, tx, &ticket, r.dataDir); err != nil {
			return out, err
		}
		task.Version++
		task.UpdateTime = time.Now().UTC()
		if err := tx.Save(&task).Error; err != nil {
			return out, err
		}
		out.result.TicketID = ticket.ID
		out.result.TaskVersion = task.Version
		out.event.Before = snapshot(before)
		out.event.After = snapshot(business)
		out.event.Reason = in.Reason
		return out, nil
	})
}
