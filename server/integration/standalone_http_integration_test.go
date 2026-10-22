//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/nodesync"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Two isolated installations use their stable local site IDs against the same
// seeded schema. HTTP, JWT, RBAC, Agent websocket evidence and ticket snapshots
// all pass through app.Build; no central or sync service participates.
func TestPostgresStandaloneTwoFactoriesHTTPAndWeighing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	type site struct {
		id                int64
		name, code, plate string
		key               string
		db                *platformdatabase.Database
		router            http.Handler
		token             string
		hub               *agentbridge.Hub
		mock              *standaloneMock
	}
	sites := []*site{}
	for _, spec := range []struct {
		id                int64
		name, code, plate string
	}{{1, "一厂", "LOCAL-A", "晋A66666"}, {2, "二厂", "LOCAL-B", "晋A66666"}} {
		postgres := startBusinessPostgres(t)
		db := openTemporaryDatabase(t, postgres.dsn)
		t.Cleanup(func() { _ = db.Close() })
		envPath := filepath.Join(t.TempDir(), "agent.env")
		if _, err := nodesync.InitializeStandalone(ctx, db.GORM, config.NodeConfig{Role: "standalone", SiteID: spec.id}, nodesync.StandaloneInit{
			SiteCode: spec.code, SiteName: spec.name, ScaleCode: fmt.Sprintf("SCALE-%d", spec.id), ScaleName: spec.name + "地磅",
			AgentID: fmt.Sprintf("standalone-agent-%d", spec.id), AdminPassword: "local-admin-password-123", AgentEnvPath: envPath,
		}, ":18900"); err != nil {
			t.Fatalf("initialize %s: %v", spec.name, err)
		}
		agentConfig, err := agentprinter.ReadEnvironment(envPath, agentprinter.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		node, err := nodesync.New(db.GORM, config.NodeConfig{Role: "standalone", SiteID: spec.id}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		deps := testDependencies(t, db, t.TempDir())
		deps.Node = node
		repo := weighing.NewRepository(db.GORM)
		bindings, err := repo.AgentConfigs(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if len(bindings) != 1 || bindings[0].ScaleID != spec.id {
			t.Fatalf("site %d bindings = %+v", spec.id, bindings)
		}
		hub := agentbridge.NewMultiple(bindings)
		deps.Weighing = weighing.NewService(repo, hub)
		apiCfg := testAPIConfig()
		apiCfg.Node = config.NodeConfig{Role: "standalone", SiteID: spec.id}
		router, err := app.Build(apiCfg, db, deps)
		if err != nil {
			t.Fatal(err)
		}
		admin := loginUser(t, router, "admin", "local-admin-password-123")
		old := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"admin123"}`, "")
		var oldBody struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(old.Body.Bytes(), &oldBody); err != nil {
			t.Fatal(err)
		}
		if oldBody.Code == 200 {
			t.Fatalf("site %d kept seeded admin password", spec.id)
		}
		status := serveJSON(router, http.MethodGet, "/api/node/status", "", admin)
		if status.Code != http.StatusOK {
			t.Fatalf("site %d node status: %d %s", spec.id, status.Code, status.Body.String())
		}
		var statusBody struct {
			Data struct {
				Role        string `json:"role"`
				SiteID      int64  `json:"siteId"`
				Initialized bool   `json:"initialized"`
				Sites       []any  `json:"sites"`
			} `json:"data"`
		}
		if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil {
			t.Fatal(err)
		}
		if statusBody.Data.Role != "standalone" || statusBody.Data.SiteID != spec.id || statusBody.Data.Initialized || len(statusBody.Data.Sites) != 0 {
			t.Fatalf("site %d status payload: %+v", spec.id, statusBody.Data)
		}
		denied := serveJSON(router, http.MethodPost, "/api/weighing/tasks/first-confirmation", `{}`, admin)
		if denied.Code != http.StatusForbidden {
			t.Fatalf("site %d formal write before activation = %d: %s", spec.id, denied.Code, denied.Body.String())
		}
		// Create two named operators through the system API, assign the seeded
		// weighing role and grant only this factory through the existing RBAC API.
		roleCode := fmt.Sprintf("LOCAL_WEIGHING_%d", spec.id)
		createdRole := serveJSON(router, http.MethodPost, "/api/system/role", fmt.Sprintf(`{"roleName":%q,"roleCode":%q,"status":1}`, "本厂称重操作员", roleCode), admin)
		if createdRole.Code != http.StatusOK {
			t.Fatalf("create role: %d %s", createdRole.Code, createdRole.Body.String())
		}
		var roleID, confirmMenuID int64
		if err := db.GORM.Table("sys_role").Select("id").Where("role_code=?", roleCode).Scan(&roleID).Error; err != nil || roleID == 0 {
			t.Fatalf("local role id=%d err=%v", roleID, err)
		}
		if err := db.GORM.Table("sys_menu").Select("id").Where("permission_code='weighing:confirm' AND status=1 AND deleted=0").Scan(&confirmMenuID).Error; err != nil || confirmMenuID == 0 {
			t.Fatalf("confirm menu id=%d err=%v", confirmMenuID, err)
		}
		roleMenus := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/role/%d/menus", roleID), fmt.Sprintf(`{"menuIds":[%d]}`, confirmMenuID), admin)
		if roleMenus.Code != http.StatusOK {
			t.Fatalf("assign confirm menu: %d %s", roleMenus.Code, roleMenus.Body.String())
		}
		var firstOperatorID int64
		for i, nickname := range []string{"实名操作员甲", "实名操作员乙"} {
			username := fmt.Sprintf("operator-%d-%d", spec.id, i)
			created := serveJSON(router, http.MethodPost, "/api/system/user", fmt.Sprintf(`{"username":%q,"nickname":%q,"status":1}`, username, nickname), admin)
			if created.Code != http.StatusOK {
				t.Fatalf("create user %s: %d %s", username, created.Code, created.Body.String())
			}
			var userID int64
			if err := db.GORM.Table("sys_user").Select("id").Where("username=?", username).Scan(&userID).Error; err != nil || userID < 2 {
				t.Fatalf("user %s id=%d err=%v", username, userID, err)
			}
			if i == 0 {
				firstOperatorID = userID
			}
			roles := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/user/%d/roles", userID), fmt.Sprintf(`{"roleIds":[%d]}`, roleID), admin)
			if roles.Code != http.StatusOK {
				t.Fatalf("assign role: %d %s", roles.Code, roles.Body.String())
			}
			grant := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/users/%d/sites", userID), fmt.Sprintf(`{"grants":[{"siteId":%d,"canOperate":1}]}`, spec.id), admin)
			if grant.Code != http.StatusOK {
				t.Fatalf("grant local site: %d %s", grant.Code, grant.Body.String())
			}
		}
		operator := loginUser(t, router, fmt.Sprintf("operator-%d-0", spec.id), "admin123")
		deniedMaster := serveJSON(router, http.MethodPost, "/api/weighing/materials", `{"name":"越权资料"}`, operator)
		assertEnvelopeCode(t, deniedMaster, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		ready, err := node.RefreshInitialized(ctx)
		if err != nil || !ready {
			t.Fatalf("site %d activation ready=%v err=%v", spec.id, ready, err)
		}
		oldSession := weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%d/session/acquire", spec.id), fmt.Sprintf(`{"clientId":"grant-revocation-%d"}`, spec.id), operator)
		userSitesPath := fmt.Sprintf("/api/weighing/users/%d/sites", firstOperatorID)
		revoked := serveJSON(router, http.MethodPut, userSitesPath, `{"grants":[]}`, admin)
		if revoked.Code != http.StatusOK {
			t.Fatalf("revoke all local grants: %d %s", revoked.Code, revoked.Body.String())
		}
		var grantCount int64
		if err := db.GORM.Table("wb_user_site").Where("user_id=?", firstOperatorID).Count(&grantCount).Error; err != nil || grantCount != 0 {
			t.Fatalf("revoked user has %d site grants, err=%v", grantCount, err)
		}
		oldRenew := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%d/session/renew", spec.id), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		assertEnvelopeCode(t, oldRenew, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		regranted := serveJSON(router, http.MethodPut, userSitesPath, fmt.Sprintf(`{"grants":[{"siteId":%d,"canOperate":1}]}`, spec.id), admin)
		if regranted.Code != http.StatusOK {
			t.Fatalf("restore local grant: %d %s", regranted.Code, regranted.Body.String())
		}
		if err := db.GORM.Table("wb_user_site").Where("user_id=? AND site_id=? AND can_operate=1", firstOperatorID, spec.id).Count(&grantCount).Error; err != nil || grantCount != 1 {
			t.Fatalf("regranted local site count=%d err=%v", grantCount, err)
		}
		weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%d/session/renew", spec.id), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%d/session/release", spec.id), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		// A grant to another factory and an attempt to mutate its configuration
		// are rejected even for the local administrator.
		crossGrant := serveJSON(router, http.MethodPut, "/api/weighing/users/2/sites", fmt.Sprintf(`{"grants":[{"siteId":%d,"canOperate":1}]}`, 3-spec.id), admin)
		assertEnvelopeCode(t, crossGrant, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		crossConfig := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/site-config/%d", 3-spec.id), `{"code":"OTHER","name":"他厂","scaleCode":"OTHER-SCALE","scaleName":"他磅","agentId":"other-agent","enabled":1,"version":1}`, admin)
		assertEnvelopeCode(t, crossConfig, http.StatusNotFound, http.StatusNotFound, "数据不存在")
		var foreignCode string
		if err := db.GORM.Table("wb_site").Select("code").Where("id=?", 3-spec.id).Scan(&foreignCode).Error; err != nil || foreignCode == "OTHER" {
			t.Fatalf("cross-site config changed foreign site code=%q err=%v", foreignCode, err)
		}
		sites = append(sites, &site{id: spec.id, name: spec.name, code: spec.code, plate: spec.plate, key: agentConfig.AgentKey, db: db, router: router, token: admin, hub: hub})
	}
	for _, s := range sites {
		s.mock = startStandaloneMock(t, s.router, s.hub, s.id, fmt.Sprintf("standalone-agent-%d", s.id), s.key)
	}
	results := make([]weighing.TicketView, 0, 2)
	taskNumbers := make([]string, 0, 2)
	for _, s := range sites {
		ticket := standaloneRoundTrip(t, s.router, s.token, s.id, s.plate, s.mock)
		results = append(results, ticket)
		var taskNo string
		if err := s.db.GORM.Table("wb_transport_task").Select("task_no").Where("id=?", ticket.TaskID).Scan(&taskNo).Error; err != nil {
			t.Fatal(err)
		}
		taskNumbers = append(taskNumbers, taskNo)
	}
	if results[0].ID == results[1].ID || results[0].TaskID == results[1].TaskID || results[0].TicketNo == results[1].TicketNo || taskNumbers[0] == taskNumbers[1] {
		t.Fatalf("factory identities/numbers collided: A=%+v task=%s B=%+v task=%s", results[0], taskNumbers[0], results[1], taskNumbers[1])
	}
	if results[0].SiteName != "一厂" || results[1].SiteName != "二厂" {
		t.Fatalf("ticket snapshots lost local site identity: %q / %q", results[0].SiteName, results[1].SiteName)
	}
}

type standaloneMock struct {
	t       *testing.T
	control string
	hub     *agentbridge.Hub
	scaleID int64
}

func startStandaloneMock(t *testing.T, router http.Handler, hub *agentbridge.Hub, scaleID int64, agentID, key string) *standaloneMock {
	t.Helper()
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	mock := agentmock.New(agentmock.Config{Server: server.URL, ID: agentID, ScaleID: scaleID, Key: key})
	control := httptest.NewServer(mock.Handler())
	t.Cleanup(control.Close)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); mock.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Error("standalone mock did not stop")
		}
	})
	a := &standaloneMock{t: t, control: control.URL, hub: hub, scaleID: scaleID}
	standaloneMocks[scaleID] = a
	t.Cleanup(func() { delete(standaloneMocks, scaleID) })
	a.weight("30")
	return a
}
func (a *standaloneMock) controlAction(body string) {
	a.t.Helper()
	response, err := http.Post(a.control+"/api/control", "application/json", strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		a.t.Fatalf("mock control: %d", response.StatusCode)
	}
}
func (a *standaloneMock) weight(weight string) {
	a.t.Helper()
	a.controlAction(fmt.Sprintf(`{"action":"up","weight":%q}`, weight))
	a.controlAction(`{"action":"stable"}`)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		state := a.hub.State(a.scaleID)
		if state.Online && state.Stable && state.Weight.String() == weight && len(state.Images) == 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.t.Fatalf("mock Agent not ready: %+v", a.hub.State(a.scaleID))
}
func standaloneRoundTrip(t *testing.T, router http.Handler, token string, scaleID int64, plate string, device *standaloneMock) weighing.TicketView {
	t.Helper()
	material := weighingData[weighing.Material](t, router, http.MethodPost, "/api/weighing/materials", `{"name":"本地物料"}`, token)
	party := weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/parties", `{"name":"本地客户","nature":"enterprise","roles":["supplier","customer"]}`, token)
	client := fmt.Sprintf("factory-%d", scaleID)
	base := map[string]any{"scaleId": scaleID, "clientId": client}
	session := weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%d/session/acquire", scaleID), encodeStandalone(t, base), token)
	business := map[string]any{"plate": plate, "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
	firstSampleInput := map[string]any{"scaleId": scaleID, "clientId": client, "sessionToken": session.SessionToken, "business": business, "source": "device"}
	firstSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeStandalone(t, firstSampleInput), token)
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	firstInput := map[string]any{"scaleId": scaleID, "clientId": client, "sessionToken": session.SessionToken, "business": business, "sampleId": firstSample.ID, "requestId": fmt.Sprintf("standalone-first-%d", scaleID), "checks": checks}
	first := weighingData[weighing.CommandResult](t, router, http.MethodPost, "/api/weighing/tasks/first-confirmation", encodeStandalone(t, firstInput), token)
	// The mock is independently controlled per factory; a new stable reading
	// establishes the tare capture after the gross snapshot has been committed.
	firstSampleInput["taskId"] = first.TaskID
	// Find this site mock by changing its control endpoint through hub state is
	// unnecessary here: helper below is registered in the global map per call.
	device.weight("10")
	secondSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeStandalone(t, firstSampleInput), token)
	secondInput := map[string]any{"scaleId": scaleID, "clientId": client, "sessionToken": session.SessionToken, "taskId": first.TaskID, "sampleId": secondSample.ID, "requestId": fmt.Sprintf("standalone-second-%d", scaleID), "version": first.TaskVersion, "checks": checks, "finalWeights": map[string]any{"gross": "30", "tare": "10", "remark": ""}}
	completed := weighingData[weighing.CommandResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", first.TaskID), encodeStandalone(t, secondInput), token)
	if completed.TicketID == 0 || completed.TaskID != first.TaskID {
		t.Fatalf("factory %d completion: %+v", scaleID, completed)
	}
	ticket := weighingData[weighing.TicketView](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/tickets/%d", completed.TicketID), "", token)
	if ticket.SiteID != scaleID || ticket.OriginalGross.String() != "30" || ticket.OriginalTare.String() != "10" {
		t.Fatalf("factory %d ticket snapshot: %+v", scaleID, ticket)
	}
	return ticket
}

var standaloneMocks = map[int64]*standaloneMock{}

func encodeStandalone(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
