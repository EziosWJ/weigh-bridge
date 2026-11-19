package weighing

import (
	"context"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
)

type ManualSupplementInput struct {
	RequestID       string              `json:"requestId"`
	Business        BusinessInput       `json:"business"`
	FirstWeight     Tonnes              `json:"firstWeight"`
	SecondWeight    Tonnes              `json:"secondWeight"`
	FirstWeighedAt  time.Time           `json:"firstWeighedAt"`
	SecondWeighedAt time.Time           `json:"secondWeighedAt"`
	Reason          string              `json:"reason"`
	NetAdjustments  NetAdjustmentsInput `json:"netAdjustments"`
	AllowAdjust     bool                `json:"-"`
}

func (s *Service) ManualSupplement(ctx context.Context, in ManualSupplementInput, e audit.Event) (CommandResult, error) {
	if err := s.masterSyncReady(ctx); err != nil {
		return CommandResult{}, err
	}
	if !validRequestID(in.RequestID) || (in.Reason != "power_failure" && in.Reason != "equipment_failure" && in.Reason != "system_failure") || in.FirstWeight <= 0 || in.SecondWeight <= 0 || in.FirstWeighedAt.IsZero() || in.SecondWeighedAt.IsZero() || in.FirstWeighedAt.After(in.SecondWeighedAt) || in.SecondWeighedAt.After(time.Now()) {
		return CommandResult{}, ErrInvalid
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return CommandResult{}, ErrForbidden
	}
	return repo.manualSupplement(ctx, in, e)
}

func (r *Repository) manualSupplement(ctx context.Context, in ManualSupplementInput, e audit.Event) (CommandResult, error) {
	return r.command(ctx, in.RequestID, "ticket.manual-supplement", in, e, func(tx *gorm.DB) error {
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		if identity.SiteCode == "" || identity.ScaleCode == "" || !mayOperate(ctx, identity.SiteCode) {
			return ErrForbidden
		}
		return nil
	}, func(tx *gorm.DB, name string) (commandOutcome, error) {
		out := commandOutcome{}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return out, err
		}
		business, err := resolveBusiness(tx, in.Business, nil)
		if err != nil {
			return out, err
		}
		var active int64
		if err = tx.Model(&Task{}).Where("plate=? AND status='awaiting_second'", business.Plate).Count(&active).Error; err != nil {
			return out, err
		}
		if active != 0 {
			return out, ErrConflict
		}
		if err = resolveItemsCategoryNames(tx, in.NetAdjustments); err != nil {
			return out, err
		}
		now := time.Now().UTC()
		task := Task{ID: 1, SiteCode: identity.SiteCode, Mode: "normal", Plate: business.Plate, Direction: business.Direction, Status: "completed", ScaleCode: identity.ScaleCode, MaterialID: business.MaterialID, PartyID: business.PartyID, CarrierID: business.CarrierID, Business: business, Version: 1, CreateTime: now, UpdateTime: now, ActorID: e.Metadata.ActorID, ActorName: name, CompletedAt: &now}
		first := WeighRecord{TaskID: 1, Source: "manual", Stage: "first", Kind: weighingKind(task.Direction, "first"), Weight: in.FirstWeight, CapturedAt: in.FirstWeighedAt, ConfirmedAt: now, ScaleCode: identity.ScaleCode, ActorID: e.Metadata.ActorID, ActorName: name, Reason: in.Reason, Evidence: snapshot(map[string]any{"source": "manual_supplement", "reason": in.Reason, "enteredAt": now}), PlateEvidence: snapshot(map[string]any{})}
		second := WeighRecord{TaskID: 1, Source: "manual", Stage: "second", Kind: weighingKind(task.Direction, "second"), Weight: in.SecondWeight, CapturedAt: in.SecondWeighedAt, ConfirmedAt: now, ScaleCode: identity.ScaleCode, ActorID: e.Metadata.ActorID, ActorName: name, Reason: in.Reason, Evidence: snapshot(map[string]any{"source": "manual_supplement", "reason": in.Reason, "enteredAt": now}), PlateEvidence: snapshot(map[string]any{})}
		if _, err = makeTicketWithItems(task, first, second, nil, in.NetAdjustments, "", in.AllowAdjust, e.Metadata.ActorID, name, nil); err != nil {
			return out, err
		}
		number, err := nextNumber(tx, "task", "RW", r.dataDir, now)
		if err != nil {
			return out, err
		}
		task.ID = 0
		task.TaskNo = number
		if err = tx.Create(&task).Error; err != nil {
			return out, err
		}
		first.TaskID = task.ID
		second.TaskID = task.ID
		if err = tx.Create(&first).Error; err != nil {
			return out, err
		}
		if err = tx.Create(&second).Error; err != nil {
			return out, err
		}
		task.CurrentFirstRecordID = ptr(first.ID)
		task.CurrentSecondRecordID = ptr(second.ID)
		if err = tx.Save(&task).Error; err != nil {
			return out, err
		}
		ticket, err := makeTicketWithItems(task, first, second, nil, in.NetAdjustments, "", in.AllowAdjust, e.Metadata.ActorID, name, nil)
		if err != nil {
			return out, err
		}
		ticket.EntryType = "manual_supplement"
		ticket.SupplementReason = in.Reason
		if err = insertTicket(ctx, tx, &ticket, r.dataDir); err != nil {
			return out, err
		}
		if err = enqueueTicketChange(tx, ticket, nil, "issue", name); err != nil {
			return out, err
		}
		out.result = CommandResult{TaskID: task.ID, TicketID: ticket.ID, TaskVersion: task.Version, Status: task.Status}
		out.event.Reason = in.Reason
		out.event.After = snapshot(map[string]any{"ticket": ticketView(ticket), "records": []WeighRecord{first, second}})
		return out, nil
	})
}
