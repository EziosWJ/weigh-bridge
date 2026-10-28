//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresMasterSyncLifecycle(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	body := `{"code":200,"message":"ok","data":{"schemaVersion":1,"siteCode":"F001","scaleCodes":["WB01"],"categories":[{"code":"C-C-0001","name":"谷物","enabled":true}],"materials":[{"code":"C-M-0001","name":"高粱","categoryCode":"C-C-0001","spec":"","remark":"","enabled":true}],"parties":[{"code":"C-P-0001","name":"供货方","shortName":"","nature":"enterprise","isSupplier":true,"isCustomer":false,"isCarrier":false,"phone":"","address":"","creditCode":"","remark":"","enabled":true,"contacts":[{"id":1,"name":"联系人","phone":"","purpose":"","remark":"","isDefault":true}]}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/weighbridge/sync/master" || r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	if err := service.ConfigureMasterSync(ctx, true, server.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmFirst(ctx, weighing.ConfirmationInput{}, audit.Event{}); err == nil || !strings.Contains(err.Error(), "首次总部资料同步") {
		t.Fatalf("first use not blocked: %v", err)
	}
	if err := service.RunMasterSync(ctx, "automatic"); err != nil {
		t.Fatal(err)
	}
	status, err := service.MasterSyncStatus(ctx)
	if err != nil || status.Bound {
		t.Fatalf("automatic first sync: %+v %v", status, err)
	}
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	status, err = service.MasterSyncStatus(ctx)
	if err != nil || !status.Bound || status.LastSuccess == nil {
		t.Fatalf("initial sync: %+v %v", status, err)
	}
	// A regular restart with the same switch must preserve the binding.
	if err := service.ConfigureMasterSync(ctx, true, server.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	status, err = service.MasterSyncStatus(ctx)
	if err != nil || !status.Bound {
		t.Fatalf("restart lost binding: %+v %v", status, err)
	}
	var material struct {
		ID   int64
		Name string
	}
	if err := db.GORM.Table("wb_material").Where("code=?", "C-M-0001").Take(&material).Error; err != nil {
		t.Fatal(err)
	}
	body = strings.Replace(body, `"name":"高粱"`, `"name":"玉米"`, 1)
	if err := service.RunMasterSync(ctx, "automatic"); err != nil {
		t.Fatal(err)
	}
	var updated struct {
		ID   int64
		Name string
	}
	if err := db.GORM.Table("wb_material").Where("code=?", "C-M-0001").Take(&updated).Error; err != nil || updated.ID != material.ID || updated.Name != "玉米" {
		t.Fatalf("update %+v %v", updated, err)
	}
	body = strings.Replace(body, `[{"code":"C-C-0001","name":"谷物","enabled":true}]`, `[]`, 1)
	if err := service.RunMasterSync(ctx, "manual"); err == nil {
		t.Fatal("missing category accepted")
	}
	status, err = service.MasterSyncStatus(ctx)
	if err != nil || !status.Bound {
		t.Fatalf("status changed on failure %+v %v", status, err)
	}
	if err := db.GORM.Exec("UPDATE wb_material SET name = '单机修改' WHERE code = 'C-M-0001'").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigureMasterSync(ctx, false, "", ""); err != nil {
		t.Fatal(err)
	}
	status, _ = service.MasterSyncStatus(ctx)
	if status.Bound || status.Enabled {
		t.Fatalf("standalone %+v", status)
	}
	if err := service.ConfigureMasterSync(ctx, true, server.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	status, _ = service.MasterSyncStatus(ctx)
	if status.Bound || !status.Enabled {
		t.Fatalf("re-entry %+v", status)
	}
	if _, err := service.ConfirmFirst(ctx, weighing.ConfirmationInput{}, audit.Event{}); err != nil && strings.Contains(err.Error(), "首次总部资料同步") {
		t.Fatalf("re-entry blocked standalone mode: %v", err)
	}
	body = strings.Replace(body, `"categories":[]`, `"categories":[{"code":"C-C-0001","name":"谷物","enabled":true}]`, 1)
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("wb_material").Where("code=?", "C-M-0001").Take(&updated).Error; err != nil || updated.ID != material.ID || updated.Name != "玉米" {
		t.Fatalf("re-entry failed to restore cloud values: %+v %v", updated, err)
	}
}
