//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// ADR 0033 moves identity setup into 系统管理 → 配置管理, which is entered with
// system:config. An administrator holding only that permission must be able to
// install the head-office code and maintain the serial watermark; the legacy
// weighing:site:manage grant keeps working for existing deployments.
func TestPostgresIdentityEntryAcceptsSystemConfigPermission(t *testing.T) {
	postgres := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, postgres.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	deps.Weighing = weighing.NewService(weighing.NewRepository(db.GORM), agentbridge.NewMultiple(nil))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)

	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"配置管理员","roleCode":"CONFIG_ADMIN","status":1}`, admin)
	assertEnvelopeCode(t, createdRole, http.StatusOK, http.StatusOK, "success")
	var roleID, configMenuID int64
	if err := db.GORM.Table("sys_role").Select("id").Where("role_code='CONFIG_ADMIN'").Scan(&roleID).Error; err != nil || roleID == 0 {
		t.Fatalf("config role id=%d err=%v", roleID, err)
	}
	if err := db.GORM.Table("sys_menu").Select("id").Where("permission_code='system:config' AND status=1 AND deleted=0").Scan(&configMenuID).Error; err != nil || configMenuID == 0 {
		t.Fatalf("system:config menu id=%d err=%v", configMenuID, err)
	}
	menus := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/role/%d/menus", roleID), fmt.Sprintf(`{"menuIds":[%d]}`, configMenuID), admin)
	assertEnvelopeCode(t, menus, http.StatusOK, http.StatusOK, "success")

	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"config-operator","nickname":"配置管理员","status":1}`, admin)
	assertEnvelopeCode(t, createdUser, http.StatusOK, http.StatusOK, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username='config-operator'").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("config user id=%d err=%v", userID, err)
	}
	roles := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/user/%d/roles", userID), fmt.Sprintf(`{"roleIds":[%d]}`, roleID), admin)
	assertEnvelopeCode(t, roles, http.StatusOK, http.StatusOK, "success")
	operator := loginUser(t, router, "config-operator", "admin123")

	initialize := serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", `{"siteCode":"F001","scaleCode":"WB01"}`, operator)
	assertEnvelopeCode(t, initialize, http.StatusOK, http.StatusOK, "success")
	for key, want := range map[string]string{"weighing.local-site-code": "F001", "weighing.local-scale-code": "WB01", "weighing.local-site-name": "本厂", "weighing.local-scale-name": "本厂地磅"} {
		var got string
		if err := db.GORM.Table("sys_config").Select("config_value").Where("config_key=?", key).Scan(&got).Error; err != nil || got != want {
			t.Fatalf("config %s = %q, want %q, err=%v", key, got, want, err)
		}
	}
	configs := serveJSON(router, http.MethodGet, "/api/weighing/site-config", "", operator)
	assertEnvelopeCode(t, configs, http.StatusOK, http.StatusOK, "success")
	watermark := serveJSON(router, http.MethodPut, "/api/weighing/ticket-sequences", `{"businessDate":"20261027","lastIssued":5}`, operator)
	assertEnvelopeCode(t, watermark, http.StatusOK, http.StatusOK, "success")
	list := serveJSON(router, http.MethodGet, "/api/weighing/ticket-sequences", "", operator)
	assertEnvelopeCode(t, list, http.StatusOK, http.StatusOK, "success")

	// A user without either configuration permission still gets 403.
	plain := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"plain-user","nickname":"普通用户","status":1}`, admin)
	assertEnvelopeCode(t, plain, http.StatusOK, http.StatusOK, "success")
	plainToken := loginUser(t, router, "plain-user", "admin123")
	denied := serveJSON(router, http.MethodPut, "/api/weighing/ticket-sequences", `{"businessDate":"20261027","lastIssued":6}`, plainToken)
	assertEnvelopeCode(t, denied, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
}
