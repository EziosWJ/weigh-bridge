//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// TestPostgresMasterSyncRealCloud exercises the real cloud HTTP service using
// an isolated local database. It is skipped unless the caller supplies an
// ephemeral cloud endpoint and its test-only factory credential.
func TestPostgresMasterSyncRealCloud(t *testing.T) {
	endpoint, token := os.Getenv("WB_SYNC_TEST_URL"), os.Getenv("WB_SYNC_TEST_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("isolated cloud endpoint not supplied")
	}
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigureMasterSync(ctx, true, endpoint, token); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatalf("real cloud synchronization: %v", err)
	}
	status, err := service.MasterSyncStatus(ctx)
	if err != nil || !status.Bound || status.LastSuccess == nil {
		t.Fatalf("status %+v: %v", status, err)
	}
	var row struct {
		Category string
		Material string
		Spec     string
		Party    string
		Contact  string
	}
	if err := db.GORM.Raw(`SELECT c.name AS category, m.name AS material, m.spec, p.name AS party, x.name AS contact
	FROM wb_master_material_map mm JOIN wb_material m ON m.id=mm.local_id
	JOIN wb_master_category_map cm ON cm.local_id=m.category_id JOIN wb_material_category c ON c.id=cm.local_id
	JOIN wb_master_party_map pm ON pm.cloud_code='C-P-0001' JOIN wb_party p ON p.id=pm.local_id
	JOIN wb_party_contact x ON x.party_id=p.id
	WHERE mm.cloud_code='C-M-0001' AND cm.cloud_code='C-C-0001' LIMIT 1`).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Category != "谷物" || row.Material != "高粱" || row.Spec != "一级" || row.Party == "" || row.Contact == "" {
		t.Fatalf("cloud values not persisted: %+v", row)
	}
	if err := service.ConfigureMasterSync(ctx, true, endpoint, "invalid-test-credential"); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMasterSync(ctx, "manual"); err == nil {
		t.Fatal("invalid credential accepted")
	}
	var name string
	if err := db.GORM.Table("wb_material").Where("code = ?", "C-M-0001").Select("name").Scan(&name).Error; err != nil || name != "高粱" {
		t.Fatalf("failed sync modified data: %q %v", name, err)
	}
}
