//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresMasterSyncPermissionsAndLogAudit(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	deps.Weighing = service
	ctx := context.Background()
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"message":"ok","data":{"schemaVersion":2,"siteCode":"F001","categories":[],"materials":[],"parties":[]}}`)
	}))
	defer remote.Close()
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	created := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"weighing-operator","nickname":"操作员","status":1}`, admin)
	assertEnvelopeCode(t, created, 200, 200, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username = ?", "weighing-operator").Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role (user_id, role_id) SELECT ?, id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO wb_local_user_access (user_id,can_operate) VALUES (?,1)", userID).Error; err != nil {
		t.Fatal(err)
	}
	operator := loginUser(t, router, "weighing-operator", "admin123")
	denied := serveJSON(router, http.MethodPost, "/api/weighing/master-sync", "{}", operator)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("operator bound before admin: %d %s", denied.Code, denied.Body.String())
	}
	success := serveJSON(router, http.MethodPost, "/api/weighing/master-sync", "{}", admin)
	if success.Code != http.StatusOK || envelopeCode(t, success) != 200 {
		t.Fatalf("admin sync: %d %s", success.Code, success.Body.String())
	}
	success = serveJSON(router, http.MethodPost, "/api/weighing/master-sync", "{}", operator)
	if success.Code != http.StatusOK || envelopeCode(t, success) != 200 {
		t.Fatalf("operator sync: %d %s", success.Code, success.Body.String())
	}
	denied = serveJSON(router, http.MethodDelete, "/api/weighing/master-sync/logs", "", operator)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("operator cleared logs: %d %s", denied.Code, denied.Body.String())
	}
	success = serveJSON(router, http.MethodDelete, "/api/weighing/master-sync/logs", "", admin)
	if success.Code != http.StatusOK || envelopeCode(t, success) != 200 {
		t.Fatalf("admin clear: %d %s", success.Code, success.Body.String())
	}
	var logs, audits int64
	if err := db.GORM.Table("wb_master_sync_log").Count(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_oper_log").Where("operation_type = ?", "master-sync.logs.clear").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if logs != 0 || audits != 1 {
		t.Fatalf("log clear: logs=%d audits=%d", logs, audits)
	}
}
