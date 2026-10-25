//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresSiteIsolation(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, "LOCAL01", "本厂", "SCALE01", "一号地磅", "local-agent")

	caps := weighingData[struct {
		Scales []weighing.Scale `json:"scales"`
		Sites  []weighing.Site  `json:"sites"`
	}](t, router, http.MethodGet, "/api/weighing/capabilities", "", admin)
	if len(caps.Sites) != 1 || caps.Sites[0].ID != 1 || len(caps.Scales) != 1 || caps.Scales[0].ID != 1 {
		t.Fatalf("capabilities must expose only the configured local site: %+v", caps)
	}
	if got := weighingData[[]weighing.Site](t, router, http.MethodGet, "/api/weighing/sites", "", admin); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("site list is not local-only: %+v", got)
	}
	if got := serveJSON(router, http.MethodGet, "/api/weighing/scales/2/device", "", admin); got.Code != http.StatusNotFound {
		t.Fatalf("foreign scale read status=%d body=%s", got.Code, got.Body.String())
	}
	if got := serveJSON(router, http.MethodGet, "/api/weighing/site-config/2/connection", "", admin); got.Code != http.StatusNotFound {
		t.Fatalf("foreign connection export status=%d body=%s", got.Code, got.Body.String())
	}
	if got := serveJSON(router, http.MethodGet, "/api/weighing/tasks?siteId=2", "", admin); got.Code != http.StatusForbidden {
		t.Fatalf("foreign site filter status=%d body=%s", got.Code, got.Body.String())
	}

	created := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"local-scope-operator","nickname":"本厂操作员","status":1}`, admin)
	assertEnvelopeCode(t, created, http.StatusOK, http.StatusOK, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username='local-scope-operator'").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("operator id=%d err=%v", userID, err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	operator := loginUser(t, router, "local-scope-operator", "admin123")
	if got := weighingData[[]weighing.Site](t, router, http.MethodGet, "/api/weighing/sites", "", operator); len(got) != 0 {
		t.Fatalf("new account received implicit local scope: %+v", got)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/users/%d/sites", userID), `{"grants":[{"siteId":2,"canOperate":1}]}`, admin), http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
	var foreignGrants int64
	if err := db.GORM.Table("wb_user_site").Where("user_id=? AND site_id=2", userID).Count(&foreignGrants).Error; err != nil || foreignGrants != 0 {
		t.Fatalf("foreign site grant was persisted: count=%d err=%v", foreignGrants, err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/users/%d/sites", userID), `{"grants":[{"siteId":1,"canOperate":1}]}`, admin), http.StatusOK, http.StatusOK, "success")
	if got := weighingData[[]weighing.Site](t, router, http.MethodGet, "/api/weighing/sites", "", operator); len(got) != 1 || got[0].ID != 1 || !got[0].CanOperate {
		t.Fatalf("explicit local grant not applied: %+v", got)
	}

	additional := serveJSON(router, http.MethodPost, "/api/weighing/site-config", `{"code":"OTHER","name":"他厂","scaleCode":"OTHER-SCALE","scaleName":"他磅","agentId":"other-agent","enabled":1}`, admin)
	if additional.Code == http.StatusOK && envelopeCode(t, additional) == http.StatusOK {
		t.Fatal("single local installation created another factory")
	}
	var sites, scales int64
	if err := db.GORM.Table("wb_site").Count(&sites).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("wb_scale").Count(&scales).Error; err != nil {
		t.Fatal(err)
	}
	if sites != 1 || scales != 1 {
		t.Fatalf("legacy add-site request changed local tables: sites=%d scales=%d", sites, scales)
	}
}
