//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

type maintenanceIntegrationRunner struct{ active bool }

func (r *maintenanceIntegrationRunner) Run(args []string) agentmaintenance.Result {
	switch strings.Join(args, " ") {
	case "systemctl --user is-active weighbridge-agent-printer.service":
		if r.active {
			return agentmaintenance.Result{Output: "active"}
		}
		return agentmaintenance.Result{Output: "inactive"}
	case "systemctl --user start weighbridge-agent-printer.service":
		r.active = true
		return agentmaintenance.Result{Output: "started"}
	case "systemctl --user stop weighbridge-agent-printer.service":
		r.active = false
		return agentmaintenance.Result{Output: "stopped"}
	case "lpstat -r":
		return agentmaintenance.Result{Output: "scheduler is running"}
	case "lpstat -p":
		return agentmaintenance.Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
	case "lpstat -W not-completed -o":
		return agentmaintenance.Result{}
	default:
		return agentmaintenance.Result{}
	}
}

func TestPostgresBusinessMaintenanceJWTPermissionsAuditAndStoppedAgentStart(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("Docker is required for PostgreSQL integration tests")
	}
	temporary := startBusinessPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer database.Close()

	var scale struct{ ID int64 }
	if err := database.GORM.Table("wb_scale").Select("id").Where("code = ?", "SCALE01").Take(&scale).Error; err != nil {
		t.Fatalf("find seeded local scale: %v", err)
	}
	identity := agentprinter.Defaults()
	identity.SiteID, identity.SiteCode, identity.SiteName = 1, "SITE01", "原厂区"
	identity.ScaleID, identity.ScaleCode, identity.ScaleName = scale.ID, "SCALE01", "一号地磅"
	identity.ServerURL, identity.AgentKey = "http://127.0.0.1:18900", "local-maintenance-key"
	if err := database.GORM.Exec("UPDATE wb_site SET server_url = ? WHERE id = ?", identity.ServerURL, identity.SiteID).Error; err != nil {
		t.Fatalf("set local site server URL: %v", err)
	}
	if err := database.GORM.Exec("INSERT INTO wb_agent_binding (scale_id,agent_id,agent_key) VALUES (?,?,?) ON CONFLICT (scale_id) DO UPDATE SET agent_id=EXCLUDED.agent_id,agent_key=EXCLUDED.agent_key", identity.ScaleID, identity.AgentID, identity.AgentKey).Error; err != nil {
		t.Fatalf("bind local Agent identity: %v", err)
	}
	maintenanceDir := t.TempDir()
	maintenanceData, err := json.Marshal(map[string]any{"draft": identity, "active": identity})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(maintenanceDir, "maintenance.json"), maintenanceData, 0600); err != nil {
		t.Fatalf("write local maintenance fixture: %v", err)
	}
	runner := &maintenanceIntegrationRunner{}
	maintenance, err := agentmaintenance.Open(maintenanceDir, runner)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := testDependencies(t, database, t.TempDir())
	dependencies.Maintenance = maintenance
	repository := weighing.NewRepository(database.GORM)
	dependencies.MaintenanceBusinessValidator = func(ctx context.Context, candidate agentmaintenance.Config) error {
		return repository.ValidateMaintenanceConfig(ctx, candidate, identity.SiteID)
	}
	maintenance.SetBusinessConfigValidator(dependencies.MaintenanceBusinessValidator)
	dependencies.AuditRecorder = audit.NewRecorder(database.GORM)
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build business maintenance API: %v", err)
	}

	base := "/api/system/agent-maintenance"
	noToken := serveJSON(router, http.MethodGet, base+"/config", "", "")
	assertUnauthenticated(t, noToken)
	token := loginAdmin(t, router)
	permissionCodes := []string{agentmaintenance.PermissionView, agentmaintenance.PermissionManage}
	var permissionRows []struct{ ID int64 }
	if err := database.GORM.Table("sys_menu").Select("id").Where("permission_code IN ?", permissionCodes).Find(&permissionRows).Error; err != nil {
		t.Fatalf("find maintenance permission menus: %v", err)
	}
	if len(permissionRows) != 2 {
		t.Fatalf("maintenance permission seed rows = %d, want 2", len(permissionRows))
	}
	if err := database.GORM.Exec("DELETE FROM sys_role_menu WHERE role_id=1 AND menu_id IN (?,?)", permissionRows[0].ID, permissionRows[1].ID).Error; err != nil {
		t.Fatalf("remove maintenance permissions for RBAC check: %v", err)
	}
	denied := serveJSON(router, http.MethodGet, base+"/config", "", token)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("user without maintenance grants status = %d, body=%s", denied.Code, denied.Body.String())
	}
	if err := database.GORM.Exec("INSERT INTO sys_role_menu (role_id,menu_id) VALUES (1,?),(1,?) ON CONFLICT DO NOTHING", permissionRows[0].ID, permissionRows[1].ID).Error; err != nil {
		t.Fatalf("restore administrator maintenance permissions: %v", err)
	}

	read := serveJSON(router, http.MethodGet, base+"/config", "", token)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"environmentKnown"`) {
		t.Fatalf("read config while Agent and independent maintenance service are stopped: %d %s", read.Code, read.Body.String())
	}
	invalidIdentity := identity
	invalidIdentity.AgentID = "other-site-agent"
	invalidIdentity.AgentKey = ""
	invalidBody, _ := json.Marshal(invalidIdentity)
	deniedBinding := serveJSON(router, http.MethodPut, base+"/config", string(invalidBody), token)
	if deniedBinding.Code != http.StatusBadRequest || !strings.Contains(deniedBinding.Body.String(), "身份密钥") {
		t.Fatalf("reject maintenance rebinding before save: %d %s", deniedBinding.Code, deniedBinding.Body.String())
	}
	config := identity
	config.AgentKey = ""
	config.SiteName, config.ScaleName = "本地改名后的厂区", "本地改名后的地磅"
	configBody, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	saved := serveJSON(router, http.MethodPut, base+"/config", string(configBody), token)
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"saved":true`) {
		t.Fatalf("save draft while Agent is stopped: %d %s", saved.Code, saved.Body.String())
	}
	start := serveJSON(router, http.MethodPost, base+"/action", `{"action":"start-agent"}`, token)
	if start.Code != http.StatusOK || !runner.active {
		t.Fatalf("start Agent from business API: active=%t status=%d body=%s", runner.active, start.Code, start.Body.String())
	}
	jobs := serveJSON(router, http.MethodGet, base+"/jobs", "", token)
	var preview struct {
		Token string `json:"previewToken"`
	}
	if err := json.Unmarshal(jobs.Body.Bytes(), &preview); err != nil || preview.Token == "" {
		t.Fatalf("queue preview token missing before stop: %s (%v)", jobs.Body.String(), err)
	}
	stopBody := fmt.Sprintf(`{"action":"stop-agent","previewToken":%q,"disposition":"keep"}`, preview.Token)
	stop := serveJSON(router, http.MethodPost, base+"/action", stopBody, token)
	if stop.Code != http.StatusOK || runner.active {
		t.Fatalf("stop Agent with queue preview: active=%t status=%d body=%s", runner.active, stop.Code, stop.Body.String())
	}

	var operation struct {
		OperatorID int64  `gorm:"column:operator_id"`
		Action     string `gorm:"column:operation_type"`
		Resource   string `gorm:"column:module_name"`
		RequestURL string `gorm:"column:request_url"`
	}
	if err := database.GORM.Table("sys_oper_log").Select("operator_id,operation_type,module_name,request_url").Where("module_name='agent-maintenance' AND operation_type='maintenance.config.save'").Take(&operation).Error; err != nil {
		t.Fatalf("load business maintenance audit event: %v", err)
	}
	if operation.OperatorID != 1 || operation.Action != "maintenance.config.save" || operation.Resource != "agent-maintenance" || operation.RequestURL != base+"/config" {
		t.Fatalf("business maintenance audit lacks actual actor/route: %+v", operation)
	}
	var operationCount int64
	if err := database.GORM.Table("sys_oper_log").Where("module_name='agent-maintenance' AND operator_id=1").Count(&operationCount).Error; err != nil {
		t.Fatalf("count business maintenance audit events: %v", err)
	}
	if operationCount != 3 {
		t.Fatalf("business audit events = %d, want config save, Agent start and stop", operationCount)
	}
}
