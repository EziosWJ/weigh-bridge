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

var ErrNotConfigured = errors.New("本厂内部身份未就绪，请检查服务启动日志与数据库")

type Identity struct {
	ID          int64 `gorm:"primaryKey;autoIncrement:false"`
	SiteID      int64
	NextID      int64
	Initialized int
}

func (Identity) TableName() string { return "wb_local_identity" }

type Status struct {
	SiteID      int64 `json:"siteId"`
	Configured  bool  `json:"configured"`
	Initialized bool  `json:"initialized"`
}

func Read(ctx context.Context, db *gorm.DB) (Identity, error) {
	var identity Identity
	err := db.WithContext(ctx).Where("id=?", 1).Limit(1).Find(&identity).Error
	return identity, err
}

// ReadStatus reports the completed local configuration. Since ADR 0029 the
// first identity transaction is sufficient: neither operator headcount nor
// device readiness participates. The legacy initialized column is retained for
// storage compatibility but is neither read nor written here, so databases
// configured by older builds with initialized=0 keep working after upgrade.
func ReadStatus(ctx context.Context, db *gorm.DB) (Status, error) {
	identity, err := Read(ctx, db)
	if err != nil {
		return Status{}, err
	}
	configured := identity.SiteID > 0
	return Status{SiteID: identity.SiteID, Configured: configured, Initialized: configured}, nil
}

// Bind installs a single allocator for numeric business IDs. No identity is
// cached, so first-time configuration becomes effective without an API restart.
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
			var allocated struct{ SiteID, NextID int64 }
			result := tx.Session(&gorm.Session{NewDB: true}).Raw("UPDATE wb_local_identity SET next_id=CASE WHEN next_id>=? THEN next_id+1 ELSE ? END WHERE id=1 RETURNING site_id,next_id", floor, floor).Scan(&allocated)
			if result.Error != nil {
				tx.AddError(result.Error)
				return
			}
			if allocated.SiteID == 0 {
				tx.AddError(ErrNotConfigured)
				return
			}
			if allocated.NextID >= idBlock {
				tx.AddError(errors.New("本厂业务编号范围已耗尽"))
				return
			}
			tx.AddError(field.Set(tx.Statement.Context, v, allocated.SiteID*idBlock+allocated.NextID))
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
