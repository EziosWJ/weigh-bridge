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

// ADR 0033 replaces automatic identity creation: the head-office codes are
// installed once through the configuration-management entry. These tests keep
// the original transactional, concurrent and non-destructive guarantees.
func TestPostgresCodeIdentityInitialization(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	ctx := context.Background()

	// A fresh database is unconfigured until the codes are supplied.
	status, err := localsite.ReadStatus(ctx, db.GORM)
	if err != nil || status.Configured || status.SiteCode != "" {
		t.Fatalf("fresh status=%+v err=%v", status, err)
	}

	// A late failure must roll back identity, bindings, grants and audit alike.
	if err := db.GORM.Exec(`CREATE FUNCTION reject_auto_binding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced setup failure'; END $$;
CREATE TRIGGER reject_auto_binding BEFORE INSERT ON wb_agent_binding FOR EACH ROW EXECUTE FUNCTION reject_auto_binding()`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err == nil {
		t.Fatal("expected failed setup")
	}
	for _, table := range []string{"wb_site", "wb_scale", "wb_local_identity", "wb_agent_binding", "wb_local_user_access"} {
		var count int64
		if err := db.GORM.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed setup left %s count=%d err=%v", table, count, err)
		}
	}
	if err := db.GORM.Exec(`DROP TRIGGER reject_auto_binding ON wb_agent_binding; DROP FUNCTION reject_auto_binding()`).Error; err != nil {
		t.Fatal(err)
	}

	// Simultaneous submissions install one identity; the rest see a conflict.
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	installed := 0
	var lastErr error
	for err := range results {
		if err == nil {
			installed++
		} else {
			lastErr = err
		}
	}
	if installed != 1 {
		t.Fatalf("concurrent initialization installed %d identities: %v", installed, lastErr)
	}
	status, err = localsite.ReadStatus(ctx, db.GORM)
	if err != nil || status != (localsite.Status{SiteCode: "F001", ScaleCode: "WB01", Configured: true, Initialized: true}) {
		t.Fatalf("installed status=%+v err=%v", status, err)
	}
	var site weighing.Site
	var scale weighing.Scale
	if err := db.GORM.Where("site_code=?", "F001").Take(&site).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Where("site_code=? AND code=?", "F001", "WB01").Take(&scale).Error; err != nil {
		t.Fatal(err)
	}
	if site.Name != "本厂" || scale.Name != "本厂地磅" {
		t.Fatalf("internal labels: %+v %+v", site, scale)
	}
	var grants []weighing.SiteGrant
	if err := db.GORM.Table("wb_local_user_access a").Select("a.user_id,i.site_code,a.can_operate").Joins("JOIN wb_local_identity i ON i.id=1").Scan(&grants).Error; err != nil || len(grants) != 1 || grants[0].UserID != 1 || grants[0].SiteCode != "F001" || grants[0].CanOperate != 1 {
		t.Fatalf("bootstrap scope=%+v err=%v", grants, err)
	}
	var audits int64
	if err := db.GORM.Table("sys_oper_log").Where("operation_type='site.initialize'").Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("initialization audit count=%d err=%v", audits, err)
	}

	// Later submissions must not undo a disabled site, revoked scope, saved
	// name, key or allocator position.
	if err := db.GORM.Exec("UPDATE wb_site SET name='既有名称',enabled=0,version=7 WHERE site_code='F001'; UPDATE wb_local_identity SET next_id=12345 WHERE id=1; UPDATE wb_local_user_access SET can_operate=0 WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	var before weighing.AgentBinding
	if err := db.GORM.First(&before, "scale_code=?", "WB01").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F999", ScaleCode: "WB09"}, audit.Event{}); err == nil {
		t.Fatal("a second identity was accepted")
	}
	identity, err := localsite.Read(ctx, db.GORM)
	if err != nil || identity.NextID != 12345 || identity.SiteCode != "F001" {
		t.Fatalf("identity changed: %+v err=%v", identity, err)
	}
	if err := db.GORM.Where("site_code=?", "F001").Take(&site).Error; err != nil || site.Enabled != 0 || site.Name != "既有名称" || site.Version != 7 {
		t.Fatalf("site changed: %+v err=%v", site, err)
	}
	var after weighing.AgentBinding
	if err := db.GORM.First(&after, "scale_code=?", "WB01").Error; err != nil || after.AgentKey != before.AgentKey {
		t.Fatal("existing binding changed", err)
	}
	if err := db.GORM.Table("wb_local_user_access a").Select("a.user_id,i.site_code,a.can_operate").Joins("JOIN wb_local_identity i ON i.id=1").Scan(&grants).Error; err != nil || len(grants) != 1 || grants[0].CanOperate != 0 {
		t.Fatal("revoked grant restored", err)
	}
}

func TestPostgresCodeIdentityPreservesLegacyDatabase(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	ctx := context.Background()
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F023", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	status, err := localsite.ReadStatus(ctx, db.GORM)
	if err != nil || status.SiteCode != "F023" || status.ScaleCode != "WB01" {
		t.Fatalf("installed codes changed: %+v err=%v", status, err)
	}
	// A damaged database must be diagnosed, never reset or assigned a new
	// identity while business rows still exist.
	if err := db.GORM.Exec("DELETE FROM wb_local_identity").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F024", ScaleCode: "WB01"}, audit.Event{}); err == nil {
		t.Fatal("orphaned business data was silently reinitialized")
	}
	var count int64
	if err := db.GORM.Table("wb_site").Where("site_code=?", "F023").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy site lost: %d %v", count, err)
	}
}
