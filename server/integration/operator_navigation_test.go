//go:build integration

package integration

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/pressly/goose/v3"
)

func TestPostgresOperatorDefaultMenus(t *testing.T) {
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
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"menu-operator","nickname":"操作员","status":1}`, admin), 200, 200, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username=?", "menu-operator").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("operator id=%d err=%v", userID, err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	repo := rbac.NewRepository(db.GORM)
	codes, err := repo.PermissionCodes(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"weighing:read", "weighing:master:read", "weighing:confirm", "system:log", "system:login-log", "system:oper-log"} {
		if !slices.Contains(codes, code) {
			t.Fatalf("operator missing %s: %v", code, codes)
		}
	}
	for _, code := range []string{"system:user", "system:role", "system:config", "weighing:master:write"} {
		if slices.Contains(codes, code) {
			t.Fatalf("operator received unrelated permission %s", code)
		}
	}
	operator := loginUser(t, router, "menu-operator", "admin123")
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", operator)
	assertEnvelopeCode(t, menus, 200, 200, "success")
	for _, path := range []string{"/log", "/system/login-log", "/system/oper-log"} {
		if !strings.Contains(menus.Body.String(), `"path":"`+path+`"`) {
			t.Fatalf("operator missing log menu %s: %s", path, menus.Body.String())
		}
	}
	for _, path := range []string{"/api/system/login-log/page", "/api/system/oper-log/page"} {
		assertEnvelopeCode(t, serveJSON(router, http.MethodGet, path, "", operator), 200, 200, "success")
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"越权","roleCode":"UNAUTHORIZED","status":1}`, operator), 403, 403, "无权执行此操作")

	// Upgrade a database with one existing log grant; retain administrator
	// grants and avoid duplicate relationships when migrations run again.
	adminBefore, err := repo.PermissionCodes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	if err := goose.DownToContext(context.Background(), db.SQL, seed, 16); err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR') AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('system:login-log','system:oper-log'))`).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := goose.UpContext(context.Background(), db.SQL, seed); err != nil {
			t.Fatal(err)
		}
	}
	upgraded, err := repo.PermissionCodes(context.Background(), userID)
	if err != nil || !slices.Equal(codes, upgraded) {
		t.Fatalf("upgrade permissions=%v, want=%v, err=%v", upgraded, codes, err)
	}
	adminAfter, err := repo.PermissionCodes(context.Background(), 1)
	if err != nil || !slices.Equal(adminBefore, adminAfter) {
		t.Fatalf("upgrade changed administrator permissions: %v", err)
	}
}
