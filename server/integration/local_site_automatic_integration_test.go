//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresAutomaticLocalSite(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	ctx := context.Background()

	// A late failure must roll back identity, bindings, grants and audit alike.
	if err := db.GORM.Exec(`CREATE FUNCTION reject_auto_binding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced automatic setup failure'; END $$;
CREATE TRIGGER reject_auto_binding BEFORE INSERT ON wb_agent_binding FOR EACH ROW EXECUTE FUNCTION reject_auto_binding()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureLocalSite(ctx); err == nil {
		t.Fatal("expected failed setup")
	}
	for _, table := range []string{"wb_site", "wb_scale", "wb_local_identity", "wb_agent_binding", "wb_user_site"} {
		var count int64
		if err := db.GORM.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed setup left %s count=%d err=%v", table, count, err)
		}
	}
	if err := db.GORM.Exec(`DROP TRIGGER reject_auto_binding ON wb_agent_binding; DROP FUNCTION reject_auto_binding()`).Error; err != nil {
		t.Fatal(err)
	}

	// Simultaneous starts all succeed while creating one internal identity.
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errors <- service.EnsureLocalSite(ctx) })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	status, err := localsite.ReadStatus(ctx, db.GORM)
	if err != nil || status != (localsite.Status{SiteID: 1, Configured: true, Initialized: true}) {
		t.Fatalf("automatic status=%+v err=%v", status, err)
	}
	var site weighing.Site
	var scale weighing.Scale
	if err := db.GORM.First(&site, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.First(&scale, 1).Error; err != nil {
		t.Fatal(err)
	}
	if site.Name != "本厂" || scale.Name != "本厂地磅" {
		t.Fatalf("internal labels: %+v %+v", site, scale)
	}
	var grants []weighing.SiteGrant
	if err := db.GORM.Find(&grants).Error; err != nil || len(grants) != 1 || grants[0].UserID != 1 || grants[0].CanOperate != 1 {
		t.Fatalf("bootstrap scope=%+v err=%v", grants, err)
	}
	var audits int64
	if err := db.GORM.Table("sys_oper_log").Where("operation_type='site.initialize' AND request_method='SYSTEM'").Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("initialization audit count=%d err=%v", audits, err)
	}

	// Startup must not undo a disabled site, revoked scope, saved name, key or
	// allocator position, even with the old initialized flag still zero.
	if err := db.GORM.Exec("UPDATE wb_site SET name='既有名称',enabled=0,version=7 WHERE id=1; UPDATE wb_local_identity SET next_id=12345,initialized=0 WHERE id=1; UPDATE wb_user_site SET can_operate=0 WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	var before weighing.AgentBinding
	if err := db.GORM.First(&before, "scale_id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureLocalSite(ctx); err != nil {
		t.Fatal(err)
	}
	identity, err := localsite.Read(ctx, db.GORM)
	if err != nil || identity.NextID != 12345 || identity.Initialized != 0 {
		t.Fatalf("identity changed: %+v err=%v", identity, err)
	}
	if err := db.GORM.First(&site, 1).Error; err != nil || site.Enabled != 0 || site.Name != "既有名称" || site.Version != 7 {
		t.Fatalf("site changed: %+v err=%v", site, err)
	}
	var after weighing.AgentBinding
	if err := db.GORM.First(&after, "scale_id=1").Error; err != nil || after.AgentKey != before.AgentKey {
		t.Fatal("existing binding changed", err)
	}
	if err := db.GORM.Find(&grants).Error; err != nil || grants[0].CanOperate != 0 {
		t.Fatal("revoked grant restored", err)
	}
}

func TestPostgresAutomaticLocalSitePreservesLegacyIdentity(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	ctx := context.Background()
	// Legacy bootstrap remains compatible and its non-default prefix is fixed.
	_, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteID: 23}, audit.Event{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureLocalSite(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := localsite.ReadStatus(ctx, db.GORM)
	if err != nil || status.SiteID != 23 {
		t.Fatalf("legacy prefix changed: %+v err=%v", status, err)
	}
	// A damaged database must be diagnosed, never reset or assigned a new ID.
	if err := db.GORM.Exec("DELETE FROM wb_local_identity").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureLocalSite(ctx); err == nil {
		t.Fatal("orphaned business data was silently reinitialized")
	}
	var count int64
	if err := db.GORM.Table("wb_site").Where("id=23").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy site lost: %d %v", count, err)
	}
}
