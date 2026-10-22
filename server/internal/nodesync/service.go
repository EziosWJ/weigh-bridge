package nodesync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"gorm.io/gorm"
)

type Service struct {
	db     *gorm.DB
	cfg    config.NodeConfig
	images string
	client *http.Client
	mu     sync.Mutex
}

func New(db *gorm.DB, cfg config.NodeConfig, images string) (*Service, error) {
	if err := Bind(db, cfg); err != nil {
		return nil, err
	}
	if cfg.Role == "central" {
		for _, peer := range cfg.Peers {
			if _, err := state(db, peer.SiteID); err != nil {
				return nil, err
			}
		}
	}
	return &Service{db: db, cfg: cfg, images: images, client: &http.Client{Timeout: 30 * time.Second}}, nil
}
func (s *Service) Role() string  { return s.cfg.Role }
func (s *Service) SiteID() int64 { return s.cfg.SiteID }
func (s *Service) Run(ctx context.Context) {
	if s.cfg.Role != "site" {
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			s.db.WithContext(ctx).Model(&State{}).Where("site_id=?", s.cfg.SiteID).Update("last_error", "中心连接或同步失败，将自动重试")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RefreshInitialized permanently enables a standalone site once local setup has
// at least two named, active users with permission to confirm weighing.
func (s *Service) RefreshInitialized(ctx context.Context) (bool, error) {
	if s.cfg.Role != "standalone" {
		id, err := loadIdentity(ctx, s.db)
		return id.Initialized == 1, err
	}
	var id Identity
	if err := s.db.WithContext(ctx).First(&id, 1).Error; err != nil {
		return false, err
	}
	if id.Initialized == 1 {
		return true, nil
	}
	var operators int64
	err := s.db.WithContext(ctx).Table("sys_user u").Joins("JOIN wb_user_site g ON g.user_id=u.id").Where("g.site_id=? AND g.can_operate=1 AND u.status=1 AND u.deleted=0 AND u.is_builtin=0 AND btrim(u.nickname)<>'' AND EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id JOIN sys_role_menu rm ON rm.role_id=r.id JOIN sys_menu m ON m.id=rm.menu_id WHERE ur.user_id=u.id AND r.status=1 AND r.deleted=0 AND m.status=1 AND m.deleted=0 AND m.permission_code='weighing:confirm')", s.cfg.SiteID).Distinct("u.id").Count(&operators).Error
	if err != nil {
		return false, err
	}
	if operators < 2 {
		return false, nil
	}
	if err := s.db.WithContext(ctx).Model(&Identity{}).Where("id=1 AND role='standalone' AND site_id=?", s.cfg.SiteID).Update("initialized", 1).Error; err != nil {
		return false, err
	}
	return true, nil
}
func (s *Service) snapshot(tx *gorm.DB, site int64, central bool) (map[string][]Row, error) {
	tables := append([]string{}, businessTables...)
	if central {
		tables = masterTables
	} else {
		tables = append([]string{"wb_party", "wb_party_contact", "wb_temporary_party", "wb_site_proposal"}, tables...)
	}
	out := map[string][]Row{}
	for _, table := range tables {
		var q *gorm.DB
		if central {
			switch table {
			case "sys_user":
				q = tx.Table(table).Where("id IN (SELECT user_id FROM wb_user_site WHERE site_id=?)", site)
			case "sys_user_role":
				q = tx.Table(table).Where("user_id IN (SELECT user_id FROM wb_user_site WHERE site_id=?)", site)
			case "wb_site", "wb_scale", "wb_site_proposal":
				column := "site_id"
				if table == "wb_site" {
					column = "id"
				}
				q = tx.Table(table).Where(column+"=?", site)
			case "wb_user_site":
				q = tx.Table(table).Where("site_id=?", site)
			case "wb_party":
				q = tx.Table(table).Where("id NOT IN (SELECT party_id FROM wb_temporary_party WHERE site_id<>?)", site)
			case "wb_party_contact":
				q = tx.Table(table).Where("party_id NOT IN (SELECT party_id FROM wb_temporary_party WHERE site_id<>?)", site)
			case "wb_temporary_party":
				q = tx.Table(table).Where("site_id=?", site)
			}
		} else {
			switch table {
			case "wb_party":
				q = tx.Table(table).Where("id IN (SELECT party_id FROM wb_temporary_party WHERE site_id=?)", site)
			case "wb_party_contact":
				q = tx.Table(table).Where("party_id IN (SELECT party_id FROM wb_temporary_party WHERE site_id=?)", site)
			case "wb_temporary_party", "wb_site_proposal":
				q = tx.Table(table).Where("site_id=?", site)
			}
		}
		rows, err := readRows(tx, table, q)
		if err != nil {
			return nil, err
		}
		if table == "sys_user" {
			for _, r := range rows {
				delete(r, "last_login_time")
				delete(r, "last_login_ip")
				delete(r, "update_time")
			}
		}
		out[table] = rows
	}
	return out, nil
}
func decode(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	return d.Decode(v)
}
func (s *Service) pending(ctx context.Context) (Packet, error) {
	var packet Packet
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		st, e := state(tx, s.cfg.SiteID)
		if e != nil {
			return e
		}
		if st.Pending != "" {
			return decode([]byte(st.Pending), &packet)
		}
		tables, e := s.snapshot(tx, s.cfg.SiteID, false)
		if e != nil {
			return e
		}
		changed, count, e := delta(tx, s.cfg.SiteID, tables)
		if e != nil {
			return e
		}
		if count == 0 {
			return nil
		}
		seq := time.Now().UnixMicro()
		if seq <= st.SentSeq {
			seq = st.SentSeq + 1
		}
		// Keep task graphs coherent while bounding catch-up work to 50 tasks per pass.
		selected := map[int64]bool{}
		for _, table := range businessTables {
			for _, r := range changed[table] {
				id := number(r["task_id"])
				if table == "wb_transport_task" {
					id = number(r["id"])
				}
				if id > 0 && len(selected) < 50 {
					selected[id] = true
				}
			}
		}
		upload := map[string][]Row{}
		for table, rows := range tables {
			if contains(businessTables, table) {
				for _, r := range rows {
					id := number(r["task_id"])
					if table == "wb_transport_task" {
						id = number(r["id"])
					}
					if selected[id] {
						upload[table] = append(upload[table], r)
					}
				}
				for _, r := range changed[table] {
					if number(r["task_id"]) == 0 && table != "wb_transport_task" {
						upload[table] = append(upload[table], r)
					}
				}
			} else {
				upload[table] = rows
			}
		}
		packet = Packet{Sequence: seq, Tables: upload}
		data, e := json.Marshal(packet)
		if e != nil {
			return e
		}
		return tx.Model(&State{}).Where("site_id=?", s.cfg.SiteID).Updates(map[string]any{"pending": string(data), "sent_seq": seq}).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return packet, err
}
func (s *Service) request(ctx context.Context, path string, body any, response any) error {
	data, e := json.Marshal(body)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.CentralURL, "/")+path, bytes.NewReader(data))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.SyncKey)
	req.Header.Set("X-Site-ID", strconv.FormatInt(s.cfg.SiteID, 10))
	res, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("sync HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	d := json.NewDecoder(io.LimitReader(res.Body, 64<<20))
	if e = d.Decode(&envelope); e != nil {
		return e
	}
	if envelope.Code != 200 {
		return errors.New("sync rejected")
	}
	if response != nil {
		return decode(envelope.Data, response)
	}
	return nil
}

// Sync transfers persisted business state first. Images are retried separately;
// only the sender initiates communication, and no business commands are replayed.
func (s *Service) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Role != "site" {
		return errors.New("only site nodes synchronize")
	}
	p, err := s.pending(ctx)
	if err != nil {
		return err
	}
	var down Packet
	if err = s.request(ctx, "/api/node/exchange", p, &down); err != nil {
		return err
	}
	if err = s.applyMasters(ctx, down); err != nil {
		return err
	}
	if p.Sequence != 0 {
		if err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := acknowledge(tx, s.cfg.SiteID, p.Tables); err != nil {
				return err
			}
			return tx.Model(&State{}).Where("site_id=? AND sent_seq=?", s.cfg.SiteID, p.Sequence).Update("pending", "").Error
		}); err != nil {
			return err
		}
	}
	if err = s.db.WithContext(ctx).Model(&State{}).Where("site_id=?", s.cfg.SiteID).Updates(map[string]any{"last_success": time.Now().UTC().Format(time.RFC3339Nano), "last_error": ""}).Error; err != nil {
		return err
	}
	return s.syncImages(ctx)
}
func number(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
func (s *Service) receive(ctx context.Context, site int64, p Packet) (Packet, error) {
	var out Packet
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		st, e := state(tx, site)
		if e != nil {
			return e
		}
		if p.Sequence > st.ReceivedSeq {
			if e = s.applyBusiness(tx, site, p); e != nil {
				return e
			}
			if e = tx.Model(&State{}).Where("site_id=?", site).Updates(map[string]any{"received_seq": p.Sequence}).Error; e != nil {
				return e
			}
		}
		tables, e := s.snapshot(tx, site, true)
		if e != nil {
			return e
		}
		out = Packet{Sequence: time.Now().UnixMicro(), Tables: tables}
		return tx.Model(&State{}).Where("site_id=?", site).Updates(map[string]any{"last_success": time.Now().UTC().Format(time.RFC3339Nano), "last_error": ""}).Error
	})
	return out, err
}
func (s *Service) applyBusiness(tx *gorm.DB, site int64, p Packet) error {
	allowed := append(append([]string{}, businessTables...), "wb_party", "wb_party_contact", "wb_temporary_party", "wb_site_proposal")
	for table, rows := range p.Tables {
		if !contains(allowed, table) {
			return errors.New("unsupported upstream table")
		}
		for _, r := range rows {
			if table == "wb_site_proposal" {
				if number(r["site_id"]) != site {
					return errors.New("proposal ownership mismatch")
				}
				continue
			}
			id := number(r["id"])
			if table == "wb_temporary_party" {
				id = number(r["party_id"])
			}
			if id <= site*idBlock || id >= (site+1)*idBlock {
				return errors.New("upstream ID outside site namespace")
			}
			if v, ok := r["site_id"]; ok && number(v) != site {
				return errors.New("upstream ownership mismatch")
			}
			if v, ok := r["scale_id"]; ok {
				var count int64
				if e := tx.Table("wb_scale").Where("id=? AND site_id=?", v, site).Count(&count).Error; e != nil {
					return e
				}
				if count != 1 {
					return errors.New("upstream scale mismatch")
				}
			}
			for _, key := range []string{"task_id", "ticket_id", "sample_id", "operation_id", "party_id", "current_first_record_id", "current_second_record_id", "target_record_id", "replaces_record_id", "retained_record_id", "first_record_id", "second_record_id", "previous_ticket_id"} {
				if v, ok := r[key]; ok && v != nil {
					n := number(v)
					if key == "party_id" && table == "wb_transport_task" {
						continue
					}
					if n <= site*idBlock || n >= (site+1)*idBlock {
						return errors.New("upstream reference outside site namespace")
					}
				}
			}
		}
	}
	for _, row := range p.Tables["wb_site_proposal"] {
		var proposal Row
		if err := tx.Table("wb_site_proposal").Where("site_id=?", site).Take(&proposal).Error; err != nil {
			return err
		}
		if number(row["applied_revision"]) == number(proposal["revision"]) && number(proposal["applied_revision"]) < number(proposal["revision"]) {
			var content struct {
				Input struct {
					Name      string
					Enabled   int
					ServerURL string
					ScaleName string
				}
				ScaleID int64
			}
			if json.Unmarshal([]byte(fmt.Sprint(proposal["configuration"])), &content) != nil {
				return errors.New("invalid proposal")
			}
			if err := tx.Table("wb_site").Where("id=?", site).Updates(map[string]any{"name": content.Input.Name, "enabled": content.Input.Enabled, "server_url": content.Input.ServerURL}).Error; err != nil {
				return err
			}
			if err := tx.Table("wb_scale").Where("site_id=?", site).Updates(map[string]any{"name": content.Input.ScaleName, "enabled": content.Input.Enabled}).Error; err != nil {
				return err
			}
			if err := tx.Table("wb_site_proposal").Where("site_id=?", site).Update("applied_revision", number(proposal["revision"])).Error; err != nil {
				return err
			}
		}
	}
	// Site-owned temporary submissions never overwrite a central review. A newer
	// revision records supplemental data; final decisions remain central-owned.
	editable := map[int64]bool{}
	for _, r := range p.Tables["wb_temporary_party"] {
		id := number(r["party_id"])
		var old Row
		found := tx.Table("wb_temporary_party").Where("party_id=?", id).Take(&old).Error
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		if found != nil || number(r["revision"]) > number(old["revision"]) {
			stateName := fmt.Sprint(r["state"])
			if found != nil && stateName != "pending" {
				return errors.New("invalid initial temporary state")
			}
			if found == nil && stateName != "resubmitted" {
				return errors.New("invalid supplemental state")
			}
			r["reviewed_revision"] = int64(0)
			r["formal_party_id"] = nil
			r["reason"] = ""
			editable[id] = true
		}
	}
	for id := range editable {
		if e := tx.Exec("DELETE FROM wb_party_contact WHERE party_id=?", id).Error; e != nil {
			return e
		}
	}
	for _, table := range []string{"wb_party", "wb_party_contact", "wb_temporary_party"} {
		for _, r := range p.Tables[table] {
			id := number(r["id"])
			if table != "wb_party" {
				id = number(r["party_id"])
			}
			if !editable[id] {
				continue
			}
			if e := upsert(tx, table, r); e != nil {
				return e
			}
		}
	}
	// A newer transport envelope cannot make an older restored business history
	// authoritative. Preserve central facts and require explicit reconciliation.
	for _, incoming := range p.Tables["wb_transport_task"] {
		var existing Row
		err := tx.Table("wb_transport_task").Where("id=?", incoming["id"]).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if existing != nil {
			if number(existing["version"]) > number(incoming["version"]) {
				return errors.New("restored task history is older than the received business version")
			}
			if number(existing["version"]) == number(incoming["version"]) {
				basis := func(row Row) Row {
					v := Row{}
					for _, key := range []string{"status", "plate", "direction", "material_id", "party_id", "carrier_id", "current_first_record_id", "current_second_record_id"} {
						v[key] = row[key]
					}
					var business any
					_ = decode([]byte(fmt.Sprint(row["business_snapshot"])), &business)
					v["business"] = business
					return v
				}
				if hash(basis(existing)) != hash(basis(incoming)) {
					return errors.New("conflicting business content at the same task version")
				}
			}
		}
	}
	for _, incoming := range p.Tables["wb_ticket"] {
		var old struct{ Status string }
		if err := tx.Table("wb_ticket").Where("id=?", incoming["id"]).Find(&old).Error; err != nil {
			return err
		}
		if old.Status == "void" && incoming["status"] == "valid" {
			return errors.New("voided ticket cannot become valid through replication")
		}
	}
	for _, incoming := range p.Tables["wb_print_attempt"] {
		var old struct{ Status string }
		if err := tx.Table("wb_print_attempt").Where("id=?", incoming["id"]).Find(&old).Error; err != nil {
			return err
		}
		if (old.Status == "printed" || old.Status == "failed") && (incoming["status"] == "initiated" || incoming["status"] == "unknown") {
			return errors.New("resolved print history cannot be reopened through replication")
		}
	}
	// Break the task/record cycle while inserting a coherent snapshot. Readers
	// observe only the committed transaction with restored current-record links.
	for _, r := range p.Tables["wb_transport_task"] {
		copy := clone(r)
		copy["current_first_record_id"] = nil
		copy["current_second_record_id"] = nil
		if e := upsert(tx, "wb_transport_task", copy); e != nil {
			return e
		}
	}
	for _, r := range p.Tables["wb_sample"] {
		copy := clone(r)
		copy["target_record_id"] = nil
		if e := upsert(tx, "wb_sample", copy); e != nil {
			return e
		}
	}
	for _, table := range []string{"wb_weigh_record", "wb_transport_task", "wb_sample", "wb_ticket", "wb_operation", "wb_business_event", "wb_print_attempt"} {
		rows := p.Tables[table]
		sort.SliceStable(rows, func(i, j int) bool { return number(rows[i]["id"]) < number(rows[j]["id"]) })
		for _, r := range rows {
			if table == "wb_operation" {
				r = clone(r)
				original := fmt.Sprint(r["request_id"])
				r["origin_request_id"] = original
				r["request_id"] = fmt.Sprintf("%d:%s", site, original)
			}
			if e := upsert(tx, table, r); e != nil {
				return e
			}
		}
	}
	return nil
}
func clone(r Row) Row {
	v := Row{}
	for k, x := range r {
		v[k] = x
	}
	return v
}
func (s *Service) applyMasters(ctx context.Context, p Packet) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for t := range p.Tables {
			if !contains(masterTables, t) {
				return errors.New("unsupported downstream table")
			}
		}
		// The central authority is limited to accounts assigned to this site. Deleted
		// grants revoke local access and sessions, including already logged-in users.
		old, e := s.snapshot(tx, s.cfg.SiteID, true)
		if e != nil {
			return e
		}
		security := func(tables map[string][]Row) string {
			v := map[string][]Row{}
			for _, t := range []string{"sys_user", "sys_role", "sys_menu", "sys_user_role", "sys_role_menu", "wb_user_site"} {
				v[t] = tables[t]
			}
			return hash(v)
		}
		if security(old) != security(p.Tables) {
			if e := tx.Exec("UPDATE auth_session SET revoked_at=? WHERE revoked_at IS NULL", time.Now().UTC()).Error; e != nil {
				return e
			}
		}
		for _, t := range []string{"sys_user_role", "sys_role_menu", "wb_user_site"} {
			if e := tx.Exec("DELETE FROM " + t).Error; e != nil {
				return e
			}
		}
		// Keep historical FK targets when a master is deleted at the center.
		for _, t := range []string{"wb_material_category", "wb_material"} {
			ids := []int64{}
			for _, r := range p.Tables[t] {
				ids = append(ids, number(r["id"]))
			}
			q := tx.Table(t).Where("1=1")
			if len(ids) > 0 {
				q = q.Where("id NOT IN ?", ids)
			}
			if e := q.Update("enabled", 0).Error; e != nil {
				return e
			}
		}
		userIDs := []int64{}
		for _, r := range p.Tables["sys_user"] {
			userIDs = append(userIDs, number(r["id"]))
		}
		q := tx.Table("sys_user").Where("1=1")
		if len(userIDs) > 0 {
			q = q.Where("id NOT IN ?", userIDs)
		}
		if e := q.Update("status", 0).Error; e != nil {
			return e
		}
		officialIDs := []int64{}
		for _, party := range p.Tables["wb_party"] {
			id := number(party["id"])
			officialIDs = append(officialIDs, id)
			var local Row
			_ = tx.Table("wb_temporary_party").Where("party_id=?", id).Take(&local).Error
			remoteRevision := int64(0)
			for _, r := range p.Tables["wb_temporary_party"] {
				if number(r["party_id"]) == id {
					remoteRevision = number(r["revision"])
				}
			}
			if local != nil && number(local["revision"]) > remoteRevision {
				continue
			}
			if err := tx.Exec("DELETE FROM wb_party_contact WHERE party_id=?", id).Error; err != nil {
				return err
			}
		}
		removed := tx.Table("wb_party").Where("id NOT IN (SELECT party_id FROM wb_temporary_party)")
		if len(officialIDs) > 0 {
			removed = removed.Where("id NOT IN ?", officialIDs)
		}
		if err := removed.Update("enabled", 0).Error; err != nil {
			return err
		}
		for _, table := range masterTables {
			for _, r := range p.Tables[table] {
				// Running site/device configuration is changed only through local Apply.
				if table == "wb_site" || table == "wb_scale" {
					var n int64
					if e := tx.Table(table).Where("id=?", r["id"]).Count(&n).Error; e != nil {
						return e
					}
					if n > 0 {
						continue
					}
				}
				if table == "wb_site_proposal" {
					var old Row
					if e := tx.Table(table).Where("site_id=?", s.cfg.SiteID).Take(&old).Error; e == nil {
						r = clone(r)
						r["applied_revision"] = old["applied_revision"]
					}
				}
				if table == "wb_temporary_party" {
					var old Row
					if e := tx.Table(table).Where("party_id=?", r["party_id"]).Take(&old).Error; e == nil && number(old["revision"]) > number(r["revision"]) {
						continue
					}
				}
				if table == "wb_party" || table == "wb_party_contact" {
					id := number(r["id"])
					if table == "wb_party_contact" {
						id = number(r["party_id"])
					}
					var tmp Row
					if e := tx.Table("wb_temporary_party").Where("party_id=?", id).Take(&tmp).Error; e == nil {
						var remote Row
						for _, v := range p.Tables["wb_temporary_party"] {
							if number(v["party_id"]) == id {
								remote = v
							}
						}
						if remote == nil || number(tmp["revision"]) > number(remote["revision"]) {
							continue
						}
					}
				}
				if e := upsert(tx, table, r); e != nil {
					return e
				}
			}
		}
		for _, table := range []string{"wb_material_category", "wb_material", "wb_party", "wb_party_contact"} {
			if e := tx.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), GREATEST(COALESCE((SELECT MAX(id) FROM "+table+"),0),1))", table).Error; e != nil {
				return e
			}
		}
		// Legacy site activation remains tied to the central exchange. Standalone
		// activation is evaluated from local account and RBAC state.
		if s.cfg.Role == "site" {
			var operators int64
			if e := tx.Table("sys_user u").Joins("JOIN wb_user_site g ON g.user_id=u.id").Where("g.site_id=? AND g.can_operate=1 AND u.status=1 AND u.deleted=0 AND u.nickname<>'' AND EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id JOIN sys_role_menu rm ON rm.role_id=r.id JOIN sys_menu m ON m.id=rm.menu_id WHERE ur.user_id=u.id AND r.status=1 AND r.deleted=0 AND m.status=1 AND m.deleted=0 AND m.permission_code='weighing:confirm')", s.cfg.SiteID).Distinct("u.id").Count(&operators).Error; e != nil {
				return e
			}
			if operators >= 2 {
				return tx.Model(&Identity{}).Where("id=1").Update("initialized", 1).Error
			}
		}
		return nil
	})
}

var imageID = regexp.MustCompile(`^[0-9]+_[a-f0-9]{32}_(front|rear)\.(png|jpg)$`)

// Referenced images only; stale Agent previews do not enlarge the sync backlog.
func (s *Service) imageEvidence(ctx context.Context, site int64) (map[string]string, error) {
	rows := []Row{}
	if err := s.db.WithContext(ctx).Table("wb_weigh_record r").Select("r.evidence_snapshot").Joins("JOIN wb_transport_task t ON t.id=r.task_id").Where("t.site_id=?", site).Find(&rows).Error; err != nil {
		return nil, err
	}
	images := map[string]string{}
	for _, r := range rows {
		var value any
		str := fmt.Sprint(r["evidence_snapshot"])
		if b, ok := r["evidence_snapshot"].([]byte); ok {
			str = string(b)
		}
		if err := json.Unmarshal([]byte(str), &value); err != nil {
			return nil, err
		}
		var walk func(any)
		walk = func(x any) {
			switch v := x.(type) {
			case map[string]any:
				if id, ok := v["id"].(string); ok && imageID.MatchString(id) {
					if digest, ok := v["sha256"].(string); ok {
						images[id] = digest
					}
				}
				for _, item := range v {
					walk(item)
				}
			case []any:
				for _, item := range v {
					walk(item)
				}
			}
		}
		walk(value)
	}
	return images, nil
}
func (s *Service) imageManifest(ctx context.Context, site int64) ([]string, error) {
	images, err := s.imageEvidence(ctx, site)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for id := range images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
func (s *Service) missingImages(ctx context.Context, site int64) ([]string, error) {
	ids, e := s.imageManifest(ctx, site)
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, id := range ids {
		if _, e := os.Stat(filepath.Join(s.images, id)); e != nil {
			out = append(out, id)
		}
	}
	return out, nil
}
func (s *Service) syncImages(ctx context.Context) error {
	var missing []string
	if e := s.request(ctx, "/api/node/images/missing", nil, &missing); e != nil {
		return e
	}
	if e := s.db.WithContext(ctx).Model(&State{}).Where("site_id=?", s.cfg.SiteID).Update("pending_images", len(missing)).Error; e != nil {
		return e
	}
	// Bound each iteration so continued production and master updates get priority.
	for i, id := range missing {
		if i >= 8 {
			break
		}
		if !imageID.MatchString(id) {
			return errors.New("invalid image ID")
		}
		data, e := os.ReadFile(filepath.Join(s.images, id))
		if e != nil {
			return e
		}
		if e = s.request(ctx, "/api/node/images", map[string]any{"id": id, "data": data}, nil); e != nil {
			return e
		}
		if e = s.db.WithContext(ctx).Model(&State{}).Where("site_id=?", s.cfg.SiteID).Update("pending_images", len(missing)-i-1).Error; e != nil {
			return e
		}
	}
	return nil
}
