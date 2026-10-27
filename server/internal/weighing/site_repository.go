package weighing

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func mayOperate(ctx context.Context, code string) bool {
	scope, ok := ctx.Value(siteScopeKey).(siteScope)
	return ok && slices.Contains(scope.Operate, code)
}

// The database represents one local factory. The scope authorizes access to
// that database; codes saved on historical rows are snapshots, not ownership.
func siteFilter(db *gorm.DB, _ PageQuery, _ string) *gorm.DB {
	if scope, ok := db.Statement.Context.Value(siteScopeKey).(siteScope); ok && len(scope.Read) == 0 {
		return db.Where("1=0")
	}
	return db
}
func (r *Repository) Sites(ctx context.Context, user int64) ([]Site, error) {
	rows := []Site{}
	db := r.db.WithContext(ctx).Table("wb_site s").Select("s.*,CASE WHEN s.enabled=1 THEN us.can_operate ELSE 0 END AS can_operate").Joins("JOIN wb_local_identity i ON i.id=1 AND i.site_code=s.site_code").Joins("JOIN wb_local_user_access us ON us.user_id=?", user)
	if scope, ok := ctx.Value(siteScopeKey).(siteScope); ok {
		db = db.Where("s.site_code IN ?", scope.Read)
	}
	err := db.Order("s.site_code").Find(&rows).Error
	return rows, err
}
func (r *Repository) SiteOptions(ctx context.Context) ([]Site, error) {
	rows := []Site{}
	err := r.db.WithContext(ctx).Order("site_code").Find(&rows).Error
	return rows, err
}
func (r *Repository) SiteGrants(ctx context.Context, user int64) ([]SiteGrant, error) {
	rows := []SiteGrant{}
	err := r.db.WithContext(ctx).Table("wb_local_user_access a").Select("a.user_id, i.site_code, a.can_operate").Joins("JOIN wb_local_identity i ON i.id=1 AND i.site_code IS NOT NULL").Where("a.user_id=?", user).Scan(&rows).Error
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
		seen := map[string]bool{}
		for i := range rows {
			v := &rows[i]
			if v.SiteCode == "" || seen[v.SiteCode] || (v.CanOperate != 0 && v.CanOperate != 1) {
				return ErrInvalid
			}
			seen[v.SiteCode] = true
			v.UserID = user
			var n int64
			if err := tx.Model(&Site{}).Where("site_code=?", v.SiteCode).Count(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return ErrInvalid
			}
		}
		if err := tx.Exec("DELETE FROM wb_local_user_access WHERE user_id=?", user).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.Exec("INSERT INTO wb_local_user_access (user_id, can_operate) VALUES (?, ?)", user, rows[0].CanOperate).Error; err != nil {
				return err
			}
		}
		e.Action = "user.sites"
		e.Resource = "weighing.user-sites"
		e.ResourceID = user
		return audit.RecordOn(ctx, tx, e)
	})
}

// ResourceScaleSite resolves the owning site of a scale by its code.
func (r *Repository) ResourceScaleSite(ctx context.Context, scaleCode string) (string, error) {
	var row struct{ SiteCode string }
	err := r.db.WithContext(ctx).Table("wb_scale").Select("site_code").Where("code=?", scaleCode).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return row.SiteCode, err
}

// ResourceSite verifies that the resource exists locally and resolves its
// authorization scope from the current local identity, not a historical code.
func (r *Repository) ResourceSite(ctx context.Context, kind string, id int64) (string, error) {
	table := map[string]string{"tasks": "wb_transport_task", "tickets": "wb_ticket", "samples": "wb_sample", "records": "wb_weigh_record", "print-attempts": "wb_print_attempt"}[kind]
	if table == "" {
		return "", ErrInvalid
	}
	var row struct{ SiteCode string }
	result := r.db.WithContext(ctx).Table(table).Select("(SELECT site_code FROM wb_local_identity WHERE id=1) AS site_code").Where("id=?", id).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return row.SiteCode, result.Error
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
	if !ok || len(scope.Operate) == 0 {
		return nil
	}
	var site Site
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_code=? AND enabled=1", scope.Operate[0]).Take(&site).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		}
		return err
	}
	return nil
}
