package weighing

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
)

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

func dateBounds(q PageQuery) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	if q.From != "" {
		v, err := time.ParseInLocation("2006-01-02", q.From, beijing)
		if err != nil {
			return nil, nil, ErrInvalid
		}
		utc := v.UTC()
		from = &utc
	}
	if q.To != "" {
		v, err := time.ParseInLocation("2006-01-02", q.To, beijing)
		if err != nil {
			return nil, nil, ErrInvalid
		}
		utc := v.AddDate(0, 0, 1).UTC()
		to = &utc
	}
	if from != nil && to != nil && !from.Before(*to) {
		return nil, nil, ErrInvalid
	}
	return from, to, nil
}
func withDates(db *gorm.DB, q PageQuery, column string) (*gorm.DB, error) {
	from, to, err := dateBounds(q)
	if err != nil {
		return db, err
	}
	if from != nil {
		db = db.Where(column+">=?", *from)
	}
	if to != nil {
		db = db.Where(column+"<?", *to)
	}
	return db, nil
}
func taskFilters(db *gorm.DB, q PageQuery) *gorm.DB {
	db = siteFilter(db, q, "t.site_id")
	if q.Direction != "" {
		db = db.Where("t.direction=?", q.Direction)
	}
	if q.MaterialID != nil {
		db = db.Where("t.material_id=?", *q.MaterialID)
	}
	if q.PartyID != nil {
		db = db.Where("CAST(t.party_id AS TEXT)"+"=?", strconv.FormatInt(*q.PartyID, 10))
	}
	return db
}
func taskQuery(db *gorm.DB, q PageQuery) (*gorm.DB, error) {
	query := db.Table("wb_transport_task AS t").Joins("LEFT JOIN wb_weigh_record cf ON cf.id=t.current_first_record_id AND cf.task_id=t.id").Joins("LEFT JOIN wb_weigh_record cs ON cs.id=t.current_second_record_id AND cs.task_id=t.id").Select("t.*,cf.weight_milli_tonne AS first_weight,cs.weight_milli_tonne AS second_weight")
	query = taskFilters(query, q)
	if q.Status != "" {
		query = query.Where("t.status=?", q.Status)
	}
	if q.Keyword != "" {
		v := "%" + q.Keyword + "%"
		query = query.Where("t.plate LIKE ? OR t.task_no LIKE ? OR EXISTS (SELECT 1 FROM wb_ticket x WHERE x.task_id=t.id AND x.ticket_no LIKE ?)", v, v, v)
	}
	return withDates(query, q, "t.create_time")
}
func ticketQuery(db *gorm.DB, q PageQuery) (*gorm.DB, error) {
	query := db.Table("wb_ticket AS k").Joins("JOIN wb_transport_task t ON t.id=k.task_id").Select("k.*")
	query = siteFilter(query, q, "t.site_id")
	if q.TaskID != nil {
		query = query.Where("k.task_id=?", *q.TaskID)
	}
	if q.Status != "" {
		query = query.Where("k.status=?", q.Status)
	}
	if q.Direction != "" {
		query = query.Where(jsonIDExpression(query, "k.business_snapshot", "direction")+"=?", q.Direction)
	}
	if q.MaterialID != nil {
		query = query.Where(jsonIDExpression(query, "k.business_snapshot", "materialId")+"=?", strconv.FormatInt(*q.MaterialID, 10))
	}
	if q.PartyID != nil {
		query = query.Where(jsonIDExpression(query, "k.business_snapshot", "partyId")+"=?", strconv.FormatInt(*q.PartyID, 10))
	}
	if q.Keyword != "" {
		v := "%" + q.Keyword + "%"
		query = query.Where("k.ticket_no LIKE ? OR t.task_no LIKE ? OR "+jsonIDExpression(query, "k.business_snapshot", "plate")+" LIKE ?", v, v, v)
	}
	return withDates(query, q, "k.issued_at")
}
func originalQuery(db *gorm.DB, q PageQuery) (*gorm.DB, error) {
	query := db.Table("wb_transport_task AS t").Joins("JOIN wb_weigh_record f ON f.id=t.current_first_record_id AND f.task_id=t.id").Joins("JOIN wb_weigh_record s ON s.id=t.current_second_record_id AND s.task_id=t.id").Joins("LEFT JOIN wb_ticket vt ON vt.task_id=t.id AND vt.status='valid'")
	query = taskFilters(query, q)
	if q.Status == "valid" {
		query = query.Where("vt.id IS NOT NULL")
	} else if q.Status == "void" {
		query = query.Where("vt.id IS NULL")
	}
	if q.Keyword != "" {
		v := "%" + q.Keyword + "%"
		query = query.Where("t.plate LIKE ? OR t.task_no LIKE ? OR EXISTS (SELECT 1 FROM wb_ticket x WHERE x.task_id=t.id AND x.ticket_no LIKE ?)", v, v, v)
	}
	return withDates(query, q, "s.confirmed_at")
}

const originalGrossSQL = "CASE WHEN f.kind='gross' THEN f.weight_milli_tonne ELSE s.weight_milli_tonne END"
const originalTareSQL = "CASE WHEN f.kind='tare' THEN f.weight_milli_tonne ELSE s.weight_milli_tonne END"

func originalColumns() string {
	return "t.*, " + originalGrossSQL + " AS original_gross, " + originalTareSQL + " AS original_tare, f.confirmed_at AS first_confirmed_at, s.confirmed_at AS second_confirmed_at, vt.id AS current_ticket_id, CASE WHEN vt.id IS NULL THEN 'void' ELSE 'valid' END AS current_ticket_status"
}
func workflowPage[T any](query *gorm.DB, q PageQuery, order string) (Page[T], error) {
	result := Page[T]{Records: []T{}, Page: q.Page, PageSize: q.PageSize}
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := query.Order(order).Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&result.Records).Error
	return result, err
}
func (r *Repository) Originals(ctx context.Context, q PageQuery) (Page[OriginalView], error) {
	query, err := originalQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return Page[OriginalView]{}, err
	}
	page, err := workflowPage[OriginalView](query.Select(originalColumns()), q, "s.confirmed_at DESC,t.id DESC")
	for i := range page.Records {
		page.Records[i].OriginalNet = page.Records[i].OriginalGross - page.Records[i].OriginalTare
	}
	return page, err
}
func (r *Repository) OriginalsAll(ctx context.Context, q PageQuery) ([]OriginalView, error) {
	query, err := originalQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return nil, err
	}
	rows := []OriginalView{}
	err = query.Select(originalColumns()).Order("s.confirmed_at DESC,t.id DESC").Find(&rows).Error
	for i := range rows {
		rows[i].OriginalNet = rows[i].OriginalGross - rows[i].OriginalTare
	}
	return rows, err
}
func (r *Repository) TicketsAll(ctx context.Context, q PageQuery) ([]Ticket, error) {
	query, err := ticketQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return nil, err
	}
	rows := []Ticket{}
	err = query.Order("k.issued_at DESC,k.id DESC").Find(&rows).Error
	return rows, err
}
func (r *Repository) Summary(ctx context.Context, kind string, q PageQuery) (Summary, error) {
	result := Summary{Rows: []SummaryLine{}}
	var query *gorm.DB
	var err error
	var column, gross, tare, direction string
	if kind == "originals" {
		query, err = originalQuery(r.db.WithContext(ctx), q)
		if err != nil {
			return result, err
		}
		query = query.Where("vt.id IS NOT NULL")
		column = "t.business_snapshot"
		gross = originalGrossSQL
		tare = originalTareSQL
		direction = "t.direction"
	} else {
		query, err = ticketQuery(r.db.WithContext(ctx), q)
		if err != nil {
			return result, err
		}
		query = query.Where("k.status='valid'")
		column = "k.business_snapshot"
		gross = "k.final_gross_milli_tonne"
		tare = "k.final_tare_milli_tonne"
		direction = jsonIDExpression(query, column, "direction")
	}
	dimensions := []struct{ key, expression string }{{"site_id", "t.site_id"}, {"direction", direction}, {"material_id", jsonIDExpression(query, column, "materialId")}, {"material_name", jsonIDExpression(query, column, "materialName")}, {"material_spec", jsonIDExpression(query, column, "materialSpec")}, {"party_id", jsonIDExpression(query, column, "partyId")}, {"party_name", jsonIDExpression(query, column, "partyName")}}
	selects := []string{}
	groups := []string{}
	for _, d := range dimensions {
		selects = append(selects, d.expression+" AS "+d.key)
		groups = append(groups, d.expression)
	}
	selects = append(selects, "COUNT(*) AS task_count", "SUM("+gross+") AS gross", "SUM("+tare+") AS tare", "SUM(("+gross+")-("+tare+")) AS net")
	if err := query.Select(strings.Join(selects, ",")).Group(strings.Join(groups, ",")).Order("material_id,party_id,direction").Scan(&result.Rows).Error; err != nil {
		return result, err
	}
	for _, row := range result.Rows {
		if row.Gross < 0 || row.Tare < 0 || row.Net < 0 || int64(result.Gross) > math.MaxInt64-int64(row.Gross) || int64(result.Tare) > math.MaxInt64-int64(row.Tare) || int64(result.Net) > math.MaxInt64-int64(row.Net) {
			return result, ErrInvalid
		}
		result.TaskCount += row.TaskCount
		result.Gross += row.Gross
		result.Tare += row.Tare
		result.Net += row.Net
	}
	return result, nil
}
func (r *Repository) RecordExport(ctx context.Context, kind string, q PageQuery, count int64, e audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		name, err := actorName(tx, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		e.Action = kind + ".export"
		e.Resource = "weighing.export"
		event := BusinessEvent{ResourceType: e.Resource, Action: e.Action, ActorID: e.Metadata.ActorID, ActorName: name, After: snapshot(map[string]any{"filter": q, "rowCount": count}), OccurredAt: time.Now().UTC()}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

func (r *Repository) TasksAll(ctx context.Context, q PageQuery) ([]Task, error) {
	query, err := taskQuery(r.db.WithContext(ctx), q)
	if err != nil {
		return nil, err
	}
	rows := []Task{}
	err = query.Order("t.create_time DESC,t.id DESC").Find(&rows).Error
	return rows, err
}

func (r *Repository) TaskEvents(ctx context.Context, id int64, q PageQuery) (Page[BusinessEvent], error) {
	result := Page[BusinessEvent]{Records: []BusinessEvent{}, Page: q.Page, PageSize: q.PageSize}
	if _, err := findOne[Task](r.db.WithContext(ctx), id); err != nil {
		return result, err
	}
	db := r.db.WithContext(ctx).Model(&BusinessEvent{}).Where("task_id=?", id)
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := db.Order("occurred_at DESC,id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&result.Records).Error
	return result, err
}
