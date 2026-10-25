//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/app"
)

func TestPostgresCameraPreviewUsesExistingReadPermissionsAndSiteScope(t *testing.T) {
	temporary := startBusinessPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer database.Close()
	deps := testDependencies(t, database, t.TempDir())
	maintenance, err := agentmaintenance.OpenWithDraft(t.TempDir(), &maintenanceIntegrationRunner{}, agentprinter.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	deps.Maintenance = maintenance
	deps.MaintenanceBusinessValidator = func(context.Context, agentmaintenance.Config) error { return nil }
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	ensureLocalSite(t, router, token, 1)
	var scale struct{ ID, SiteID int64 }
	if err := database.GORM.Table("wb_scale").Select("id,site_id").Where("site_id=?", 1).Take(&scale).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/weighing/scales/%d/camera-preview", scale.ID)
	req := serveJSON(router, http.MethodGet, path, "", token)
	if req.Code != 200 {
		t.Fatalf("valid operator denied: %d %s", req.Code, req.Body.String())
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := database.GORM.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE wb_user_site SET can_operate=0 WHERE user_id=1 AND site_id=?", scale.SiteID)
	if got := serveJSON(router, http.MethodGet, path, "", token); got.Code != 200 {
		t.Fatalf("existing read-only site access denied: %d", got.Code)
	}
	exec("UPDATE wb_user_site SET can_operate=1 WHERE user_id=1 AND site_id=?", scale.SiteID)
	exec("UPDATE sys_menu SET status=0 WHERE permission_code='weighing:read'")
	if got := serveJSON(router, http.MethodGet, path, "", token); got.Code != 403 {
		t.Fatalf("live permission removal ignored: %d", got.Code)
	}
	exec("UPDATE sys_menu SET status=1 WHERE permission_code='weighing:read'")
	if got := serveJSON(router, http.MethodGet, path, "", token); got.Code != 200 {
		t.Fatalf("restored permissions unavailable: %d %s", got.Code, got.Body.String())
	}
	if got := serveJSON(router, http.MethodGet, path, "", ""); got.Code != 401 {
		t.Fatalf("anonymous preview config=%d", got.Code)
	}
	exec("UPDATE auth_session SET revoked_at=CURRENT_TIMESTAMP WHERE user_id=1")
	if got := serveJSON(router, http.MethodGet, path, "", token); got.Code != 401 {
		t.Fatalf("revoked login allowed preview: %d", got.Code)
	}
}
