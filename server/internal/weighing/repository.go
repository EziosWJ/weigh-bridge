package weighing

import (
	"context"
	"errors"
	"fmt"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"slices"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db         *gorm.DB
	sessionTTL time.Duration
	dataDir    string
}
type RuntimeOptions struct {
	SessionTTL time.Duration
	// DataDir holds the out-of-database ticket-number watermark so restoring an
	// older database backup cannot reuse an issued serial.
	DataDir string
}

type maintenanceIdentity struct {
	SiteCode  string
	ScaleCode string
	ServerURL string
	AgentID   string
	AgentKey  string
}

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB, options ...RuntimeOptions) *Repository {
	r := &Repository{db: db, sessionTTL: time.Minute}
	if len(options) > 0 {
		if options[0].SessionTTL > 0 {
			r.sessionTTL = options[0].SessionTTL
		}
		r.dataDir = options[0].DataDir
	}
	return r
}

func matchesMaintenanceIdentity(candidate agentprinter.Config, identity maintenanceIdentity) bool {
	return candidate.SiteCode == identity.SiteCode && candidate.ScaleCode == identity.ScaleCode &&
		candidate.ServerURL == identity.ServerURL && candidate.AgentID == identity.AgentID &&
		candidate.AgentKey == identity.AgentKey
}

func masterFilter(db *gorm.DB, q PageQuery) *gorm.DB {
	if q.Enabled != nil {
		db = db.Where("enabled=?", *q.Enabled)
	}
	return db
}
func readPage[T any](db *gorm.DB, q PageQuery) (Page[T], error) {
	p := Page[T]{Records: []T{}, Page: q.Page, PageSize: q.PageSize}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	err := db.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, err
}
func (r *Repository) Categories(ctx context.Context, q PageQuery) (Page[Category], error) {
	db := masterFilter(r.db.WithContext(ctx).Model(&Category{}), q)
	if q.Keyword != "" {
		db = db.Where("LOWER(name) LIKE LOWER(?)", "%"+q.Keyword+"%")
	}
	return readPage[Category](db, q)
}
func (r *Repository) Materials(ctx context.Context, q PageQuery) (Page[Material], error) {
	db := masterFilter(r.db.WithContext(ctx).Model(&Material{}), q)
	if q.Keyword != "" {
		v := "%" + q.Keyword + "%"
		db = db.Where("LOWER(name) LIKE LOWER(?) OR LOWER(code) LIKE LOWER(?) OR LOWER(spec) LIKE LOWER(?)", v, v, v)
	}
	if q.CategoryID != nil {
		db = db.Where("category_id=?", *q.CategoryID)
	}
	return readPage[Material](db, q)
}
func (r *Repository) Parties(ctx context.Context, q PageQuery) (Page[Party], error) {
	db := masterFilter(r.db.WithContext(ctx).Model(&Party{}), q)
	if q.Keyword != "" {
		v := "%" + q.Keyword + "%"
		db = db.Where("LOWER(name) LIKE LOWER(?) OR LOWER(code) LIKE LOWER(?) OR LOWER(short_name) LIKE LOWER(?) OR phone LIKE ?", v, v, v, v)
	}
	if q.Nature != "" {
		db = db.Where("nature=?", q.Nature)
	}
	switch q.Role {
	case "supplier":
		db = db.Where("is_supplier=1")
	case "customer":
		db = db.Where("is_customer=1")
	case "carrier":
		db = db.Where("is_carrier=1")
	}
	p, err := readPage[Party](db, q)
	if err != nil {
		return p, err
	}
	// Load all contacts in one query, including empty contact lists.
	ids := make([]int64, 0, len(p.Records))
	for _, v := range p.Records {
		ids = append(ids, v.ID)
	}
	contacts := []Contact{}
	if len(ids) > 0 {
		if err := r.db.WithContext(ctx).Where("party_id IN ?", ids).Order("id").Find(&contacts).Error; err != nil {
			return p, err
		}
	}
	byParty := map[int64][]Contact{}
	for _, c := range contacts {
		byParty[c.PartyID] = append(byParty[c.PartyID], c)
	}
	for i := range p.Records {
		decorateParty(&p.Records[i], byParty[p.Records[i].ID])
	}
	return p, nil
}
func findOne[T any](db *gorm.DB, id int64) (T, error) {
	var v T
	err := db.First(&v, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return v, err
}
func (r *Repository) FindCategory(ctx context.Context, id int64) (Category, error) {
	return findOne[Category](r.db.WithContext(ctx), id)
}
func (r *Repository) FindMaterial(ctx context.Context, id int64) (Material, error) {
	return findOne[Material](r.db.WithContext(ctx), id)
}
func (r *Repository) FindParty(ctx context.Context, id int64) (Party, error) {
	v, err := findOne[Party](r.db.WithContext(ctx), id)
	if err != nil {
		return v, err
	}
	contacts := []Contact{}
	err = r.db.WithContext(ctx).Where("party_id=?", id).Order("id").Find(&contacts).Error
	decorateParty(&v, contacts)
	return v, err
}
func decorateParty(v *Party, contacts []Contact) {
	v.Roles = []string{}
	v.Contacts = contacts
	if v.Contacts == nil {
		v.Contacts = []Contact{}
	}
	if v.IsSupplier == 1 {
		v.Roles = append(v.Roles, "supplier")
	}
	if v.IsCustomer == 1 {
		v.Roles = append(v.Roles, "customer")
	}
	if v.IsCarrier == 1 {
		v.Roles = append(v.Roles, "carrier")
	}
}
func (r *Repository) Scales(ctx context.Context) ([]Scale, error) {
	v := []Scale{}
	db := r.db.WithContext(ctx).Table("wb_scale").Select("wb_scale.*,s.name AS site_name").Joins("JOIN wb_site s ON s.site_code=wb_scale.site_code")
	err := siteFilter(db, PageQuery{}, "wb_scale.site_code").Order("wb_scale.site_code,wb_scale.code").Find(&v).Error
	for i := range v {
		v[i].CanOperate = mayOperate(ctx, v[i].SiteCode)
	}
	return v, err
}
func lockRow[T any](tx *gorm.DB, id int64) (T, error) {
	return findOne[T](tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
}
func lockRowWhere[T any](tx *gorm.DB, query string, args ...any) (T, error) {
	var v T
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(query, args...).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return v, err
}
func meta(old MasterMeta, id, version, actor int64) (MasterMeta, error) {
	if id > 0 && old.Version != version {
		return old, ErrConflict
	}
	if id == 0 {
		old.Version = 0
		old.CreateBy = actor
	}
	old.Version++
	old.UpdateBy = actor
	return old, nil
}

// nextNumber allocates per-site, per-day sequence numbers. Tickets use the
// "siteCode-YYYYMMDD-NNNN" form required by ADR 0033; task numbers share the
// same daily sequence with an RW prefix; material codes keep the WL<date><nnnn>
// form and party codes stay random. The atomic upsert serializes concurrent
// issuers inside the same transaction as the business write, so a network
// outage never blocks local numbering.
func nextNumber(tx *gorm.DB, kind, prefix, dataDir string, at time.Time) (string, error) {
	identity, err := localsite.Read(tx.Statement.Context, tx)
	if err != nil {
		return "", err
	}
	if identity.SiteCode == "" {
		return "", localsite.ErrNotConfigured
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	// The business date is taken from the same instant that freezes the
	// document, so a confirmation crossing Beijing midnight cannot produce a
	// number dated differently from its issue time (ADR 0033).
	date := businessDate(at)
	if kind == "party" {
		return prefix + "-" + identity.SiteCode + "-" + agentbridge.ID(), nil
	}
	number, err := allocateDaily(tx, kind, identity.SiteCode, date, dataDir)
	if err != nil {
		return "", err
	}
	switch kind {
	case "material":
		return fmt.Sprintf("%s%s%04d", prefix, date, number), nil
	case "task":
		return fmt.Sprintf("%s-%s-%s-%04d", prefix, identity.SiteCode, date, number), nil
	default:
		return fmt.Sprintf("%s-%s-%04d", identity.SiteCode, date, number), nil
	}
}

// allocateDaily bumps the (site, kind, date) counter atomically. Before a new
// sequence row is created the counter is raised above any higher number already
// issued for the same prefix, both from rows still in the database and from the
// out-of-database watermark file, so restoring an older backup cannot reuse an
// issued number (ADR 0033).
// businessDate renders the Beijing-time business date of an instant.
func businessDate(at time.Time) string { return at.UTC().Add(8 * time.Hour).Format("20060102") }

func allocateDaily(tx *gorm.DB, kind, siteCode, date, dataDir string) (int64, error) {
	head := siteCode + "-" + date + "-"
	floorTable, floorColumn, floorPrefix := "", "", ""
	switch kind {
	case "ticket", "test_ticket":
		floorTable, floorColumn, floorPrefix = "wb_ticket", "ticket_no", head
	case "task":
		floorTable, floorColumn, floorPrefix = "wb_transport_task", "task_no", "RW-"+head
	case "material":
		floorTable, floorColumn, floorPrefix = "wb_material", "code", "WL"+date
	}
	floor := "0"
	if floorTable != "" {
		floor = fmt.Sprintf(`COALESCE((SELECT MAX(SUBSTRING(%[2]s FROM %[3]d)::BIGINT) FROM %[1]s WHERE %[2]s LIKE ? || '%%' AND SUBSTRING(%[2]s FROM %[3]d) ~ '^[0-9]+$'),0)`, floorTable, floorColumn, len(floorPrefix)+1)
	}
	key := watermarkKey{Kind: kind, SiteCode: siteCode, Date: date}
	fileFloor, err := watermarkFloor(dataDir, key)
	if err != nil {
		return 0, err
	}
	// The floor expression is interpolated twice (insert and conflict update),
	// each carrying one placeholder, so the prefix argument repeats per use.
	args := []any{siteCode, kind, date}
	if floorTable != "" {
		args = append(args, floorPrefix)
	}
	args = append(args, fileFloor)
	if floorTable != "" {
		args = append(args, floorPrefix)
	}
	args = append(args, fileFloor)
	var number int64
	query := fmt.Sprintf(`INSERT INTO wb_number_sequence (site_code,kind,business_date,next_value)
VALUES (?,?,?,GREATEST(GREATEST(%[1]s,0),?)+1)
ON CONFLICT (site_code,kind,business_date) DO UPDATE SET next_value=GREATEST(wb_number_sequence.next_value+1,GREATEST(GREATEST(%[1]s,0),?)+1)
RETURNING next_value`, floor)
	if err := tx.Raw(query, args...).Scan(&number).Error; err != nil {
		return 0, err
	}
	// %04d is only a minimum width: serials expand naturally past 9999 and the
	// ticket_no column (VARCHAR(100)) bounds the physical length (ADR 0033).
	if err := raiseWatermark(dataDir, key, number); err != nil {
		return 0, err
	}
	return number, nil
}
func (r *Repository) SaveCategory(ctx context.Context, id int64, in CategoryInput, e audit.Event) (Category, error) {
	var v Category
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if id > 0 {
			v, err = lockRow[Category](tx, id)
			if err != nil {
				return err
			}
		}
		before := v
		v.MasterMeta, err = meta(v.MasterMeta, id, in.Version, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		v.Name = in.Name
		v.Enabled = enabled(in.Enabled)
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return masterEvent(ctx, tx, e, before, v)
	})
	return v, err
}
func (r *Repository) SaveMaterial(ctx context.Context, id int64, in MaterialInput, e audit.Event) (Material, error) {
	var v Material
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if id > 0 {
			v, err = lockRow[Material](tx, id)
			if err != nil {
				return err
			}
		}
		if in.CategoryID != nil {
			category, err := lockRow[Category](tx, *in.CategoryID)
			if err != nil {
				return err
			}
			if category.Enabled != 1 && (v.CategoryID == nil || *v.CategoryID != *in.CategoryID) {
				return ErrInvalid
			}
		}
		before := v
		v.MasterMeta, err = meta(v.MasterMeta, id, in.Version, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		if id == 0 {
			v.Code, err = nextNumber(tx, "material", "WL", r.dataDir, time.Now().UTC())
			if err != nil {
				return err
			}
		}
		v.Name = in.Name
		v.CategoryID = in.CategoryID
		v.Spec = in.Spec
		v.Remark = in.Remark
		v.Enabled = enabled(in.Enabled)
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return masterEvent(ctx, tx, e, before, v)
	})
	return v, err
}
func (r *Repository) SaveParty(ctx context.Context, id int64, in PartyInput, e audit.Event) (Party, error) {
	var v Party
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if id > 0 {
			v, err = lockRow[Party](tx, id)
			if err != nil {
				return err
			}
		}
		before := v
		v.MasterMeta, err = meta(v.MasterMeta, id, in.Version, e.Metadata.ActorID)
		if err != nil {
			return err
		}
		oldContacts := []Contact{}
		if id > 0 {
			if err := tx.Where("party_id=?", id).Find(&oldContacts).Error; err != nil {
				return err
			}
		}
		decorateParty(&before, oldContacts)
		for _, c := range in.Contacts {
			if c.ID > 0 && !slices.ContainsFunc(oldContacts, func(old Contact) bool { return old.ID == c.ID }) {
				return ErrInvalid
			}
		}
		if id == 0 {
			v.Code, err = nextNumber(tx, "party", "ZT", r.dataDir, time.Now().UTC())
			if err != nil {
				return err
			}
		}
		v.Name = in.Name
		v.ShortName = in.ShortName
		v.Nature = in.Nature
		v.Phone = in.Phone
		v.Address = in.Address
		v.CreditCode = in.CreditCode
		v.Remark = in.Remark
		v.Enabled = enabled(in.Enabled)
		v.IsSupplier = flag(slices.Contains(in.Roles, "supplier"))
		v.IsCustomer = flag(slices.Contains(in.Roles, "customer"))
		v.IsCarrier = flag(slices.Contains(in.Roles, "carrier"))
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		if err := tx.Where("party_id=?", v.ID).Delete(&Contact{}).Error; err != nil {
			return err
		}
		contacts := make([]Contact, 0, len(in.Contacts))
		for _, c := range in.Contacts {
			contacts = append(contacts, Contact{ID: c.ID, PartyID: v.ID, Name: c.Name, Phone: c.Phone, Purpose: c.Purpose, Remark: c.Remark, IsDefault: flag(c.IsDefault)})
		}
		if len(contacts) > 0 {
			if err := tx.Create(&contacts).Error; err != nil {
				return err
			}
		}
		decorateParty(&v, contacts)
		e.ResourceID = v.ID
		return masterEvent(ctx, tx, e, before, v)
	})
	return v, err
}
func masterModel(kind string) any {
	switch kind {
	case "category":
		return &Category{}
	case "material":
		return &Material{}
	case "party":
		return &Party{}
	default:
		return nil
	}
}
func (r *Repository) DeleteMaster(ctx context.Context, kind string, id, version int64, e audit.Event) error {
	model := masterModel(kind)
	if model == nil {
		return ErrInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ID, Version int64 }
		if err := tx.Model(model).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&row).Error; err != nil {
			return err
		}
		if row.Version != version {
			return ErrConflict
		}
		used, err := usedByBusiness(tx, kind, id)
		if err != nil {
			return err
		}
		if used {
			return ErrReferenced
		}
		if kind == "category" {
			var n int64
			if err := tx.Model(&Material{}).Where("category_id=?", id).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return ErrReferenced
			}
		}
		if kind == "party" {
			if err := tx.Where("party_id=?", id).Delete(&Contact{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Delete(model, id).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrReferenced
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
func (r *Repository) SetMasterStatus(ctx context.Context, kind string, id, version int64, value int, e audit.Event) error {
	model := masterModel(kind)
	if model == nil {
		return ErrInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(model).Where("id=? AND version=?", id, version).Updates(map[string]any{"enabled": value, "version": version + 1, "update_by": e.Metadata.ActorID, "update_time": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
