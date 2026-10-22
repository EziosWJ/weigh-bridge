// Package nodesync owns independent node identity and durable central/site replication.
package nodesync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const idBlock int64 = 1 << 45

var businessTables = []string{"wb_transport_task", "wb_sample", "wb_weigh_record", "wb_ticket", "wb_operation", "wb_business_event", "wb_print_attempt"}
var masterTables = []string{"sys_dept", "sys_user", "sys_role", "sys_menu", "sys_user_role", "sys_role_menu", "wb_site", "wb_scale", "wb_user_site", "wb_material_category", "wb_material", "wb_party", "wb_party_contact", "wb_temporary_party", "wb_site_proposal"}

type Identity struct {
	ID          int64 `gorm:"primaryKey"`
	Role        string
	SiteID      int64
	NextID      int64
	Initialized int
}

func (Identity) TableName() string { return "wb_node_identity" }

type State struct {
	SiteID           int64  `json:"siteId" gorm:"primaryKey"`
	ReceivedSeq      int64  `json:"-"`
	SentSeq          int64  `json:"-"`
	LastSuccess      string `json:"lastSuccess"`
	LastError        string `json:"lastError"`
	Pending          string `json:"-"`
	AcknowledgedHash string `json:"-"`
	PendingImages    int    `json:"pendingImages"`
}

func (State) TableName() string { return "wb_sync_state" }

type Row = map[string]any
type Packet struct {
	Sequence int64            `json:"sequence"`
	Tables   map[string][]Row `json:"tables"`
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Bind refuses to reinterpret an existing database as a different authority.
// This performs data initialization only; tables must already exist through Goose.
func Bind(db *gorm.DB, cfg config.NodeConfig) error {
	if cfg.Role == "" {
		return nil
	}
	if cfg.Role != "site" && cfg.Role != "central" {
		return errors.New("invalid node role")
	}
	if cfg.Role == "site" && (cfg.SiteID < 1 || cfg.SiteID > 255) {
		return errors.New("invalid node site")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var old Identity
		e := tx.First(&old, 1).Error
		if e == nil {
			if old.Role != cfg.Role || old.SiteID != cfg.SiteID {
				return errors.New("database node identity cannot be changed")
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		{
			var count int64
			if err := tx.Table("wb_transport_task").Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return errors.New("node initialization requires an empty production transaction database; archive development data first")
			}
		}
		return tx.Create(&Identity{ID: 1, Role: cfg.Role, SiteID: cfg.SiteID}).Error
	})
	if err != nil {
		return err
	}
	// Existing business types retain numeric API IDs, partitioned by stable site ID.
	// Wall-clock floor advances IDs beyond an older restored DB; a DB counter handles
	// bursts and clock rollback during normal operation. Public numbers use UUIDs.
	return db.Callback().Create().Before("gorm:create").Register("nodesync:identity", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Error != nil {
			return
		}
		table := tx.Statement.Table
		tables := append(append([]string{}, businessTables...), "wb_party", "wb_party_contact")
		if cfg.Role == "central" {
			tables = append(tables, "wb_material", "wb_material_category")
		}
		if !contains(tables, table) {
			return
		}
		field := tx.Statement.Schema.LookUpField("ID")
		if field == nil {
			return
		}
		assign := func(v reflect.Value) {
			current, zero := field.ValueOf(tx.Statement.Context, v)
			_ = current
			if !zero {
				return
			}
			if cfg.Role == "central" {
				var next int64
				e := tx.Session(&gorm.Session{NewDB: true}).Raw("UPDATE wb_node_identity SET next_id=CASE WHEN next_id>=(SELECT COALESCE(MAX(id),0) FROM "+table+" WHERE id<?) THEN next_id+1 ELSE (SELECT COALESCE(MAX(id),0)+1 FROM "+table+" WHERE id<?) END WHERE id=1 RETURNING next_id", idBlock, idBlock).Scan(&next).Error
				if e != nil {
					tx.AddError(e)
					return
				}
				tx.AddError(field.Set(tx.Statement.Context, v, next))
				return
			}
			floor := time.Now().UTC().Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Milliseconds() * 32
			if floor < 1 || floor >= idBlock-1 {
				tx.AddError(errors.New("node clock outside supported identity epoch"))
				return
			}
			var next int64
			e := tx.Session(&gorm.Session{NewDB: true}).Raw("UPDATE wb_node_identity SET next_id=CASE WHEN next_id>=? THEN next_id+1 ELSE ? END WHERE id=1 RETURNING next_id", floor, floor).Scan(&next).Error
			if e != nil {
				tx.AddError(e)
				return
			}
			if next >= idBlock {
				tx.AddError(errors.New("node identity range exhausted"))
				return
			}
			tx.AddError(field.Set(tx.Statement.Context, v, cfg.SiteID*idBlock+next))
		}
		v := tx.Statement.ReflectValue
		switch v.Kind() {
		case reflect.Struct:
			assign(v)
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				assign(v.Index(i))
			}
		}
	})
}

func readRows(tx *gorm.DB, table string, query *gorm.DB) ([]Row, error) {
	rows := []map[string]any{}
	if query == nil {
		query = tx.Table(table)
	}
	for _, pk := range primary(table) {
		query = query.Order(pk.Name)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		for k, v := range row {
			if b, ok := v.([]byte); ok {
				row[k] = string(b)
			}
		}
		out = append(out, Row(row))
	}
	return out, nil
}
func primary(table string) []clause.Column {
	switch table {
	case "wb_user_site":
		return []clause.Column{{Name: "user_id"}, {Name: "site_id"}}
	case "sys_user_role":
		return []clause.Column{{Name: "user_id"}, {Name: "role_id"}}
	case "sys_role_menu":
		return []clause.Column{{Name: "role_id"}, {Name: "menu_id"}}
	case "wb_temporary_party":
		return []clause.Column{{Name: "party_id"}}
	case "wb_site_proposal":
		return []clause.Column{{Name: "site_id"}}
	default:
		return []clause.Column{{Name: "id"}}
	}
}

var columnCache sync.Map

func upsert(tx *gorm.DB, table string, row Row) error {
	m := map[string]any{}
	cacheKey := tx.Dialector.Name() + "/" + table
	cached, ok := columnCache.Load(cacheKey)
	if !ok {
		columns, err := tx.Migrator().ColumnTypes(table)
		if err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, col := range columns {
			allowed[col.Name()] = true
		}
		cached, _ = columnCache.LoadOrStore(cacheKey, allowed)
	}
	allowed := cached.(map[string]bool)
	for k, v := range row {
		if !allowed[k] {
			return fmt.Errorf("unsupported column %s.%s", table, k)
		}
		if n, ok := v.(json.Number); ok {
			i, e := n.Int64()
			if e != nil {
				return e
			}
			v = i
		}
		// Drivers accept RFC3339 timestamps on PostgreSQL; SQLite requires time.Time
		// to preserve comparisons with timestamps produced by GORM.
		if s, ok := v.(string); ok {
			if t, e := time.Parse(time.RFC3339Nano, s); e == nil {
				v = t
			}
		}
		m[k] = v
	}
	updates := []string{}
	for k := range m {
		isKey := false
		for _, pk := range primary(table) {
			if pk.Name == k {
				isKey = true
			}
		}
		if !isKey {
			updates = append(updates, k)
		}
	}
	sort.Strings(updates)
	return tx.Table(table).Clauses(clause.OnConflict{Columns: primary(table), DoUpdates: clause.AssignmentColumns(updates)}).Create(&m).Error
}
func state(tx *gorm.DB, site int64) (State, error) {
	s := State{SiteID: site}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&s).Error; err != nil {
		return s, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, "site_id=?", site).Error
	return s, err
}
func loadIdentity(ctx context.Context, db *gorm.DB) (Identity, error) {
	var v Identity
	err := db.WithContext(ctx).First(&v, 1).Error
	return v, err
}

type Digest struct {
	SiteID    int64
	TableName string
	RowID     int64
	Digest    string
}

func rowID(table string, r Row) int64 {
	key := "id"
	if table == "wb_temporary_party" {
		key = "party_id"
	}
	if table == "wb_site_proposal" {
		key = "site_id"
	}
	return number(r[key])
}
func delta(tx *gorm.DB, site int64, tables map[string][]Row) (map[string][]Row, int, error) {
	old := []Digest{}
	if err := tx.Table("wb_sync_digest").Where("site_id=?", site).Find(&old).Error; err != nil {
		return nil, 0, err
	}
	known := map[string]string{}
	for _, v := range old {
		known[fmt.Sprintf("%s/%d", v.TableName, v.RowID)] = v.Digest
	}
	changed := map[string][]Row{}
	count := 0
	for table, rows := range tables {
		for _, r := range rows {
			if known[fmt.Sprintf("%s/%d", table, rowID(table, r))] != hash(r) {
				changed[table] = append(changed[table], r)
				count++
			}
		}
	}
	return changed, count, nil
}
func acknowledge(tx *gorm.DB, site int64, tables map[string][]Row) error {
	for table, rows := range tables {
		for _, r := range rows {
			d := map[string]any{"site_id": site, "table_name": table, "row_id": rowID(table, r), "digest": hash(r)}
			if err := tx.Table("wb_sync_digest").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "table_name"}, {Name: "row_id"}}, DoUpdates: clause.AssignmentColumns([]string{"digest"})}).Create(&d).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
