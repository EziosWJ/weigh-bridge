package weighing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/tripsync"

	"gorm.io/gorm"
)

func tripBindingByTask(tx *gorm.DB, taskID int64) (*TripBinding, error) {
	var binding TripBinding
	err := tx.Where("task_id=?", taskID).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func (r *Repository) TripBinding(ctx context.Context, taskID int64) (*TripBinding, error) {
	return tripBindingByTask(r.db.WithContext(ctx), taskID)
}

func (r *Repository) TaskTripBasis(ctx context.Context, taskID int64) (TaskTripBasis, error) {
	binding, err := tripBindingByTask(r.db.WithContext(ctx), taskID)
	if err != nil || binding == nil {
		return TaskTripBasis{Deductions: []tripsync.Deduction{}}, err
	}
	result := TaskTripBasis{
		Bound: true, Status: binding.Status, TripNo: binding.TripNo,
		TicketID: binding.TicketID, CloudTicketNo: binding.CloudTicketNo,
		Deductions: []tripsync.Deduction{},
	}
	var trip tripsync.Trip
	if binding.TripSnapshot != "" && binding.TripSnapshot != "null" && json.Unmarshal([]byte(binding.TripSnapshot), &trip) == nil {
		result.Trip = &trip
	}
	var inspection tripsync.Inspection
	if binding.InspectionSnapshot != "" && binding.InspectionSnapshot != "null" && json.Unmarshal([]byte(binding.InspectionSnapshot), &inspection) == nil {
		result.Inspection = &inspection
	}
	result.Deductions = []tripsync.Deduction{}
	if binding.DeductionSnapshot != "" && binding.DeductionSnapshot != "null" && json.Unmarshal([]byte(binding.DeductionSnapshot), &result.Deductions) != nil {
		result.Deductions = []tripsync.Deduction{}
	}
	return result, nil
}

func (r *Repository) bindTripTicket(tx *gorm.DB, taskID, ticketID int64) error {
	now := time.Now().UTC()
	return tx.Model(&TripBinding{}).Where("task_id=?", taskID).Updates(map[string]any{
		"ticket_id": ticketID, "status": "ticketed", "ticketed_at": now, "updated_at": now,
		"cloud_ticket_no": "", "linked_at": nil,
	}).Error
}

func (r *Repository) markTripBindingLinked(ctx context.Context, id int64, ticketNo string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&TripBinding{}).Where("id=?", id).Updates(map[string]any{
		"status": "linked", "cloud_ticket_no": ticketNo, "linked_at": now, "updated_at": now,
	}).Error
}

func (r *Repository) markTripBindingConflict(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&TripBinding{}).Where("id=?", id).Updates(map[string]any{
		"status": "conflict", "updated_at": time.Now().UTC(),
	}).Error
}

type pendingTripLink struct {
	TripBinding
	IssuedTicketNo string `gorm:"column:issued_ticket_no"`
}

func (r *Repository) pendingTripLinks(ctx context.Context, limit int) ([]pendingTripLink, error) {
	rows := []pendingTripLink{}
	err := r.db.WithContext(ctx).Table("wb_trip_binding b").
		Select("b.*, k.ticket_no AS issued_ticket_no").
		Joins("JOIN wb_ticket k ON k.id=b.ticket_id").
		Where("b.status='ticketed' AND b.ticket_id IS NOT NULL AND k.status='valid'").
		Order("b.id").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}
