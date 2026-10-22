//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/nodesync"
	"github.com/gin-gonic/gin"
)

func TestPostgresStandaloneInitializationAndLocalActivation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	root := projectRoot(t)
	postgres := startPostgres(t)
	runMigrations(t, root, postgres.dsn)
	db := openTemporaryDatabase(t, postgres.dsn)
	defer db.Close()

	path := filepath.Join(t.TempDir(), "agent.env")
	var initialPassword string
	if err := db.GORM.Table("sys_user").Select("password").Where("id=1").Scan(&initialPassword).Error; err != nil {
		t.Fatal(err)
	}
	failedEnv := filepath.Join(t.TempDir(), "failed", "agent.env")
	if _, err := nodesync.InitializeStandalone(ctx, db.GORM, config.NodeConfig{Role: "standalone", SiteID: 1}, nodesync.StandaloneInit{
		SiteCode: "SITE02", SiteName: "冲突厂区", ScaleCode: "ROLLBACK01", ScaleName: "冲突磅", AgentID: "rollback-agent",
		AdminPassword: "local-admin-password", AgentEnvPath: failedEnv,
	}, ":18900"); err == nil {
		t.Fatal("duplicate site code should roll back initialization")
	}
	if _, err := os.Stat(failedEnv); !os.IsNotExist(err) {
		t.Fatalf("failed initialization left agent.env: %v", err)
	}
	var count int64
	if err := db.GORM.Table("wb_node_identity").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed initialization identity count=%d err=%v", count, err)
	}
	var afterFailurePassword string
	if err := db.GORM.Table("sys_user").Select("password").Where("id=1").Scan(&afterFailurePassword).Error; err != nil || afterFailurePassword != initialPassword {
		t.Fatalf("failed initialization changed seeded admin credential: %v", err)
	}
	legacyTemporary, err := nodesync.InitializeStandalone(ctx, db.GORM, config.NodeConfig{Role: "standalone", SiteID: 1}, nodesync.StandaloneInit{
		SiteCode: "LOCAL01", SiteName: "本厂", ScaleCode: "SCALE01", ScaleName: "一号地磅",
		AgentID: "local-agent-1", AdminPassword: "local-admin-password", AgentEnvPath: path,
	}, ":18900")
	if err != nil {
		t.Fatal(err)
	}
	if legacyTemporary != 0 {
		t.Fatalf("legacy temporary parties = %d, want 0", legacyTemporary)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("agent.env mode = %o, want 600", info.Mode().Perm())
	}
	agentConfig, err := agentprinter.ReadEnvironment(path, agentprinter.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if agentConfig.SiteID != 1 || agentConfig.SiteCode != "LOCAL01" || agentConfig.AgentID != "local-agent-1" || agentConfig.ServerURL != "http://127.0.0.1:18900" || agentConfig.SerialEnabled {
		t.Fatalf("agent config = %+v", agentConfig)
	}
	var identity nodesync.Identity
	if err := db.GORM.First(&identity, 1).Error; err != nil {
		t.Fatal(err)
	}
	if identity.Role != "standalone" || identity.SiteID != 1 || identity.Initialized != 0 {
		t.Fatalf("identity = %+v", identity)
	}
	var syncRows int64
	if err := db.GORM.Table("wb_sync_state").Count(&syncRows).Error; err != nil {
		t.Fatal(err)
	}
	if syncRows != 0 {
		t.Fatalf("sync state rows = %d, standalone must not create them", syncRows)
	}

	service, err := nodesync.New(db.GORM, config.NodeConfig{Role: "standalone", SiteID: 1}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.RefreshInitialized(ctx)
	if err != nil || ready {
		t.Fatalf("initial ready=%v err=%v, want false before second operator", ready, err)
	}
	router := gin.New()
	router.Use(service.Boundary)
	router.POST("/api/weighing/tasks", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/api/weighing/materials", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/weighing/tasks", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("formal write status=%d, want 403 before local activation", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/weighing/materials", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("management preparation status=%d, want 204 before local activation", response.Code)
	}

	var secondUserID int64
	if err := db.GORM.Raw("INSERT INTO sys_user (username,nickname,password,gender,dept_id,status,is_builtin,deleted,remark) SELECT 'local-operator-2','操作员乙',password,gender,dept_id,1,0,0,'integration' FROM sys_user WHERE id=1 RETURNING id").Scan(&secondUserID).Error; err != nil {
		t.Fatal(err)
	}
	if secondUserID < 2 {
		t.Fatalf("second user id = %d", secondUserID)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role (user_id,role_id) VALUES (?,1)", secondUserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO wb_user_site (user_id,site_id,can_operate) VALUES (?,1,1)", secondUserID).Error; err != nil {
		t.Fatal(err)
	}
	ready, err = service.RefreshInitialized(ctx)
	if err != nil || ready {
		t.Fatalf("ready=%v err=%v after one named non-built-in operator, want false", ready, err)
	}
	var thirdUserID int64
	if err := db.GORM.Raw("INSERT INTO sys_user (username,nickname,password,gender,dept_id,status,is_builtin,deleted,remark) SELECT 'local-operator-3','操作员丙',password,gender,dept_id,1,0,0,'integration' FROM sys_user WHERE id=1 RETURNING id").Scan(&thirdUserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role (user_id,role_id) VALUES (?,1)", thirdUserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO wb_user_site (user_id,site_id,can_operate) VALUES (?,1,1)", thirdUserID).Error; err != nil {
		t.Fatal(err)
	}
	ready, err = service.RefreshInitialized(ctx)
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v after two named non-built-in operators, want true", ready, err)
	}
	if err := db.GORM.First(&identity, 1).Error; err != nil {
		t.Fatal(err)
	}
	if identity.Initialized != 1 {
		t.Fatalf("identity initialized=%d, want 1", identity.Initialized)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/weighing/tasks", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("formal write status=%d, want 204 after local activation", response.Code)
	}

	reinitEnv := filepath.Join(t.TempDir(), "other.env")
	if _, err := nodesync.InitializeStandalone(ctx, db.GORM, config.NodeConfig{Role: "standalone", SiteID: 2}, nodesync.StandaloneInit{
		SiteCode: "LOCAL02", SiteName: "另一个厂", ScaleCode: "SCALE02", ScaleName: "二号地磅",
		AgentID: "local-agent-2", AdminPassword: "different-local-admin-password", AgentEnvPath: reinitEnv,
	}, ":18900"); err == nil {
		t.Fatal("reinitialization changed an existing site identity")
	}
	var adminPassword string
	if err := db.GORM.Table("sys_user").Select("password").Where("id=1").Scan(&adminPassword).Error; err != nil || adminPassword == initialPassword {
		t.Fatalf("successful initialization password hash was not rotated: %v", err)
	}
	if err := db.GORM.Table("sys_user").Select("password").Where("id=1").Scan(&afterFailurePassword).Error; err != nil || afterFailurePassword != adminPassword {
		t.Fatalf("reinitialization changed admin credential: %v", err)
	}
	if _, err := os.Stat(reinitEnv); !os.IsNotExist(err) {
		t.Fatalf("reinitialization wrote a second environment file: %v", err)
	}
}
