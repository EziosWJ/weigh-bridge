// Package localsite owns this installation's fixed identity and business readiness.
package localsite

import (
	"context"
	"errors"
	"reflect"
	"time"

	"gorm.io/gorm"
)

const idBlock int64 = 1 << 45

var ErrNotConfigured = errors.New("本厂身份未就绪，请在系统管理→本厂身份完成配置")

type Identity struct {
	ID             int64 `gorm:"primaryKey;autoIncrement:false"`
	SiteCode       string
	ScaleCode      string
	NextID         int64
	InstallationID string `gorm:"->"`
}

func (Identity) TableName() string { return "wb_local_identity" }

type Status struct {
	SiteCode    string `json:"siteCode"`
	ScaleCode   string `json:"scaleCode"`
	Configured  bool   `json:"configured"`
	Initialized bool   `json:"initialized"`
}

func Read(ctx context.Context, db *gorm.DB) (Identity, error) {
	var identity Identity
	err := db.WithContext(ctx).Where("id=?", 1).Limit(1).Find(&identity).Error
	return identity, err
}

// ReadStatus reports the current local configuration. Identity values are
// maintained through the dedicated local identity entry.
// The legacy initialized column is retained for storage compatibility but is
// neither read nor written here.
func ReadStatus(ctx context.Context, db *gorm.DB) (Status, error) {
	identity, err := Read(ctx, db)
	if err != nil {
		return Status{}, err
	}
	configured := identity.SiteCode != ""
	return Status{SiteCode: identity.SiteCode, ScaleCode: identity.ScaleCode, Configured: configured, Initialized: configured}, nil
}

// Bind installs a single allocator for numeric business row ids. These remain
// internal surrogate keys; they no longer encode any site identity. No
// identity is cached, so first-time configuration becomes effective without an
// API restart.
func Bind(db *gorm.DB) error {
	if _, err := Read(context.Background(), db); err != nil {
		return err
	}
	if db.Callback().Create().Get("localsite:identity") != nil {
		return nil
	}
	tables := map[string]bool{"wb_transport_task": true, "wb_sample": true, "wb_weigh_record": true, "wb_ticket": true, "wb_operation": true, "wb_business_event": true, "wb_print_attempt": true, "wb_party": true, "wb_party_contact": true, "wb_material": true, "wb_material_category": true}
	return db.Callback().Create().Before("gorm:create").Register("localsite:identity", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Error != nil || !tables[tx.Statement.Table] {
			return
		}
		field := tx.Statement.Schema.LookUpField("ID")
		if field == nil {
			return
		}
		assign := func(v reflect.Value) {
			_, zero := field.ValueOf(tx.Statement.Context, v)
			if !zero {
				return
			}
			floor := time.Now().UTC().Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Milliseconds() * 32
			if floor < 1 || floor >= idBlock-1 {
				tx.AddError(errors.New("本机时间超出业务编号支持范围"))
				return
			}
			var allocated struct {
				SiteCode string
				NextID   int64
			}
			result := tx.Session(&gorm.Session{NewDB: true}).Raw("UPDATE wb_local_identity SET next_id=CASE WHEN next_id>=? THEN next_id+1 ELSE ? END WHERE id=1 RETURNING site_code,next_id", floor, floor).Scan(&allocated)
			if result.Error != nil {
				tx.AddError(result.Error)
				return
			}
			if allocated.SiteCode == "" {
				tx.AddError(ErrNotConfigured)
				return
			}
			if allocated.NextID >= idBlock {
				tx.AddError(errors.New("本厂业务编号范围已耗尽"))
				return
			}
			tx.AddError(field.Set(tx.Statement.Context, v, allocated.NextID))
		}
		value := tx.Statement.ReflectValue
		switch value.Kind() {
		case reflect.Struct:
			assign(value)
		case reflect.Slice, reflect.Array:
			for i := 0; i < value.Len(); i++ {
				v := value.Index(i)
				if v.Kind() == reflect.Ptr {
					v = v.Elem()
				}
				assign(v)
			}
		}
	})
}

// CheckAdminBootstrap keeps the seeded default password out of production.
// It is independent of whether the local factory has been configured yet.
func CheckAdminBootstrap(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Table("wb_local_admin_bootstrap").Where("id=1").Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("请先运行 local-admin-init 设置本地管理员口令")
	}
	return nil
}
