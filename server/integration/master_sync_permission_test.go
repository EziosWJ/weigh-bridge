//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	var readyChecks atomic.Int32
	var readinessStatus atomic.Int32
	readinessStatus.Store(http.StatusOK)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			readyChecks.Add(1)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
				t.Error("readiness request must be an unauthenticated, parameter-free GET")
			}
			w.WriteHeader(int(readinessStatus.Load()))
			fmt.Fprint(w, `{"code":200,"message":"ok","data":{"status":"ok"}}`)
		case "/api/v1/weighbridge/sync/master":
			fmt.Fprint(w, `{"code":200,"message":"ok","data":{"schemaVersion":2,"siteCode":"F001","categories":[],"materials":[],"parties":[]}}`)
		default:
			http.NotFound(w, r)
		}
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
	capabilities := weighingData[struct {
		MasterSyncEnabled bool `json:"masterSyncEnabled"`
		MasterSyncBound   bool `json:"masterSyncBound"`
	}](t, router, http.MethodGet, "/api/weighing/capabilities", "", admin)
	if !capabilities.MasterSyncEnabled || !capabilities.MasterSyncBound {
		t.Fatalf("expected configured and bound capabilities: %+v", capabilities)
	}
	const connectionPath = "/api/weighing/sync/connection"
	denied = serveJSON(router, http.MethodGet, connectionPath, "", operator)
	assertEnvelopeCode(t, denied, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
	if readyChecks.Load() != 0 {
		t.Fatal("operator's request reached the cloud")
	}
	if err := db.GORM.Exec(`INSERT INTO wb_master_sync_log (sync_type, trigger_type, success, reason, attempted_at)
		VALUES ('master', 'automatic', false, '模拟资料同步失败', now() + interval '1 minute'),
		('issue', 'automatic', true, '', now() + interval '2 minutes')`).Error; err != nil {
		t.Fatal(err)
	}
	connection := weighingData[weighing.CloudConnection](t, router, http.MethodGet, connectionPath, "", admin)
	if readyChecks.Load() != 1 || connection.Address != remote.URL || !connection.Enabled || !connection.Bound ||
		connection.SiteCode != "F001" || connection.Readiness.Status != "ready" || connection.Readiness.CheckedAt == nil ||
		connection.LastAttempt == nil || connection.LastAttempt.Success || connection.LastAttempt.Reason != "模拟资料同步失败" ||
		connection.LastSuccess == nil {
		t.Fatalf("unexpected connection details: %+v", connection)
	}
	readinessStatus.Store(http.StatusServiceUnavailable)
	connection = weighingData[weighing.CloudConnection](t, router, http.MethodGet, connectionPath, "", admin)
	if readyChecks.Load() != 2 || connection.Readiness.Status != "not_ready" || connection.LastAttempt == nil || connection.LastAttempt.Success {
		t.Fatalf("recheck did not preserve the last sync result: %+v", connection)
	}
	var beforeClear int64
	if err := db.GORM.Table("wb_master_sync_log").Count(&beforeClear).Error; err != nil || beforeClear != 4 {
		t.Fatalf("probe changed sync logs: count=%d err=%v", beforeClear, err)
	}
	if err := service.ConfigureMasterSync(ctx, false, remote.URL); err != nil {
		t.Fatal(err)
	}
	connection = weighingData[weighing.CloudConnection](t, router, http.MethodGet, connectionPath, "", admin)
	if readyChecks.Load() != 2 || connection.Readiness.Status != "disabled" || connection.Readiness.CheckedAt != nil ||
		connection.Address != remote.URL || connection.Enabled {
		t.Fatalf("disabled sync should preserve the address and skip the probe: %+v", connection)
	}
	if err := service.ConfigureMasterSync(ctx, true, "https://user:test-only@cloud.example"); err != nil {
		t.Fatal(err)
	}
	connection = weighingData[weighing.CloudConnection](t, router, http.MethodGet, connectionPath, "", admin)
	if readyChecks.Load() != 2 || connection.Readiness.Status != "invalid_address" || connection.Address != "" {
		t.Fatalf("invalid address should skip the probe and be omitted: %+v", connection)
	}

	denied = serveJSON(router, http.MethodDelete, "/api/weighing/master-sync/logs", "", operator)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("operator cleared logs: %d %s", denied.Code, denied.Body.String())
	}
	const logsPath = "/api/weighing/sync/logs"
	entries := weighingData[[]weighing.MasterSyncLog](t, router, http.MethodGet, logsPath, "", admin)
	var throughID int64
	for _, entry := range entries {
		throughID = max(throughID, entry.ID)
	}
	if err := db.GORM.Exec("INSERT INTO wb_master_sync_log (trigger_type, sync_type, success, reason, task_no) VALUES ('automatic', 'task', true, '', 'RW-F001-0001')").Error; err != nil {
		t.Fatal(err)
	}
	stale := serveJSON(router, http.MethodDelete, fmt.Sprintf("%s?throughId=%d&count=%d", logsPath, throughID, len(entries)-1), "", admin)
	if stale.Code == http.StatusOK && envelopeCode(t, stale) == 200 {
		t.Fatal("clear accepted a stale count")
	}
	success = serveJSON(router, http.MethodDelete, fmt.Sprintf("%s?throughId=%d&count=%d", logsPath, throughID, len(entries)), "", admin)
	assertEnvelopeCode(t, success, http.StatusOK, 200, "success")
	remaining := weighingData[[]weighing.MasterSyncLog](t, router, http.MethodGet, logsPath, "", admin)
	if len(remaining) != 1 || remaining[0].TaskNo != "RW-F001-0001" {
		t.Fatalf("clear deleted a newly added log: %+v", remaining)
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
	if logs != 0 || audits != 2 {
		t.Fatalf("log clear: logs=%d audits=%d", logs, audits)
	}
}
