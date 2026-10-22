//go:build integration

package integration

import (
	"bytes"
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
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresSiteIsolation(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	checkSiteIsolation(t, db, testDependencies(t, db, t.TempDir()))
}
func checkSiteIsolation(t *testing.T, db *platformdatabase.Database, deps app.Dependencies) {
	initial := []agentbridge.Config{{ID: "site-agent-1", ScaleID: 1, Key: "site-key-1", ImageDir: t.TempDir()}, {ID: "site-agent-2", ScaleID: 2, Key: "site-key-2", ImageDir: t.TempDir()}}
	repo := weighing.NewRepository(db.GORM)
	if err := repo.ImportAgentBindings(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	hub := agentbridge.NewMultiple(initial)
	deps.Weighing = weighing.NewService(weighing.NewRepository(db.GORM), hub)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	encode := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	controls := map[int64]*httptest.Server{}
	stopped := map[int64]chan struct{}{}
	for _, id := range []int64{1, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		mock := agentmock.New(agentmock.Config{Server: server.URL, ID: fmt.Sprintf("site-agent-%d", id), ScaleID: id, Key: fmt.Sprintf("site-key-%d", id), OnKick: cancel})
		control := httptest.NewServer(mock.Handler())
		controls[id] = control
		t.Cleanup(control.Close)
		done := make(chan struct{})
		stopped[id] = done
		go func() { defer close(done); mock.Run(ctx) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(12 * time.Second):
				t.Error("mock did not exit")
			}
		})
	}
	weight := func(id int64, v string) {
		for _, body := range []string{fmt.Sprintf(`{"action":"up","weight":%q}`, v), `{"action":"stable"}`} {
			resp, e := http.Post(controls[id].URL+"/api/control", "application/json", bytes.NewBufferString(body))
			if e != nil {
				t.Fatal(e)
			}
			resp.Body.Close()
			if resp.StatusCode != 204 {
				t.Fatal(resp.Status)
			}
		}
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			s := hub.State(id)
			if s.Online && s.Stable && s.Weight.String() == v && len(s.Images) == 2 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("device %d not ready: %+v", id, hub.State(id))
	}
	weight(1, "30")
	weight(2, "31")
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"共用高粱"}`, admin)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"共用供方","nature":"enterprise","roles":["supplier"]}`, admin)
	business := map[string]any{"plate": "川A23456", "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
	sessions := map[int64]weighing.SessionResult{}
	tasks := map[int64]weighing.CommandResult{}
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	for _, id := range []int64{1, 2} {
		session := weighingData[weighing.SessionResult](t, router, "POST", fmt.Sprintf("/api/weighing/scales/%d/session/acquire", id), fmt.Sprintf(`{"clientId":"browser-%d"}`, id), admin)
		sessions[id] = session
		input := map[string]any{"scaleId": id, "clientId": fmt.Sprintf("browser-%d", id), "sessionToken": session.SessionToken, "business": business}
		sample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(input), admin)
		input["sampleId"] = sample.ID
		input["checks"] = checks
		input["requestId"] = fmt.Sprintf("site-%d-first-request", id)
		tasks[id] = weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), admin)
		row := weighingData[weighing.TaskDetail](t, router, "GET", fmt.Sprintf("/api/weighing/tasks/%d", tasks[id].TaskID), "", admin)
		if row.SiteID != id {
			t.Fatalf("wrong task ownership: %+v", row)
		}
	}
	for _, id := range []int64{1, 2} {
		row := weighingData[weighing.TaskDetail](t, router, "GET", fmt.Sprintf("/api/weighing/tasks/active?plate=川A23456&scaleId=%d", id), "", admin)
		if row.ID != tasks[id].TaskID {
			t.Fatal("active plate crossed sites")
		}
	}
	// Even a user authorized at both sites cannot combine their weighing bases.
	cross := serveJSON(router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 2, "taskId": tasks[1].TaskID, "clientId": "browser-2", "sessionToken": sessions[2].SessionToken}), admin)
	assertEnvelopeCode(t, cross, 403, 403, weighing.ErrForbidden.Error())
	// Complete the first site, retaining the same plate's independent pending task at site 2.
	weight(1, "10")
	sample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 1, "taskId": tasks[1].TaskID, "clientId": "browser-1", "sessionToken": sessions[1].SessionToken}), admin)
	result := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", tasks[1].TaskID), encode(map[string]any{"scaleId": 1, "sampleId": sample.ID, "clientId": "browser-1", "sessionToken": sessions[1].SessionToken, "version": 1, "checks": checks, "requestId": "site-1-second-request"}), admin)
	ticket := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", admin)
	if ticket.SiteID != 1 || ticket.SiteName != "原厂区" || !strings.Contains(ticket.PrintSheet.Title, "原厂区") {
		t.Fatalf("missing ticket site snapshot: %+v", ticket)
	}
	// Legacy tickets gain a display label without rewriting the stored snapshot.
	if err := db.GORM.Model(&weighing.Ticket{}).Where("id=?", result.TicketID).Update("site_name", "").Error; err != nil {
		t.Fatal(err)
	}
	legacy := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", admin)
	if legacy.SiteName != "原厂区" || !strings.Contains(legacy.PrintSheet.Title, "原厂区") {
		t.Fatal("legacy ticket lost its site label")
	}
	var storedName string
	if err := db.GORM.Table("wb_ticket").Select("site_name").Where("id=?", result.TicketID).Scan(&storedName).Error; err != nil || storedName != "" {
		t.Fatal("legacy snapshot was rewritten")
	}
	// A new user has no site access until explicitly granted; role actions are separate.
	weighingData[map[string]any](t, router, "POST", "/api/system/user", `{"username":"site-operator","nickname":"厂区操作员","status":1}`, admin)
	var user int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username='site-operator'").Scan(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", user).Error; err != nil {
		t.Fatal(err)
	}
	login := weighingData[struct {
		TokenValue string `json:"tokenValue"`
	}](t, router, "POST", "/api/auth/login", `{"username":"site-operator","password":"admin123"}`, "")
	operator := login.TokenValue
	if sites := weighingData[[]weighing.Site](t, router, "GET", "/api/weighing/sites", "", operator); len(sites) != 0 {
		t.Fatal("new user received implicit grants")
	}
	grantPath := fmt.Sprintf("/api/weighing/users/%d/sites", user)
	assertEnvelopeCode(t, serveJSON(router, "PUT", grantPath, `{"grants":[{"siteId":2,"canOperate":1}]}`, operator), 403, 403, weighing.ErrForbidden.Error())
	assertEnvelopeCode(t, serveJSON(router, "PUT", grantPath, `{"grants":[{"siteId":1,"canOperate":1}]}`, admin), 200, 200, "success")
	caps := weighingData[struct {
		Scales []weighing.Scale `json:"scales"`
		Sites  []weighing.Site  `json:"sites"`
	}](t, router, "GET", "/api/weighing/capabilities", "", operator)
	if len(caps.Sites) != 1 || len(caps.Scales) != 1 || caps.Scales[0].SiteID != 1 || !caps.Scales[0].CanOperate {
		t.Fatalf("panel scope: %+v", caps)
	}
	for _, path := range []string{fmt.Sprintf("/api/weighing/tasks/%d", tasks[2].TaskID), fmt.Sprintf("/api/weighing/tasks/%d/events", tasks[2].TaskID), "/api/weighing/scales/2/device", "/api/weighing/scales/2/session", "/api/weighing/tasks?siteId=2", "/api/weighing/tickets?taskId=" + itoa(tasks[2].TaskID), hub.State(2).Images[0].URL} {
		assertEnvelopeCode(t, serveJSON(router, "GET", path, "", operator), 403, 403, weighing.ErrForbidden.Error())
	}
	// Mixed-case JSON must be checked exactly as the handler's JSON decoder sees it.
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/samples", `{"ScaleID":2,"clientId":"attack"}`, operator), 403, 403, weighing.ErrForbidden.Error())
	rows := weighingData[weighing.Page[weighing.Task]](t, router, "GET", "/api/weighing/tasks", "", operator)
	if rows.Total != 1 || rows.Records[0].SiteID != 1 {
		t.Fatalf("list leaked task: %+v", rows)
	}
	exported := serveJSON(router, "GET", "/api/weighing/tasks/export", "", operator)
	if exported.Code != 200 || strings.Contains(exported.Body.String(), "site-2") {
		t.Fatal("export failed or leaked")
	}
	// Keep action permissions, grant only read access at site 2.
	assertEnvelopeCode(t, serveJSON(router, "PUT", grantPath, `{"grants":[{"siteId":1,"canOperate":1},{"siteId":2,"canOperate":0}]}`, admin), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "GET", "/api/weighing/scales/2/device", "", operator), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/scales/2/session/acquire", `{"clientId":"read-only"}`, operator), 403, 403, weighing.ErrForbidden.Error())
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/scales/1/agent/kick", encode(map[string]any{"connectionId": hub.State(1).ConnectionID}), operator), 403, 403, weighing.ErrForbidden.Error())
	// Preserve an unconfirmed sample and a print with no final receipt across kick.
	unconfirmed := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 1, "clientId": "browser-1", "sessionToken": sessions[1].SessionToken, "business": business}), admin)
	weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", result.TicketID), `{"requestId":"site-print-before-kick","intent":"initial"}`, admin)
	// Kick one site's mock; it must actually stop, while the other site's device remains live.
	other := hub.State(2)
	kicked := weighingData[agentbridge.KickResult](t, router, "POST", "/api/weighing/scales/1/agent/kick", encode(map[string]any{"connectionId": hub.State(1).ConnectionID}), admin)
	if !kicked.Disconnected || !kicked.ExitConfirmed {
		t.Fatalf("kick not confirmed: %+v", kicked)
	}
	select {
	case <-stopped[1]:
	case <-time.After(2 * time.Second):
		t.Fatal("mock remained running after kick")
	}
	if hub.State(1).Online || !hub.State(2).Online || hub.State(2).Revision != other.Revision {
		t.Fatal("kick crossed sites or failed to disconnect")
	}
	stale := serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", encode(map[string]any{"scaleId": 1, "sampleId": unconfirmed.ID, "clientId": "browser-1", "sessionToken": sessions[1].SessionToken, "business": business, "checks": checks, "requestId": "site-stale-after-kick"}), admin)
	assertEnvelopeCode(t, stale, 200, 400, weighing.ErrConditions.Error())
	afterKick := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", admin)
	attempts := weighingData[[]weighing.PrintAttempt](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", result.TicketID), "", admin)
	if afterKick.PrintStatus != "unknown" || len(attempts) != 1 {
		t.Fatal("kick changed or replayed the unresolved print")
	}
	var auditCount int64
	if err := db.GORM.Table("sys_oper_log").Where("operation_type LIKE 'agent.kick%'").Count(&auditCount).Error; err != nil || auditCount == 0 {
		t.Fatalf("missing kick audit: %v", err)
	}
	// Disabling a completed site must preserve historical reads and block writes.
	configs := weighingData[[]weighing.SiteConfiguration](t, router, "GET", "/api/weighing/site-config", "", admin)
	var firstConfig weighing.SiteConfiguration
	for _, v := range configs {
		if v.ID == 1 {
			firstConfig = v
		}
	}
	configInput := weighing.SiteConfigurationInput{Code: firstConfig.Code, Name: firstConfig.Name, ScaleCode: firstConfig.ScaleCode, ScaleName: firstConfig.ScaleName, AgentID: "site-agent-1", Enabled: 0, Version: firstConfig.Version}
	configPath := "/api/weighing/site-config/1"
	assertEnvelopeCode(t, serveJSON(router, "PUT", configPath, encode(configInput), admin), 200, 400, "不能停用：1 笔未决打印")
	weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/print-attempts/%d/result", attempts[0].ID), `{"requestId":"site-print-resolve-for-disable","status":"printed","reason":"现场已核实"}`, admin)
	weighingData[weighing.SiteConfiguration](t, router, "PUT", configPath, encode(configInput), admin)
	historical := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", operator)
	if historical.SiteID != 1 {
		t.Fatal("disabled site history missing")
	}
	historicalList := weighingData[weighing.Page[weighing.Task]](t, router, "GET", "/api/weighing/tasks?siteId=1", "", operator)
	if historicalList.Total != 1 {
		t.Fatal("disabled site task list missing")
	}
	exported = serveJSON(router, "GET", "/api/weighing/tasks/export?siteId=1", "", operator)
	if exported.Code != 200 || !strings.Contains(exported.Body.String(), "川A23456") {
		t.Fatal("disabled history export missing")
	}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/scales/1/session/acquire", `{"clientId":"disabled-site"}`, admin), 403, 403, weighing.ErrForbidden.Error())
	configInput.Enabled = 1
	configInput.Version++
	weighingData[weighing.SiteConfiguration](t, router, "PUT", configPath, encode(configInput), admin)
	assertEnvelopeCode(t, serveJSON(router, "GET", "/api/weighing/site-config", "", operator), 403, 403, weighing.ErrForbidden.Error())
	assertEnvelopeCode(t, serveJSON(router, "GET", "/api/weighing/site-config/1/connection", "", operator), 403, 403, weighing.ErrForbidden.Error())

}
