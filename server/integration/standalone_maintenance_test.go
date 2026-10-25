//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"golang.org/x/net/websocket"
)

type maintenanceIntegrationRunner struct {
	active bool
}

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

func TestPostgresBusinessMaintenanceJWTPermissionsAuditAndStoppedDeviceAgentOnline(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("Docker is required for PostgreSQL integration tests")
	}
	temporary := startBusinessPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer database.Close()

	repository := weighing.NewRepository(database.GORM)
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleID int64) (agentbridge.Config, error) {
		return repository.AgentConfig(ctx, scaleID, t.TempDir())
	})
	dependencies := testDependencies(t, database, t.TempDir())
	dependencies.Weighing = weighing.NewService(repository, hub)
	dependencies.AuditRecorder = audit.NewRecorder(database.GORM)
	dependencies.RemoteMaintenance = agentmaintenance.NewRemote(hub, func(ctx context.Context) (agentmaintenance.Config, error) {
		status, err := localsite.ReadStatus(ctx, database.GORM)
		if err != nil || !status.Configured {
			return agentmaintenance.Config{}, fmt.Errorf("local site is not configured: %w", err)
		}
		return repository.BoundMaintenanceConfig(ctx, status.SiteID, 0, "http://127.0.0.1:18900")
	}, nil)
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build remote maintenance API: %v", err)
	}
	token := loginAdmin(t, router)
	ensureLocalSite(t, router, token, 1, "MAINT01", "维护测试厂", "MAINT-SCALE", "维护测试地磅", "maintenance-test-agent")
	deviceConfig, err := repository.DevelopmentMaintenanceConfig(context.Background(), 1, "http://127.0.0.1:18900")
	if err != nil {
		t.Fatalf("read DB-bound Agent config: %v", err)
	}
	deviceConfig.Queue = "Integration-Queue"
	deviceConfig.OffsetXMM = 7
	deviceConfig.CameraRTSPBase = "rtsp://127.0.0.1:8554"
	deviceConfig.SerialPort = "/dev/ttyUSB-integration"
	runner := &maintenanceIntegrationRunner{}
	device, err := agentmaintenance.OpenDevice(t.TempDir(), "", deviceConfig, runner)
	if err != nil {
		t.Fatalf("open Agent-owned maintenance service: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	if err := runRemoteMaintenanceAgent(t, server.URL, hub, deviceConfig, device, runner); err != nil {
		t.Fatal(err)
	}
	const base = "/api/system/agent-maintenance"
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, base+"/config", "", ""))
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
	if read.Code != http.StatusOK {
		t.Fatalf("read Agent-owned config: %d %s", read.Code, read.Body.String())
	}
	var state struct {
		Config           agentmaintenance.Config `json:"config"`
		EnvironmentKnown bool                    `json:"environmentKnown"`
		HasKey           bool                    `json:"hasAgentKey"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	// agent.env no longer stores display metadata, so the returned config has
	// no site identity; only the connection identity and parameter redaction matter.
	if !state.EnvironmentKnown || !state.HasKey || state.Config.AgentID != deviceConfig.AgentID || state.Config.AgentKey != "" {
		t.Fatalf("remote config did not redact identity key or report Agent-owned environment: %+v", state)
	}
	if strings.Contains(read.Body.String(), deviceConfig.AgentKey) {
		t.Fatal("maintenance response leaked the Agent connection key")
	}
	invalidIdentity := state.Config
	invalidIdentity.AgentID = "other-agent"
	invalidBody, _ := json.Marshal(invalidIdentity)
	deniedBinding := serveJSON(router, http.MethodPut, base+"/config", string(invalidBody), token)
	if deniedBinding.Code != http.StatusBadRequest || !strings.Contains(deniedBinding.Body.String(), "身份或归属") {
		t.Fatalf("reject maintenance rebinding before forwarding: %d %s", deniedBinding.Code, deniedBinding.Body.String())
	}
	state.Config.OffsetXMM = 9
	state.Config.AgentKey = ""
	configBody, err := json.Marshal(state.Config)
	if err != nil {
		t.Fatal(err)
	}
	saved := serveJSON(router, http.MethodPut, base+"/config", string(configBody), token)
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"saved":true`) {
		t.Fatalf("save Agent-owned parameter draft: %d %s", saved.Code, saved.Body.String())
	}
	read = serveJSON(router, http.MethodGet, base+"/config", "", token)
	if err := json.Unmarshal(read.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Config.OffsetXMM != 9 || state.Config.Queue != deviceConfig.Queue || state.Config.SerialPort != deviceConfig.SerialPort || state.Config.CameraRTSPBase != deviceConfig.CameraRTSPBase {
		t.Fatalf("remote save lost printer/camera/serial parameters: %+v", state.Config)
	}
	bindingAfterSave, err := repository.AgentConfig(context.Background(), deviceConfig.ScaleID, "")
	if err != nil || bindingAfterSave.Key != deviceConfig.AgentKey || bindingAfterSave.ID != deviceConfig.AgentID {
		t.Fatal("remote maintenance changed the DB-managed Agent binding", err)
	}
	var initialized int16
	if err := database.GORM.Table("wb_local_identity").Select("initialized").Where("id=1 AND site_id=1").Scan(&initialized).Error; err != nil || initialized != 0 {
		t.Fatalf("maintenance must not write the legacy initialization flag: initialized=%d err=%v", initialized, err)
	}
	var siteURL string
	if err := database.GORM.Table("wb_site").Select("server_url").Where("id=1").Scan(&siteURL).Error; err != nil || siteURL != "" {
		t.Fatalf("legacy site address must stay unused and unwritten: %q err=%v", siteURL, err)
	}

	start := serveJSON(router, http.MethodPost, base+"/action", `{"action":"start-agent"}`, token)
	if start.Code != http.StatusOK || !runner.active || !hub.Connected(1) {
		t.Fatalf("start device execution over online Agent: active=%t connected=%t status=%d body=%s", runner.active, hub.Connected(1), start.Code, start.Body.String())
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
	if stop.Code != http.StatusOK || runner.active || !hub.Connected(1) {
		t.Fatalf("stop device execution without stopping Agent process: active=%t connected=%t status=%d body=%s", runner.active, hub.Connected(1), stop.Code, stop.Body.String())
	}
	status := serveJSON(router, http.MethodGet, base+"/status", "", token)
	var statusPayload struct {
		Online bool `json:"agentProcessOnline"`
		Remote bool `json:"remote"`
	}
	if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &statusPayload) != nil || !statusPayload.Online || !statusPayload.Remote {
		t.Fatalf("stopped device must remain remotely maintainable while Agent is connected: %d %s", status.Code, status.Body.String())
	}
	readAfterStop := serveJSON(router, http.MethodGet, base+"/config", "", token)
	if readAfterStop.Code != http.StatusOK {
		t.Fatalf("read maintenance config with device execution stopped: %d %s", readAfterStop.Code, readAfterStop.Body.String())
	}

	var operation struct {
		OperatorID int64  `gorm:"column:operator_id"`
		Action     string `gorm:"column:operation_type"`
		Resource   string `gorm:"column:module_name"`
		RequestURL string `gorm:"column:request_url"`
	}
	if err := database.GORM.Table("sys_oper_log").Select("operator_id,operation_type,module_name,request_url").Where("module_name='agent-maintenance' AND operation_type='maintenance/config'").Take(&operation).Error; err != nil {
		t.Fatalf("load remote maintenance audit event: %v", err)
	}
	if operation.OperatorID != 1 || operation.Action != "maintenance/config" || operation.Resource != "agent-maintenance" || operation.RequestURL != base+"/config" {
		t.Fatalf("remote maintenance audit lacks actual actor/route: %+v", operation)
	}
	var operationCount int64
	if err := database.GORM.Table("sys_oper_log").Where("module_name='agent-maintenance' AND operator_id=1").Count(&operationCount).Error; err != nil {
		t.Fatalf("count remote maintenance audit events: %v", err)
	}
	if operationCount != 3 {
		t.Fatalf("business audit events = %d, want config save, Agent start and stop", operationCount)
	}
}

func runRemoteMaintenanceAgent(t *testing.T, serverURL string, hub *agentbridge.Hub, cfg agentprinter.Config, device *agentmaintenance.Service, runner *maintenanceIntegrationRunner) error {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/agent/ws"
	wsConfig, err := websocket.NewConfig(wsURL, serverURL)
	if err != nil {
		return err
	}
	wsConfig.Header = http.Header{
		"Authorization": []string{"Bearer " + cfg.AgentKey},
		"X-Agent-Id":    []string{cfg.AgentID},
		"X-Scale-Id":    []string{fmt.Sprint(cfg.ScaleID)},
	}
	connection, err := websocket.DialConfig(wsConfig)
	if err != nil {
		return fmt.Errorf("connect fake Agent to API: %w", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := websocket.JSON.Send(connection, p.New("maintenance.state", agentbridge.ID(), "", p.MaintenanceState{Config: cfg, Running: runner.active})); err != nil {
		return fmt.Errorf("send initial maintenance state: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		for {
			var message p.Message
			if err := websocket.JSON.Receive(connection, &message); err != nil {
				if ctx.Err() != nil {
					done <- nil
				} else {
					done <- err
				}
				return
			}
			if message.Type != "maintenance.request" {
				continue
			}
			var request p.MaintenanceRequest
			if err := json.Unmarshal(message.Payload, &request); err != nil {
				done <- err
				return
			}
			response := device.DeviceRequest(ctx, request)
			if err := websocket.JSON.Send(connection, p.New("maintenance.state", agentbridge.ID(), "", p.MaintenanceState{Config: cfg, Running: runner.active})); err != nil {
				done <- err
				return
			}
			if err := websocket.JSON.Send(connection, p.New("maintenance.result", agentbridge.ID(), message.RequestID, response)); err != nil {
				done <- err
				return
			}
		}
	}()
	if hub != nil {
		deadline := time.Now().Add(2 * time.Second)
		for !hub.Connected(cfg.ScaleID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if !hub.Connected(cfg.ScaleID) {
			_ = connection.Close()
			return fmt.Errorf("API did not register the remote Agent connection")
		}
	} else {
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}
