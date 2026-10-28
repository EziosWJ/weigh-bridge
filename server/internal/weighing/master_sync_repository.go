package weighing

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/mastersync"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type masterSyncState struct {
	ID            int `gorm:"primaryKey"`
	Bound         bool
	ConfigEnabled bool
	LastSuccess   *time.Time
}

func (masterSyncState) TableName() string { return "wb_master_sync_state" }

type categoryMapping struct {
	CloudCode string `gorm:"primaryKey"`
	LocalID   int64
}

func (categoryMapping) TableName() string { return "wb_master_category_map" }

type contactMapping struct {
	CloudID int64 `gorm:"primaryKey"`
	LocalID int64
}

func (contactMapping) TableName() string { return "wb_master_contact_map" }

// applyMasterSnapshot atomically replaces the active cloud view. Local IDs and
// existing task snapshots remain intact. It accepts only a previously validated
// snapshot and checks identity again under the same lock used by identity edits.
func (r *Repository) applyMasterSnapshot(ctx context.Context, snapshot mastersync.Snapshot) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		var state masterSyncState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, 1).Error; err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		matched := false
		for _, code := range snapshot.ScaleCodes {
			if code == identity.ScaleCode {
				matched = true
			}
		}
		if identity.SiteCode == "" || identity.ScaleCode == "" || identity.SiteCode != snapshot.SiteCode || !matched || snapshot.SchemaVersion != 1 {
			return RejectionError("厂区或地磅身份不匹配")
		}
		// A missing previously synced record must reject the entire snapshot.
		// Contacts are intentionally excluded: headquarters may remove them.
		sets := []struct {
			table string
			codes map[string]bool
		}{
			{"wb_master_category_map", map[string]bool{}},
			{"wb_master_material_map", map[string]bool{}},
			{"wb_master_party_map", map[string]bool{}},
		}
		for _, v := range snapshot.Categories {
			sets[0].codes[v.Code] = true
		}
		for _, v := range snapshot.Materials {
			sets[1].codes[v.Code] = true
		}
		for _, v := range snapshot.Parties {
			sets[2].codes[v.Code] = true
		}
		for _, set := range sets {
			var codes []string
			if err := tx.Table(set.table).Pluck("cloud_code", &codes).Error; err != nil {
				return err
			}
			for _, code := range codes {
				if !set.codes[code] {
					return RejectionError("总部资料缺失，已保留上一版")
				}
			}
		}
		categoryIDs := map[string]int64{}
		if err := tx.Model(&Category{}).Where("id IN (SELECT local_id FROM wb_master_category_map)").Updates(map[string]any{"enabled": 0, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		for _, item := range snapshot.Categories {
			var m categoryMapping
			err := tx.Where("cloud_code = ?", item.Code).Take(&m).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c := Category{MasterMeta: MasterMeta{Enabled: flag(item.Enabled), Version: 1}, Name: item.Name}
				if err := tx.Create(&c).Error; err != nil {
					return err
				}
				m = categoryMapping{CloudCode: item.Code, LocalID: c.ID}
				if err := tx.Create(&m).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if err := tx.Model(&Category{}).Where("id = ?", m.LocalID).Updates(map[string]any{"name": item.Name, "enabled": flag(item.Enabled)}).Error; err != nil {
				return err
			}
			categoryIDs[item.Code] = m.LocalID
		}
		if err := tx.Model(&Material{}).Where("id IN (SELECT local_id FROM wb_master_material_map)").Updates(map[string]any{"enabled": 0, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		for _, item := range snapshot.Materials {
			var categoryID *int64
			if item.CategoryCode != nil {
				id, ok := categoryIDs[*item.CategoryCode]
				if !ok {
					return RejectionError("分类引用无效")
				}
				categoryID = &id
			}
			var row Material
			err := tx.Where("code = ?", item.Code).Take(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				row = Material{MasterMeta: MasterMeta{Enabled: flag(item.Enabled), Version: 1}, Code: item.Code, Name: item.Name, CategoryID: categoryID, Spec: item.Spec, Remark: item.Remark}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				if err := tx.Table("wb_master_material_map").Create(map[string]any{"cloud_code": item.Code, "local_id": row.ID}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				var count int64
				if err := tx.Table("wb_master_material_map").Where("cloud_code = ? AND local_id = ?", item.Code, row.ID).Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return RejectionError("总部物料编码与本地资料冲突")
				}
				if err := tx.Model(&row).Updates(map[string]any{"name": item.Name, "category_id": categoryID, "spec": item.Spec, "remark": item.Remark, "enabled": flag(item.Enabled)}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&Party{}).Where("id IN (SELECT local_id FROM wb_master_party_map)").Updates(map[string]any{"enabled": 0, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		for _, item := range snapshot.Parties {
			var row Party
			err := tx.Where("code = ?", item.Code).Take(&row).Error
			values := map[string]any{"name": item.Name, "short_name": item.ShortName, "nature": item.Nature, "is_supplier": flag(item.IsSupplier), "is_customer": flag(item.IsCustomer), "is_carrier": flag(item.IsCarrier), "phone": item.Phone, "address": item.Address, "credit_code": item.CreditCode, "remark": item.Remark, "enabled": flag(item.Enabled)}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				row = Party{MasterMeta: MasterMeta{Enabled: flag(item.Enabled), Version: 1}, Code: item.Code, Name: item.Name, ShortName: item.ShortName, Nature: item.Nature, IsSupplier: flag(item.IsSupplier), IsCustomer: flag(item.IsCustomer), IsCarrier: flag(item.IsCarrier), Phone: item.Phone, Address: item.Address, CreditCode: item.CreditCode, Remark: item.Remark}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				if err := tx.Table("wb_master_party_map").Create(map[string]any{"cloud_code": item.Code, "local_id": row.ID}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				var count int64
				if err := tx.Table("wb_master_party_map").Where("cloud_code = ? AND local_id = ?", item.Code, row.ID).Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return RejectionError("总部主体编码与本地资料冲突")
				}
				if err := tx.Model(&row).Updates(values).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Contact{}).Where("party_id = ?", row.ID).Update("is_default", 0).Error; err != nil {
				return err
			}
			keep := []int64{}
			for _, c := range item.Contacts {
				var m contactMapping
				err := tx.Where("cloud_id = ?", c.ID).Take(&m).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					local := Contact{PartyID: row.ID, Name: c.Name, Phone: c.Phone, Purpose: c.Purpose, Remark: c.Remark, IsDefault: flag(c.IsDefault)}
					if err := tx.Create(&local).Error; err != nil {
						return err
					}
					m = contactMapping{CloudID: c.ID, LocalID: local.ID}
					if err := tx.Create(&m).Error; err != nil {
						return err
					}
				} else if err != nil {
					return err
				} else {
					var local Contact
					if err := tx.First(&local, m.LocalID).Error; err != nil {
						return err
					}
					if local.PartyID != row.ID {
						return RejectionError("联系人归属变化")
					}
					if err := tx.Model(&local).Updates(map[string]any{"name": c.Name, "phone": c.Phone, "purpose": c.Purpose, "remark": c.Remark, "is_default": flag(c.IsDefault)}).Error; err != nil {
						return err
					}
				}
				keep = append(keep, m.LocalID)
			}
			q := tx.Where("party_id = ?", row.ID)
			if len(keep) > 0 {
				q = q.Where("id NOT IN ?", keep)
			}
			if err := q.Delete(&Contact{}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&state).Updates(map[string]any{"bound": true, "last_success": time.Now().UTC()}).Error
	})
}

// Use the identity lock for both local writes and binding, closing the race
// between the first snapshot and an administrator editing a local record.
func rejectCloudMasterWrite(tx *gorm.DB) error {
	if _, err := lockRow[AgentSetting](tx, 1); err != nil {
		return err
	}
	var state masterSyncState
	if err := tx.First(&state, 1).Error; err != nil {
		return err
	}
	if state.Bound {
		return RejectionError("总部资料只能通过同步更新")
	}
	return nil
}
