//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Two isolated installations use their head-office site codes against the same
// seeded schema. HTTP, JWT, RBAC, Agent websocket evidence and ticket snapshots
// all pass through app.Build; each factory has an independent local database.
func TestPostgresIndependentLocalSitesHTTPAndWeighing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	type site struct {
		id                           int64
		name, code, plate, scaleCode string
		agentID                      string
		key                          string
		db                           *platformdatabase.Database
		router                       http.Handler
		token                        string
		hub                          *agentbridge.Hub
		mock                         *localSiteMock
	}
	sites := []*site{}
	for _, spec := range []struct {
		id                           int64
		name, code, plate, scaleCode string
	}{{1, "一厂", "LOCAL-A", "晋A66666", "SCALE-A"}, {2, "二厂", "LOCAL-B", "晋A66666", "SCALE-B"}} {
		postgres := startBusinessPostgres(t)
		db := openTemporaryDatabase(t, postgres.dsn)
		t.Cleanup(func() { _ = db.Close() })
		deps := testDependencies(t, db, t.TempDir())
		repo := weighing.NewRepository(db.GORM)
		hub := agentbridge.NewMultiple(nil)
		hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
			return repo.AgentConfig(ctx, scaleCode, t.TempDir())
		})
		deps.Weighing = weighing.NewService(repo, hub)
		deps.Camera, _, _ = newCaptureService(t, db, t.TempDir())
		router, err := app.Build(testAPIConfig(), db, deps)
		if err != nil {
			t.Fatal(err)
		}
		admin := loginAdmin(t, router)
		ensureLocalSite(t, router, admin, spec.id, spec.code, spec.name, spec.scaleCode, spec.name+"地磅", "integration-agent")
		addCaptureCameras(t, db, "rtsp://camera/site-1", "rtsp://camera/site-2")
		agentConfig, err := repo.AgentConfig(ctx, spec.scaleCode, t.TempDir())
		if err != nil {
			t.Fatalf("site %d agent binding: %v", spec.id, err)
		}
		// Create one named operator through the system API, assign the seeded
		// weighing role and grant only this factory through the existing RBAC API.
		// One authorized account is enough; no second account gates business.
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
		for i, nickname := range []string{"实名操作员甲"} {
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
			grant := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/users/%d/sites", userID), fmt.Sprintf(`{"grants":[{"siteCode":%q,"canOperate":1}]}`, spec.code), admin)
			if grant.Code != http.StatusOK {
				t.Fatalf("grant local site: %d %s", grant.Code, grant.Body.String())
			}
		}
		operator := loginUser(t, router, fmt.Sprintf("operator-%d-0", spec.id), "admin123")
		deniedMaster := serveJSON(router, http.MethodPost, "/api/weighing/materials", `{"name":"越权资料"}`, operator)
		assertEnvelopeCode(t, deniedMaster, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		ready := weighingData[struct {
			Initialized bool `json:"initialized"`
		}](t, router, http.MethodGet, "/api/weighing/local-status", "", admin)
		if !ready.Initialized {
			t.Fatalf("site %d local identity did not complete from the first configuration transaction", spec.id)
		}
		oldSession := weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%s/session/acquire", spec.scaleCode), fmt.Sprintf(`{"clientId":"grant-revocation-%d"}`, spec.id), operator)
		userSitesPath := fmt.Sprintf("/api/weighing/users/%d/sites", firstOperatorID)
		revoked := serveJSON(router, http.MethodPut, userSitesPath, `{"grants":[]}`, admin)
		if revoked.Code != http.StatusOK {
			t.Fatalf("revoke all local grants: %d %s", revoked.Code, revoked.Body.String())
		}
		var grantCount int64
		if err := db.GORM.Table("wb_local_user_access").Where("user_id=?", firstOperatorID).Count(&grantCount).Error; err != nil || grantCount != 0 {
			t.Fatalf("revoked user has %d site grants, err=%v", grantCount, err)
		}
		oldRenew := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%s/session/renew", spec.scaleCode), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		assertEnvelopeCode(t, oldRenew, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		regranted := serveJSON(router, http.MethodPut, userSitesPath, fmt.Sprintf(`{"grants":[{"siteCode":%q,"canOperate":1}]}`, spec.code), admin)
		if regranted.Code != http.StatusOK {
			t.Fatalf("restore local grant: %d %s", regranted.Code, regranted.Body.String())
		}
		if err := db.GORM.Table("wb_local_user_access").Where("user_id=? AND can_operate=1", firstOperatorID).Count(&grantCount).Error; err != nil || grantCount != 1 {
			t.Fatalf("regranted local site count=%d err=%v", grantCount, err)
		}
		weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%s/session/renew", spec.scaleCode), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%s/session/release", spec.scaleCode), fmt.Sprintf(`{"clientId":"grant-revocation-%d","sessionToken":%q}`, spec.id, oldSession.SessionToken), operator)
		// A grant to another factory and an attempt to replace the existing
		// Agent identifier are rejected even for the local administrator.
		crossGrant := serveJSON(router, http.MethodPut, "/api/weighing/users/2/sites", `{"grants":[{"siteCode":"OTHER","canOperate":1}]}`, admin)
		assertEnvelopeCode(t, crossGrant, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
		crossConfig := serveJSON(router, http.MethodPut, "/api/weighing/site-config", `{"code":"OTHER","name":"他厂","scaleCode":"OTHER-SCALE","scaleName":"他磅","agentId":"other-agent","enabled":1,"version":1}`, admin)
		assertEnvelopeCode(t, crossConfig, http.StatusOK, http.StatusBadRequest, "已有 Agent 编号不可修改，请保留原身份")
		var siteCount int64
		if err := db.GORM.Table("wb_site").Where("site_code=?", "OTHER").Count(&siteCount).Error; err != nil || siteCount != 0 {
			t.Fatalf("cross-site config created a foreign site count=%d err=%v", siteCount, err)
		}
		sites = append(sites, &site{id: spec.id, name: spec.name, code: spec.code, plate: spec.plate, scaleCode: spec.scaleCode, agentID: agentConfig.ID, key: agentConfig.Key, db: db, router: router, token: admin, hub: hub})
	}
	for _, s := range sites {
		s.mock = startLocalSiteMock(t, s.router, s.hub, s.scaleCode, s.agentID, s.key)
	}
	results := make([]weighing.TicketView, 0, 2)
	taskNumbers := make([]string, 0, 2)
	for _, s := range sites {
		ticket := localSiteRoundTrip(t, s.router, s.token, s.token, s.code, s.scaleCode, s.plate, s.mock)
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

type localSiteMock struct {
	t         *testing.T
	control   string
	hub       *agentbridge.Hub
	scaleCode string
}

func startLocalSiteMock(t *testing.T, router http.Handler, hub *agentbridge.Hub, scaleCode string, agentID, key string) *localSiteMock {
	t.Helper()
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	mock := agentmock.New(agentmock.Config{Server: server.URL, ID: agentID, ScaleCode: scaleCode, Key: key})
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
			t.Error("local site mock did not stop")
		}
	})
	a := &localSiteMock{t: t, control: control.URL, hub: hub, scaleCode: scaleCode}
	a.weight("30")
	return a
}
func (a *localSiteMock) controlAction(body string) {
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
func (a *localSiteMock) weight(weight string) {
	a.t.Helper()
	a.controlAction(fmt.Sprintf(`{"action":"up","weight":%q}`, weight))
	a.controlAction(`{"action":"stable"}`)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		state := a.hub.State(a.scaleCode)
		if state.Online && state.Stable && state.Weight.String() == weight {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.t.Fatalf("mock Agent not ready: %+v", a.hub.State(a.scaleCode))
}
func localSiteRoundTrip(t *testing.T, router http.Handler, masterToken, operatorToken, siteCode, scaleCode, plate string, device *localSiteMock) weighing.TicketView {
	t.Helper()
	material := weighingData[weighing.Material](t, router, http.MethodPost, "/api/weighing/materials", `{"name":"本地物料"}`, masterToken)
	party := weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/parties", `{"name":"本地客户","nature":"enterprise","roles":["supplier","customer"]}`, masterToken)
	client := fmt.Sprintf("factory-%s", scaleCode)
	base := map[string]any{"scaleCode": scaleCode, "clientId": client}
	session := weighingData[weighing.SessionResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/scales/%s/session/acquire", scaleCode), encodeLocalSite(t, base), operatorToken)
	business := map[string]any{"plate": plate, "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
	firstSampleInput := map[string]any{"scaleCode": scaleCode, "clientId": client, "sessionToken": session.SessionToken, "business": business, "source": "device"}
	firstSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, firstSampleInput), operatorToken)
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	firstInput := map[string]any{"scaleCode": scaleCode, "clientId": client, "sessionToken": session.SessionToken, "business": business, "sampleId": firstSample.ID, "requestId": fmt.Sprintf("local-first-%s-%s", scaleCode, plate), "checks": checks}
	first := weighingData[weighing.CommandResult](t, router, http.MethodPost, "/api/weighing/tasks/first-confirmation", encodeLocalSite(t, firstInput), operatorToken)
	// The mock is independently controlled per factory; a new stable reading
	// establishes the tare capture after the gross snapshot has been committed.
	firstSampleInput["taskId"] = first.TaskID
	// Find this site mock by changing its control endpoint through hub state is
	// unnecessary because the local Agent mock is passed directly.
	device.weight("10")
	secondSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, firstSampleInput), operatorToken)
	secondInput := map[string]any{"scaleCode": scaleCode, "clientId": client, "sessionToken": session.SessionToken, "taskId": first.TaskID, "sampleId": secondSample.ID, "requestId": fmt.Sprintf("local-second-%s-%s", scaleCode, plate), "version": first.TaskVersion, "checks": checks, "finalWeights": map[string]any{"gross": "30", "tare": "10", "remark": ""}}
	completed := weighingData[weighing.CommandResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", first.TaskID), encodeLocalSite(t, secondInput), operatorToken)
	if completed.TicketID == 0 || completed.TaskID != first.TaskID {
		t.Fatalf("factory %s completion: %+v", scaleCode, completed)
	}
	ticket := weighingData[weighing.TicketView](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/tickets/%d", completed.TicketID), "", operatorToken)
	if ticket.SiteCode != siteCode || ticket.OriginalGross.String() != "30" || ticket.OriginalTare.String() != "10" {
		t.Fatalf("factory %s ticket snapshot: %+v", scaleCode, ticket)
	}
	return ticket
}

func encodeLocalSite(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// One named operator with the weighing role and this factory's operate scope is
// enough to complete formal weighing. The first identity transaction already
// completes initialization, and the existing RBAC/scope/status checks still
// reject accounts without permission, scope or an enabled status.
func TestPostgresSingleAuthorizedOperatorCompletesWeighing(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	repository := weighing.NewRepository(db.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repository.AgentConfig(ctx, scaleCode, imageDir)
	})
	deps.Weighing = weighing.NewService(repository, hub)
	deps.Camera, _, _ = newCaptureService(t, db, t.TempDir())
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, "SOLO01", "单人称重厂", "SOLO-SCALE", "单人称重地磅", "solo-agent")
	addCaptureCameras(t, db, "rtsp://camera/solo-1", "rtsp://camera/solo-2")
	status := weighingData[struct {
		Configured  bool `json:"configured"`
		Initialized bool `json:"initialized"`
	}](t, router, http.MethodGet, "/api/weighing/local-status", "", admin)
	if !status.Configured || !status.Initialized {
		t.Fatalf("first configuration transaction must complete initialization before any operator exists: %+v", status)
	}
	var roleID int64
	if err := db.GORM.Table("sys_role").Select("id").Where("role_code='WEIGHING_OPERATOR' AND status=1 AND deleted=0").Scan(&roleID).Error; err != nil || roleID == 0 {
		t.Fatalf("seeded weighing role id=%d err=%v", roleID, err)
	}
	createUser := func(username, nickname string, withRole, withGrant bool) int64 {
		t.Helper()
		created := serveJSON(router, http.MethodPost, "/api/system/user", fmt.Sprintf(`{"username":%q,"nickname":%q,"status":1}`, username, nickname), admin)
		if created.Code != http.StatusOK {
			t.Fatalf("create user %s: %d %s", username, created.Code, created.Body.String())
		}
		var userID int64
		if err := db.GORM.Table("sys_user").Select("id").Where("username=?", username).Scan(&userID).Error; err != nil || userID < 2 {
			t.Fatalf("user %s id=%d err=%v", username, userID, err)
		}
		if withRole {
			if response := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/user/%d/roles", userID), fmt.Sprintf(`{"roleIds":[%d]}`, roleID), admin); response.Code != http.StatusOK {
				t.Fatalf("assign role to %s: %d %s", username, response.Code, response.Body.String())
			}
		}
		if withGrant {
			if response := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/weighing/users/%d/sites", userID), `{"grants":[{"siteCode":"SOLO01","canOperate":1}]}`, admin); response.Code != http.StatusOK {
				t.Fatalf("grant site to %s: %d %s", username, response.Code, response.Body.String())
			}
		}
		return userID
	}
	createUser("solo-operator", "单人称重操作员", true, true)
	operator := loginUser(t, router, "solo-operator", "admin123")

	acquire := func(token string) *httptest.ResponseRecorder {
		return serveJSON(router, http.MethodPost, "/api/weighing/scales/SOLO-SCALE/session/acquire", `{"clientId":"solo-check"}`, token)
	}
	createUser("solo-no-grant", "无本厂操作范围", true, false)
	assertEnvelopeCode(t, acquire(loginUser(t, router, "solo-no-grant", "admin123")), http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
	createUser("solo-no-role", "无称重权限", false, true)
	assertEnvelopeCode(t, acquire(loginUser(t, router, "solo-no-role", "admin123")), http.StatusForbidden, http.StatusForbidden, "无权执行此操作")

	agentConfig, err := repository.AgentConfig(context.Background(), "SOLO-SCALE", imageDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startLocalSiteMock(t, router, hub, "SOLO-SCALE", agentConfig.ID, agentConfig.Key)
	ticket := localSiteRoundTrip(t, router, admin, operator, "SOLO01", "SOLO-SCALE", "晋A11111", device)
	if ticket.SiteCode != "SOLO01" || ticket.ID == 0 || ticket.TaskID == 0 {
		t.Fatalf("single-operator ticket = %+v", ticket)
	}

	disabledID := createUser("solo-disabled", "停用操作员", true, true)
	if err := db.GORM.Table("sys_user").Where("id=?", disabledID).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	login := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"solo-disabled","password":"admin123"}`, "")
	assertEnvelopeCode(t, login, http.StatusOK, http.StatusForbidden, "用户已禁用")
}
