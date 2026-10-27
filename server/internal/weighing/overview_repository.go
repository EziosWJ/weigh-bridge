package weighing

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"time"
)

// Overview reads all business sections from the same local, permission-scoped
// snapshot. Today's figures use ticket issue time; pending tasks span all dates.
func (r *Repository) Overview(ctx context.Context, now time.Time) (Overview, error) {
	date := now.In(beijing).Format("2006-01-02")
	result := Overview{Date: date, CheckedAt: now, AwaitingTasks: []OverviewTask{}, RecentTickets: []TicketView{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx, sessionTTL: r.sessionTTL}
		summary, err := repo.Summary(ctx, "tickets", PageQuery{From: date, To: date})
		if err != nil {
			return err
		}
		result.ValidTickets = summary.TaskCount
		for _, row := range summary.Rows {
			var target *OverviewDirection
			if row.Direction == "inbound" {
				target = &result.Inbound
			}
			if row.Direction == "outbound" {
				target = &result.Outbound
			}
			if target != nil {
				target.TaskCount += row.TaskCount
				target.Net += row.Net
			}
		}
		pending := func() *gorm.DB {
			return siteFilter(tx.Table("wb_transport_task AS t"), PageQuery{}, "t.site_code").
				Joins("JOIN wb_weigh_record f ON f.id=t.current_first_record_id AND f.task_id=t.id").
				Where("t.status=?", "awaiting_second")
		}
		if err := pending().Count(&result.AwaitingCount).Error; err != nil {
			return err
		}
		start, _, _ := dateBounds(PageQuery{From: date})
		if err := pending().Where("f.confirmed_at < ?", *start).Count(&result.OvernightCount).Error; err != nil {
			return err
		}
		if err := pending().Select("t.*,f.confirmed_at AS first_confirmed_at").
			Order("f.confirmed_at ASC,t.id ASC").Limit(5).Find(&result.AwaitingTasks).Error; err != nil {
			return err
		}
		tickets, err := repo.Tickets(ctx, PageQuery{Status: "valid", Page: 1, PageSize: 5})
		result.RecentTickets = tickets.Records
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}
