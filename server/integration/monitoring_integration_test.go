//go:build integration

package integration

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/monitoring"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

func TestSQLiteMonitoringMenuAndAuthorization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitoring.db")
	runSQLiteMigrations(t, path)
	runSQLiteMigrations(t, path)
	database := openSQLiteDatabase(t, path)
	defer func() { _ = database.Close() }()
	testMonitoringMenuAndAuthorization(t, database, sqliteDependencies(t, database, t.TempDir()))
}

func TestPostgresMonitoringMenuAndAuthorization(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	testMonitoringMenuAndAuthorization(t, database, testDependencies(t, database, t.TempDir()))
}

func testMonitoringMenuAndAuthorization(t *testing.T, database *platformdatabase.Database, deps app.Dependencies) {
	t.Helper()
	var menus []struct {
		ID             int64
		ParentID       int64
		MenuName       string
		MenuType       string
		Path           string
		Component      string
		PermissionCode string
		Status         int
		Visible        int
		IsBuiltin      int
	}
	if err := database.GORM.Table("sys_menu").Where("permission_code IN ?", []string{"monitor", "monitor:server"}).Order("id").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	if len(menus) != 2 || menus[0].PermissionCode != "monitor" || menus[0].MenuType != "DIR" || menus[0].ParentID != 0 || menus[0].MenuName != "系统监控" || menus[0].Path != "/monitor" {
		t.Fatalf("monitor directory seed = %+v", menus)
	}
	if menus[1].ParentID != menus[0].ID || menus[1].PermissionCode != "monitor:server" || menus[1].Path != "/monitor/server" || menus[1].Component != "monitor/server/index" || menus[1].MenuName != "服务器监控" || menus[1].MenuType != "MENU" {
		t.Fatalf("monitor page seed = %+v", menus[1])
	}
	for _, menu := range menus {
		if menu.Status != 1 || menu.Visible != 1 || menu.IsBuiltin != 1 {
			t.Fatalf("monitor menu is not enabled built-in: %+v", menu)
		}
	}
	var relations int64
	if err := database.GORM.Table("sys_role_menu rm").Joins("JOIN sys_role r ON r.id = rm.role_id").Joins("JOIN sys_menu m ON m.id = rm.menu_id").Where("r.role_code = ? AND m.permission_code IN ?", "ADMIN", []string{"monitor", "monitor:server"}).Count(&relations).Error; err != nil {
		t.Fatal(err)
	}
	if relations != 2 {
		t.Fatalf("monitor ADMIN relations = %d; want 2 after repeated migration", relations)
	}
	var version int64
	if err := database.GORM.Table("goose_seed_db_version").Select("MAX(version_id)").Where("is_applied = ?", true).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Fatalf("seed version = %d; want shared version 8", version)
	}
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	service := monitoring.NewService(monitoring.NewRepository(database.GORM), nil, func() time.Time { return now })
	service.Ingest(monitoring.Snapshot{
		SourceID: "controlled-host", SampledAt: now,
		Host:   monitoring.Region[monitoring.Host]{Status: monitoring.StatusOK, CollectedAt: &now, Data: &monitoring.Host{Hostname: "integration-host", OS: "Linux", Kernel: "test", UptimeSeconds: 100}},
		Memory: monitoring.Region[monitoring.Memory]{Status: monitoring.StatusOK, CollectedAt: &now, Data: &monitoring.Memory{TotalBytes: 1000, UsedBytes: 500, AvailableBytes: 500, UsagePercent: 50}},
	})
	deps.Monitoring = service
	cfg := testAPIConfig()
	cfg.Monitoring.Enabled = true
	router, err := app.Build(cfg, database, deps)
	if err != nil {
		t.Fatal(err)
	}
	adminToken := loginAdmin(t, router)
	adminMenus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", adminToken)
	if adminMenus.Code != http.StatusOK || !strings.Contains(adminMenus.Body.String(), `"path":"/monitor/server"`) {
		t.Fatalf("ADMIN monitor menus = %d %s", adminMenus.Code, adminMenus.Body.String())
	}
	paths := []string{"/api/v1/monitoring/overview", "/api/v1/monitoring/history?resource=memory"}
	for _, path := range paths {
		assertUnauthenticated(t, serveJSON(router, http.MethodGet, path, "", ""))
		response := serveJSON(router, http.MethodGet, path, "", adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("ADMIN %s = %d %s", path, response.Code, response.Body.String())
		}
	}
	overview := serveJSON(router, http.MethodGet, paths[0], "", adminToken)
	if !strings.Contains(overview.Body.String(), `"hostname":"integration-host"`) || !strings.Contains(overview.Body.String(), `"usagePercent":50`) {
		t.Fatalf("controlled overview = %s", overview.Body.String())
	}
	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"监控普通用户","roleCode":"MONITOR_VIEWER","status":1}`, adminToken)
	assertEnvelopeCode(t, createdRole, http.StatusOK, 200, "success")
	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"monitor-viewer","nickname":"监控普通用户","status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assigned := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[2]}`, adminToken)
	assertEnvelopeCode(t, assigned, http.StatusOK, 200, "success")
	viewerToken := loginUser(t, router, "monitor-viewer", "admin123")
	viewerMenus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", viewerToken)
	if viewerMenus.Code != http.StatusOK || strings.Contains(viewerMenus.Body.String(), `"path":"/monitor/server"`) {
		t.Fatalf("ordinary role monitor menus = %d %s", viewerMenus.Code, viewerMenus.Body.String())
	}
	for _, path := range paths {
		assertMonitoringForbidden(t, router, path, viewerToken)
	}
	// Disabled instances share menu records without changing enabled instances.
	cfg.Monitoring.Enabled = false
	disabledRouter, err := app.Build(cfg, database, deps)
	if err != nil {
		t.Fatal(err)
	}
	disabledMenus := serveJSON(disabledRouter, http.MethodGet, "/api/auth/menus", "", adminToken)
	if disabledMenus.Code != http.StatusOK || strings.Contains(disabledMenus.Body.String(), `"path":"/monitor/server"`) || strings.Contains(disabledMenus.Body.String(), `"path":"/monitor"`) {
		t.Fatalf("disabled menus: %d %s", disabledMenus.Code, disabledMenus.Body.String())
	}
	for _, path := range paths {
		result := serveJSON(disabledRouter, http.MethodGet, path, "", adminToken)
		if result.Code != http.StatusServiceUnavailable || !strings.Contains(result.Body.String(), `"reason":"monitoring_disabled"`) {
			t.Fatalf("disabled route: %d %s", result.Code, result.Body.String())
		}
		assertMonitoringForbidden(t, disabledRouter, path, viewerToken)
		assertUnauthenticated(t, serveJSON(disabledRouter, http.MethodGet, path, "", ""))
	}
	if result := serveJSON(router, http.MethodGet, paths[0], "", adminToken); result.Code != http.StatusOK {
		t.Fatal("disabled instance changed the enabled instance")
	}
	// Modify current role state without revoking the valid login session; the
	// monitoring authorization must observe each change on the next request.
	if err := database.GORM.Table("sys_user_role").Where("user_id = ? AND role_id = ?", 1, 1).Delete(nil).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		assertMonitoringForbidden(t, router, path, adminToken)
	}
	if err := database.GORM.Exec("INSERT INTO sys_user_role (user_id, role_id) VALUES (?, ?)", 1, 1).Error; err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"status", "deleted"} {
		value := 0
		if field == "deleted" {
			value = 1
		}
		if err := database.GORM.Table("sys_role").Where("id = ?", 1).Update(field, value).Error; err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			assertMonitoringForbidden(t, router, path, adminToken)
		}
		restored := 1 - value
		if err := database.GORM.Table("sys_role").Where("id = ?", 1).Update(field, restored).Error; err != nil {
			t.Fatal(err)
		}
		if response := serveJSON(router, http.MethodGet, paths[0], "", adminToken); response.Code != http.StatusOK {
			t.Fatalf("restored ADMIN access = %d %s", response.Code, response.Body.String())
		}
	}
}

func assertMonitoringForbidden(t *testing.T, router http.Handler, path, token string) {
	t.Helper()
	response := serveJSON(router, http.MethodGet, path, "", token)
	if response.Code != http.StatusForbidden {
		t.Fatalf("restricted %s = %d %s; want 403", path, response.Code, response.Body.String())
	}
}
