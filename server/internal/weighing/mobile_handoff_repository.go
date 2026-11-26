package weighing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) MobileHandoff(ctx context.Context, handoffID int64, action string) (*MobileHandoff, error) {
	var row MobileHandoff
	err := r.db.WithContext(ctx).Where("handoff_id = ? AND action = ?", handoffID, action).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) SaveMobileHandoff(ctx context.Context, row *MobileHandoff) error {
	now := time.Now().UTC()
	row.UpdatedAt = now
	if row.Status == MobileHandoffCompleted {
		row.CompletedAt = &now
	}
	if row.ID == 0 {
		row.ClaimedAt = now
		row.CreatedAt = now
		return r.db.WithContext(ctx).Create(row).Error
	}
	return r.db.WithContext(ctx).Model(&MobileHandoff{}).Where("id = ?", row.ID).Updates(map[string]any{
		"status": row.Status, "source": row.Source, "device": row.Device,
		"task_id": row.TaskID, "ticket_id": row.TicketID,
		"first_record_id": row.FirstRecordID, "second_record_id": row.SecondRecordID,
		"result": row.Result, "error": row.Error, "completed_at": row.CompletedAt, "updated_at": now,
	}).Error
}

func (r *Repository) MobileHandoffs(ctx context.Context, page, pageSize int) (Page[MobileHandoff], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&MobileHandoff{}).Count(&total).Error; err != nil {
		return Page[MobileHandoff]{}, err
	}
	rows := make([]MobileHandoff, 0)
	err := r.db.WithContext(ctx).Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	if err != nil {
		return Page[MobileHandoff]{}, err
	}
	return Page[MobileHandoff]{Records: rows, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *Repository) MaterialIDByCloudCode(ctx context.Context, code string) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Table("wb_master_material_map m").
		Select("m.local_id").Joins("JOIN wb_material x ON x.id = m.local_id").
		Where("m.cloud_code = ? AND x.enabled = 1", code).Scan(&id).Error
	if err == nil && id == 0 {
		err = ErrNotFound
	}
	return id, err
}

func (r *Repository) PartyIDByCloudCode(ctx context.Context, code string) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Table("wb_master_party_map m").
		Select("m.local_id").Joins("JOIN wb_party x ON x.id = m.local_id").
		Where("m.cloud_code = ? AND x.enabled = 1", code).Scan(&id).Error
	if err == nil && id == 0 {
		err = ErrNotFound
	}
	return id, err
}

func (r *Repository) TaskByNo(ctx context.Context, taskNo string) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).Where("task_no = ?", taskNo).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}

func (r *Repository) TicketByTask(ctx context.Context, taskID int64) (*TicketView, error) {
	var ticket Ticket
	err := r.db.WithContext(ctx).Where("task_id = ? AND status = 'valid'", taskID).Order("id DESC").Take(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view := ticketView(ticket)
	return &view, nil
}
