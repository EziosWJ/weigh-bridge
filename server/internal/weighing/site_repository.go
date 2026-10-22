package weighing

import (
	"context"
	"errors"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"slices"
	"time"
)

func mayOperate(ctx context.Context, id int64) bool {
	scope, ok := ctx.Value(siteScopeKey).(siteScope)
	return ok && slices.Contains(scope.Operate, id)
}
func siteFilter(db *gorm.DB, q PageQuery, column string) *gorm.DB {
	scope, ok := db.Statement.Context.Value(siteScopeKey).(siteScope)
	if ok {
		db = db.Where(column+" IN ?", scope.Read)
	}
	if q.SiteID != nil {
		db = db.Where(column+"=?", *q.SiteID)
	}
	return db
}
func (r *Repository) Sites(ctx context.Context, user int64) ([]Site, error) {
	rows := []Site{}
	db := r.db.WithContext(ctx).Table("wb_site s").Select("s.*,CASE WHEN s.enabled=1 THEN us.can_operate ELSE 0 END AS can_operate").Joins("JOIN wb_user_site us ON us.site_id=s.id AND us.user_id=?", user)
	if scope, ok := ctx.Value(siteScopeKey).(siteScope); ok {
		db = db.Where("s.id IN ?", scope.Read)
	}
	err := db.Order("s.id").Find(&rows).Error
	return rows, err
}
func (r *Repository) SiteOptions(ctx context.Context) ([]Site, error) {
	rows := []Site{}
	err := r.db.WithContext(ctx).Order("id").Find(&rows).Error
	return rows, err
}
func (r *Repository) SiteGrants(ctx context.Context, user int64) ([]SiteGrant, error) {
	rows := []SiteGrant{}
	err := r.db.WithContext(ctx).Where("user_id=?", user).Order("site_id").Find(&rows).Error
	return rows, err
}
func (r *Repository) SaveSiteGrants(ctx context.Context, user int64, rows []SiteGrant, e audit.Event) error {
	if len(rows) > 100 {
		return ErrInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u struct{ ID int64 }
		if err := tx.Table("sys_user").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted=0", user).Take(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		seen := map[int64]bool{}
		for i := range rows {
			v := &rows[i]
			if v.SiteID < 1 || seen[v.SiteID] || (v.CanOperate != 0 && v.CanOperate != 1) {
				return ErrInvalid
			}
			seen[v.SiteID] = true
			v.UserID = user
			var n int64
			if err := tx.Model(&Site{}).Where("id=?", v.SiteID).Count(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return ErrInvalid
			}
		}
		if err := tx.Where("user_id=?", user).Delete(&SiteGrant{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		e.Action = "user.sites"
		e.Resource = "weighing.user-sites"
		e.ResourceID = user
		return audit.RecordOn(ctx, tx, e)
	})
}

// ResourceSite resolves ownership from persisted rows, never client-provided site ids.
func (r *Repository) ResourceSite(ctx context.Context, kind string, id int64) (int64, error) {
	db := r.db.WithContext(ctx)
	switch kind {
	case "scales":
		db = db.Table("wb_scale").Select("site_id").Where("id=?", id)
	case "tasks":
		db = db.Table("wb_transport_task").Select("site_id").Where("id=?", id)
	case "tickets":
		db = db.Table("wb_ticket").Select("site_id").Where("id=?", id)
	case "samples":
		db = db.Table("wb_sample x").Select("s.site_id").Joins("JOIN wb_scale s ON s.id=x.scale_id").Where("x.id=?", id)
	case "records":
		db = db.Table("wb_weigh_record x").Select("t.site_id").Joins("JOIN wb_transport_task t ON t.id=x.task_id").Where("x.id=?", id)
	case "print-attempts":
		db = db.Table("wb_print_attempt x").Select("t.site_id").Joins("JOIN wb_ticket t ON t.id=x.ticket_id").Where("x.id=?", id)
	default:
		return 0, ErrInvalid
	}
	var row struct{ SiteID int64 }
	result := db.Take(&row)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return 0, ErrNotFound
		}
		return 0, result.Error
	}
	return row.SiteID, nil
}
func (r *Repository) RecordAgentKick(ctx context.Context, e audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		name, err := actorName(tx, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		event := BusinessEvent{ResourceType: e.Resource, ResourceID: ptr(e.ResourceID), Action: e.Action, ActorID: e.Metadata.ActorID, ActorName: name, After: JSON(e.Summary), OccurredAt: time.Now().UTC()}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

// Serialize business writes and site disable without changing replayed commands.
func lockWriteSite(tx *gorm.DB, ctx context.Context) error {
	scope, ok := ctx.Value(siteScopeKey).(siteScope)
	if !ok || scope.WriteSiteID == 0 {
		return nil
	}
	site, err := lockRow[Site](tx, scope.WriteSiteID)
	if err != nil {
		return err
	}
	if site.Enabled != 1 {
		return ErrForbidden
	}
	return nil
}
