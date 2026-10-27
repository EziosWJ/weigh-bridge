//go:build integration

package integration

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"github.com/pressly/goose/v3"
)

// TestPostgresCodeIdentityMigrationUpgradesPopulatedDatabase applies ADR 0033's
// code-identity migration to a database that is still on the numeric identity
// schema but already holds business rows. It verifies that every historical row
// keeps its surrogate id and issued number, that the code backfill matches the
// former site/scale rows, and that new business can still be written afterwards.
func TestPostgresCodeIdentityMigrationUpgradesPopulatedDatabase(t *testing.T) {
	temporary := startPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpToContext(ctx, database.SQL, schema, 20); err != nil {
		t.Fatalf("schema to 20: %v", err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpToContext(ctx, database.SQL, seed, 17); err != nil {
		t.Fatalf("seed to 17: %v", err)
	}
	insertLegacyIdentityRows(t, database)

	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpContext(ctx, database.SQL, schema); err != nil {
		t.Fatalf("apply code identity migration: %v", err)
	}

	checkLegacyRowsPreserved(t, database)
	var access struct {
		UserID     int64
		CanOperate int
	}
	if err := database.GORM.Table("wb_local_user_access").Where("user_id=1").Take(&access).Error; err != nil || access.UserID != 1 || access.CanOperate != 1 {
		t.Fatalf("local account grant was not preserved: %+v err=%v", access, err)
	}
	checkNumericColumnsRemoved(t, database)
	checkCodeIdentityConstraints(t, database)
	checkNewNumberingAfterMigration(t, database)
}

// TestPostgresCodeIdentityMigrationDoesNotInventIdentity covers a database that
// has business rows but no wb_local_identity row. The migration must not infer
// an owner: it may only leave the identity empty so first-time configuration
// refuses to rebuild over existing business data.
func TestPostgresCodeIdentityMigrationDoesNotInventIdentity(t *testing.T) {
	temporary := startPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpToContext(ctx, database.SQL, schema, 20); err != nil {
		t.Fatalf("schema to 20: %v", err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpToContext(ctx, database.SQL, seed, 17); err != nil {
		t.Fatalf("seed to 17: %v", err)
	}
	insertLegacySiteScaleAndHistory(t, database, false)
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpContext(ctx, database.SQL, schema); err != nil {
		t.Fatalf("migration must complete without an identity row and without creating one: %v", err)
	}
	rows := legacyIdentityCodes(t, database)
	if rows.SiteCode != "" || rows.ScaleCode != "" {
		t.Fatalf("migration invented an identity: %+v", rows)
	}
	deps := testDependencies(t, database, t.TempDir())
	deps.Weighing = weighing.NewService(weighing.NewRepository(database.GORM), agentbridge.NewMultiple(nil))
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	response := serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", `{"siteCode":"F001","scaleCode":"WB01"}`, admin)
	assertEnvelopeCode(t, response, http.StatusOK, http.StatusBadRequest, "缺少本厂身份但已有业务数据（wb_site），请检查数据库与备份，不可自动重建身份")
}

func insertLegacyIdentityRows(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	insertLegacySiteScaleAndHistory(t, database, true)
}

// insertLegacySiteScaleAndHistory writes rows in the old numeric identity shape.
// withIdentity controls whether wb_local_identity carries the numeric site id.
func insertLegacySiteScaleAndHistory(t *testing.T, database *platformdatabase.Database, withIdentity bool) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := database.GORM.Exec(query, args...).Error; err != nil {
			t.Fatalf("legacy insert failed: %s: %v", query, err)
		}
	}
	exec(`INSERT INTO wb_site (id,code,name,enabled,version,server_url) VALUES (1,'SITE-01','一厂',1,1,'')`)
	exec(`INSERT INTO wb_scale (id,code,name,enabled,site_id) VALUES (1,'WB01','一号地磅',1,1)`)
	if withIdentity {
		exec(`INSERT INTO wb_local_identity (id,site_id,next_id,initialized) VALUES (1,1,1000,1)`)
	}
	exec(`INSERT INTO wb_user_site (user_id,site_id,can_operate) VALUES (1,1,1)`)
	exec(`INSERT INTO wb_agent_binding (scale_id,agent_id,agent_key) VALUES (1,'agent-legacy','legacy-key')`)
	exec(`INSERT INTO wb_scale_session (scale_id,generation,version,conditions_version) VALUES (1,3,2,4)`)
	exec(`INSERT INTO wb_material_category (id,name,create_by,update_by) VALUES (1,'酿酒原料',1,1)`)
	exec(`INSERT INTO wb_material (id,code,name,category_id,create_by,update_by) VALUES (1,'WL-LEGACY','高粱',1,1,1)`)
	exec(`INSERT INTO wb_party (id,code,name,nature,is_supplier,create_by,update_by) VALUES (1,'ZT-LEGACY','供应商甲','enterprise',1,1,1)`)
	exec(`INSERT INTO wb_number_sequence (kind,business_date,next_value) VALUES ('material','20261001',5)`)
	exec(`INSERT INTO wb_transport_task (id,task_no,mode,plate,direction,status,scale_id,site_id,material_id,party_id,business_snapshot,version,create_time,update_time,actor_id,actor_name)
VALUES (1,'RW-SITE-01-legacy','normal','晋A12345','inbound','completed',1,1,1,1,'{"plate":"晋A12345"}',1,?,?,1,'管理员')`, now, now)
	for _, sample := range []struct {
		id     int
		stage  string
		kind   string
		weight int
	}{
		{1, "first", "gross", 30000},
		{2, "second", "tare", 10000},
	} {
		exec(`INSERT INTO wb_sample (id,scale_id,owner_user_id,client_id,session_generation,task_id,business_hash,business_snapshot,stage,kind,plate,source,weight_milli_tonne,captured_at,expires_at,stable,device_ok,evidence_snapshot,plate_evidence,conditions_version)
VALUES (?,1,1,'legacy-browser',3,1,'legacy-hash','{}',?,?,'晋A12345','simulated',?,?,NULL,1,1,'{}','{}',4)`,
			sample.id, sample.stage, sample.kind, sample.weight, now)
		exec(`INSERT INTO wb_weigh_record (id,task_id,sample_id,stage,kind,weight_milli_tonne,captured_at,confirmed_at,scale_id,actor_id,actor_name,evidence_snapshot,plate_evidence)
VALUES (?,1,?,?,?,?,?,?,1,1,'管理员','{}','{}')`, sample.id, sample.id, sample.stage, sample.kind, sample.weight, now, now)
	}
	exec(`UPDATE wb_transport_task SET current_first_record_id=1,current_second_record_id=2 WHERE id=1`)
	exec(`INSERT INTO wb_ticket (id,ticket_no,task_id,revision,mode,status,first_record_id,second_record_id,business_snapshot,basis_snapshot,original_gross_milli_tonne,original_tare_milli_tonne,final_gross_milli_tonne,final_tare_milli_tonne,gross_source,tare_source,issued_at,issued_by,issuer_name,site_id,site_name,ticket_title)
VALUES (1,'GB-SITE-01-legacyhex',1,1,'normal','valid',1,2,'{}','{}',30000,10000,30000,10000,'collected','collected',?,1,'管理员',1,'一厂','酒厂过磅单')`, now)
	exec(`INSERT INTO wb_weighing_image (id,scale_id,source,status,captured_at) VALUES ('1_legacyimage_front.png',1,'agent','captured',?)`, now)
	exec(`INSERT INTO wb_agent_config_backup (id,scale_id,config_version,digest,settings,applied_at) VALUES (1,1,7,'legacy-digest','{}',?)`, now)
	exec(`INSERT INTO wb_agent_runtime (scale_id,public_id,connected) VALUES (1,'legacy-runtime',0)`)
	exec(`INSERT INTO wb_print_attempt (id,ticket_id,request_id,attempt_no,intent,status,initiated_at,initiated_by) VALUES (1,1,'legacy-print-request',1,'initial','printed',?,1)`, now)
}

func checkLegacyRowsPreserved(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	var task struct {
		ID        int64
		TaskNo    string
		SiteCode  string
		ScaleCode string
	}
	if err := database.GORM.Raw(`SELECT id,task_no,site_code,scale_code FROM wb_transport_task WHERE id=1`).Scan(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.ID != 1 || task.TaskNo != "RW-SITE-01-legacy" || task.SiteCode != "SITE-01" || task.ScaleCode != "WB01" {
		t.Fatalf("task identity changed: %+v", task)
	}
	var ticket struct {
		ID       int64
		TicketNo string
		SiteCode string
		SiteName string
	}
	if err := database.GORM.Raw(`SELECT id,ticket_no,site_code,site_name FROM wb_ticket WHERE id=1`).Scan(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.ID != 1 || ticket.TicketNo != "GB-SITE-01-legacyhex" || ticket.SiteCode != "SITE-01" || ticket.SiteName != "一厂" {
		t.Fatalf("issued ticket changed: %+v", ticket)
	}
	for _, check := range []struct {
		table string
		query string
		want  string
	}{
		{"wb_sample", `SELECT scale_code FROM wb_sample WHERE id=1`, "WB01"},
		{"wb_weigh_record", `SELECT scale_code FROM wb_weigh_record WHERE id=1`, "WB01"},
		{"wb_weighing_image", `SELECT scale_code FROM wb_weighing_image WHERE id='1_legacyimage_front.png'`, "WB01"},
		{"wb_agent_config_backup", `SELECT scale_code FROM wb_agent_config_backup WHERE id=1`, "WB01"},
		{"wb_agent_runtime", `SELECT scale_code FROM wb_agent_runtime WHERE public_id='legacy-runtime'`, "WB01"},
		{"wb_agent_binding", `SELECT scale_code FROM wb_agent_binding WHERE agent_id='agent-legacy'`, "WB01"},
		{"wb_scale_session", `SELECT scale_code FROM wb_scale_session LIMIT 1`, "WB01"},
		{"wb_user_site", `SELECT site_code FROM wb_user_site WHERE user_id=1`, "SITE-01"},
		{"wb_number_sequence", `SELECT site_code FROM wb_number_sequence WHERE kind='material' AND business_date='20261001'`, "SITE-01"},
	} {
		var got string
		if err := database.GORM.Raw(check.query).Scan(&got).Error; err != nil {
			t.Fatalf("%s: %v", check.table, err)
		}
		if got != check.want {
			t.Fatalf("%s kept %q, want %q", check.table, got, check.want)
		}
	}
	identity := legacyIdentityCodes(t, database)
	if identity.SiteCode != "SITE-01" || identity.ScaleCode != "WB01" {
		t.Fatalf("local identity codes: %+v", identity)
	}
	var counts struct{ Samples, Records, Images, PrintAttempts int64 }
	if err := database.GORM.Raw(`SELECT (SELECT COUNT(*) FROM wb_sample) AS samples,(SELECT COUNT(*) FROM wb_weigh_record) AS records,(SELECT COUNT(*) FROM wb_weighing_image) AS images,(SELECT COUNT(*) FROM wb_print_attempt) AS print_attempts`).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Samples != 2 || counts.Records != 2 || counts.Images != 1 || counts.PrintAttempts != 1 {
		t.Fatalf("historical rows were lost: %+v", counts)
	}
	var sequence struct {
		NextValue int64
	}
	if err := database.GORM.Raw(`SELECT next_value FROM wb_number_sequence WHERE kind='material' AND business_date='20261001'`).Scan(&sequence).Error; err != nil || sequence.NextValue != 5 {
		t.Fatalf("sequence value changed: %+v err=%v", sequence, err)
	}
}

func legacyIdentityCodes(t *testing.T, database *platformdatabase.Database) struct{ SiteCode, ScaleCode string } {
	t.Helper()
	var row struct{ SiteCode, ScaleCode string }
	if err := database.GORM.Raw(`SELECT COALESCE(site_code,'') AS site_code,COALESCE(scale_code,'') AS scale_code FROM wb_local_identity WHERE id=1`).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	return struct{ SiteCode, ScaleCode string }{row.SiteCode, row.ScaleCode}
}

func checkNumericColumnsRemoved(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	var remaining []string
	if err := database.GORM.Raw(`SELECT table_name||'.'||column_name FROM information_schema.columns WHERE table_schema='public' AND column_name IN ('site_id','scale_id') AND table_name LIKE 'wb_%' ORDER BY 1`).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("numeric identity columns remain: %v", remaining)
	}
	var siteID, scaleID int64
	if err := database.GORM.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND ((table_name='wb_site' AND column_name='id') OR (table_name='wb_scale' AND column_name='id'))`).Scan(&siteID).Error; err != nil {
		t.Fatal(err)
	}
	if siteID != 0 {
		t.Fatalf("wb_site.id or wb_scale.id still exists: %d", siteID)
	}
	if err := database.GORM.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='wb_local_identity' AND column_name='site_id'`).Scan(&scaleID).Error; err != nil || scaleID != 0 {
		t.Fatalf("wb_local_identity.site_id still exists: %d err=%v", scaleID, err)
	}
}

func checkCodeIdentityConstraints(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	// wb_scale is keyed by (site_code, code); wb_number_sequence by
	// (site_code, kind, business_date); the active-plate index covers the single local factory.
	var scalePK, sequencePK []string
	if err := database.GORM.Raw(`SELECT a.attname FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=ANY(i.indkey) WHERE c.relname='wb_scale' AND i.indisprimary ORDER BY array_position(i.indkey,a.attnum)`).Scan(&scalePK).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(scalePK, ",") != "site_code,code" {
		t.Fatalf("wb_scale primary key=%v", scalePK)
	}
	if err := database.GORM.Raw(`SELECT a.attname FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=ANY(i.indkey) WHERE c.relname='wb_number_sequence' AND i.indisprimary ORDER BY array_position(i.indkey,a.attnum)`).Scan(&sequencePK).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(sequencePK, ",") != "site_code,kind,business_date" {
		t.Fatalf("wb_number_sequence primary key=%v", sequencePK)
	}
	var definition string
	if err := database.GORM.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname='public' AND indexname='uk_wb_task_active_plate'`).Scan(&definition).Error; err != nil || !strings.Contains(definition, "(plate)") {
		t.Fatalf("active plate index: %q err=%v", definition, err)
	}
}

// checkNewNumberingAfterMigration runs a complete formal weighing on the
// migrated database. The new ticket number must use the code identity and must
// not collide with the migrated legacy number.
func checkNewNumberingAfterMigration(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	deps := testDependencies(t, database, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	imageDir := t.TempDir()
	repository := weighing.NewRepository(database.GORM)
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repository.AgentConfig(ctx, scaleCode, imageDir)
	})
	deps.Weighing = weighing.NewService(repository, hub)
	deps.Camera, _, _ = newCaptureService(t, database, t.TempDir())
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	addCaptureCameras(t, database, "rtsp://camera/migrated-1")
	agentConfig, err := repository.AgentConfig(context.Background(), "WB01", imageDir)
	if err != nil {
		t.Fatalf("agent binding after migration: %v", err)
	}
	device := startLocalSiteMock(t, router, hub, "WB01", agentConfig.ID, agentConfig.Key)
	device.weight("30")
	ticket := localSiteRoundTrip(t, router, admin, admin, "SITE-01", "WB01", "晋A99999", device)
	date := time.Now().UTC().Add(8 * time.Hour).Format("20060102")
	if !regexp.MustCompile(`^SITE-01-` + date + `-\d{4,}$`).MatchString(ticket.TicketNo) {
		t.Fatalf("new ticket number %q does not use the code identity", ticket.TicketNo)
	}
	if ticket.TicketNo == "GB-SITE-01-legacyhex" {
		t.Fatalf("new ticket reused the legacy number")
	}
	var legacy int64
	if err := database.GORM.Raw(`SELECT COUNT(*) FROM wb_ticket WHERE ticket_no='GB-SITE-01-legacyhex'`).Scan(&legacy).Error; err != nil || legacy != 1 {
		t.Fatalf("legacy ticket number was rewritten: count=%d err=%v", legacy, err)
	}
}

// TestPostgresCodeIdentityMigrationUpgradesPreSitesDatabase reproduces an
// installation that stopped at the early weighing release: seed 00006 created
// SCALE01 with the default numeric site id before any wb_site row existed.
// Schema migrations run before seed migrations in "migrate up --kind all", so
// 00021 must clear that unreferenced placeholder instead of failing at the
// site_code NOT NULL step, and the upgraded database must still accept the
// first-time code identity.
func TestPostgresCodeIdentityMigrationUpgradesPreSitesDatabase(t *testing.T) {
	temporary := startPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = database.Close() })
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	bootstrapPreSites := func() {
		t.Helper()
		if err := goose.SetDialect("postgres"); err != nil {
			t.Fatal(err)
		}
		goose.SetTableName("goose_schema_db_version")
		if err := goose.UpToContext(context.Background(), database.SQL, schema, 12); err != nil {
			t.Fatalf("schema to 12: %v", err)
		}
		goose.SetTableName("goose_seed_db_version")
		if err := goose.UpToContext(context.Background(), database.SQL, seed, 6); err != nil {
			t.Fatalf("seed to 6: %v", err)
		}
		var placeholder int64
		if err := database.GORM.Table("wb_scale").Count(&placeholder).Error; err != nil || placeholder != 1 {
			t.Fatalf("pre-sites placeholder missing: count=%d err=%v", placeholder, err)
		}
		if err := database.GORM.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='wb_site'`).Scan(&placeholder).Error; err != nil || placeholder != 0 {
			t.Fatalf("pre-sites database unexpectedly has wb_site: count=%d err=%v", placeholder, err)
		}
	}
	bootstrapPreSites()

	// The documented upgrade entrypoint applies schema before seed. With no
	// historic rows beyond the seed placeholder, the whole upgrade succeeds.
	runMigrations(t, projectRoot(t), temporary.dsn)

	for _, table := range []string{"wb_scale", "wb_site", "wb_user_site"} {
		var count int64
		if err := database.GORM.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("placeholder %s survived the upgrade: count=%d err=%v", table, count, err)
		}
	}
	var identity struct{ SiteCode, ScaleCode string }
	if err := database.GORM.Raw("SELECT site_code,scale_code FROM wb_local_identity WHERE id=1").Scan(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if identity.SiteCode != "" || identity.ScaleCode != "" {
		t.Fatalf("upgrade invented an identity: %+v", identity)
	}
	deps := testDependencies(t, database, t.TempDir())
	deps.Weighing = weighing.NewService(weighing.NewRepository(database.GORM), agentbridge.NewMultiple(nil))
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	response := serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", `{"siteCode":"F001","scaleCode":"WB01"}`, admin)
	assertEnvelopeCode(t, response, http.StatusOK, http.StatusOK, "success")
}

// TestPostgresCodeIdentityMigrationKeepsLegacyCounters covers a pre-sites
// database that already used the material counter while wb_site was empty.
// The schema phase of the documented upgrade (which runs before seeds) must
// keep that counter instead of failing the NOT NULL backfill; the later seed
// phase still refuses to migrate historic data in place, which is the
// documented one-time cutover rule rather than this migration's choice.
func TestPostgresCodeIdentityMigrationKeepsLegacyCounters(t *testing.T) {
	temporary := startPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpToContext(ctx, database.SQL, schema, 12); err != nil {
		t.Fatalf("schema to 12: %v", err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpToContext(ctx, database.SQL, seed, 6); err != nil {
		t.Fatalf("seed to 6: %v", err)
	}
	if err := database.GORM.Exec(`INSERT INTO wb_number_sequence (kind,business_date,next_value) VALUES ('material','20261001',3)`).Error; err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpContext(ctx, database.SQL, schema); err != nil {
		t.Fatalf("schema upgrade must keep a legacy counter while wb_site is empty: %v", err)
	}
	var sequence struct {
		SiteCode string
		Next     int64
	}
	if err := database.GORM.Raw(`SELECT site_code,next_value AS next FROM wb_number_sequence WHERE kind='material' AND business_date='20261001'`).Scan(&sequence).Error; err != nil {
		t.Fatal(err)
	}
	if sequence.SiteCode != "" || sequence.Next != 3 {
		t.Fatalf("legacy material counter changed during upgrade: %+v", sequence)
	}
	// Seeds still apply the documented local-only cutover guard.
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpContext(ctx, database.SQL, seed); err == nil || !strings.Contains(err.Error(), "fresh local database") {
		t.Fatalf("historic counter should require the documented fresh-database cutover, got %v", err)
	}
}
