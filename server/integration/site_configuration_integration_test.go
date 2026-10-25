//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresSiteConfiguration(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	repo := weighing.NewRepository(db.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	// ADR 0030 production admission is keyless: the Server resolves the local
	// site's sole scale and does not require the retired connection secret.
	hub.SetLocalResolver(func(ctx context.Context) (agentbridge.Config, error) {
		return repo.LocalAgentConfig(ctx, imageDir)
	})
	deps.Weighing = weighing.NewService(repo, hub)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, "LOCAL01", "本厂", "SCALE01", "一号地磅", "existing-agent")

	// The legacy export route no longer borrows a database address: callers must
	// supply the address the Agent host can reach.
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, "/api/weighing/site-config/1/connection", "", admin), http.StatusOK, http.StatusBadRequest, "请显式提供 Agent 主机可访问的业务服务地址")
	const exportPath = "/api/weighing/site-config/1/connection?serverUrl=http%3A%2F%2F127.0.0.1%3A18900"
	connection := weighingData[agentconfig.Connection](t, router, "GET", exportPath, "", admin)
	again := weighingData[agentconfig.Connection](t, router, "GET", exportPath, "", admin)
	if connection != again || connection.SiteID != 1 || connection.ScaleID != 1 || connection.AgentID != "agent-01" || connection.AgentKey != "" || connection.ServerURL != "http://127.0.0.1:18900" {
		t.Fatalf("local connection export was unstable or leaked a legacy key: first=%+v second=%+v", connection, again)
	}
	if err := connection.ValidateEndpoint(); err != nil {
		t.Fatal(err)
	}

	configs := weighingData[[]weighing.SiteConfiguration](t, router, http.MethodGet, "/api/weighing/site-config", "", admin)
	if len(configs) != 1 || configs[0].ID != 1 {
		t.Fatalf("single local configuration = %+v", configs)
	}
	input := weighing.SiteConfigurationInput{
		ServerURL: configs[0].ServerURL, Code: configs[0].Code, Name: configs[0].Name,
		ScaleCode: configs[0].ScaleCode, ScaleName: configs[0].ScaleName, AgentID: configs[0].AgentID,
		Enabled: configs[0].Enabled, Version: configs[0].Version,
	}
	encode := func(v any) string {
		t.Helper()
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	for field, change := range map[string]func(*weighing.SiteConfigurationInput){
		"factory code": func(v *weighing.SiteConfigurationInput) { v.Code = "FOREIGN" },
		"scale code":   func(v *weighing.SiteConfigurationInput) { v.ScaleCode = "FOREIGN-SCALE" },
		"agent ID":     func(v *weighing.SiteConfigurationInput) { v.AgentID = "foreign-agent" },
	} {
		candidate := input
		change(&candidate)
		response := serveJSON(router, http.MethodPut, "/api/weighing/site-config/1", encode(candidate), admin)
		if response.Code != http.StatusOK || envelopeCode(t, response) != http.StatusBadRequest {
			t.Errorf("changing fixed %s returned status=%d envelope=%d body=%s", field, response.Code, envelopeCode(t, response), response.Body.String())
		}
	}
	input.Name = "本厂改名"
	updated := weighingData[weighing.SiteConfiguration](t, router, http.MethodPut, "/api/weighing/site-config/1", encode(input), admin)
	if updated.Name != "本厂改名" {
		t.Fatalf("editable local name was not saved: %+v", updated)
	}

	// A local installation cannot add a second factory or write another
	// factory's configuration through legacy multi-site routes.
	additional := weighing.SiteConfigurationInput{Code: "OTHER", Name: "他厂", ScaleCode: "OTHER-SCALE", ScaleName: "他磅", AgentID: "other-agent", Enabled: 1}
	added := serveJSON(router, http.MethodPost, "/api/weighing/site-config", encode(additional), admin)
	if added.Code == http.StatusOK && envelopeCode(t, added) == http.StatusOK {
		t.Fatal("local installation created a second factory")
	}
	cross := serveJSON(router, http.MethodPut, "/api/weighing/site-config/2", encode(additional), admin)
	if cross.Code == http.StatusOK && envelopeCode(t, cross) == http.StatusOK {
		t.Fatal("local installation changed a foreign factory")
	}
	var sites, scales, bindings int64
	for table, count := range map[string]*int64{"wb_site": &sites, "wb_scale": &scales, "wb_agent_binding": &bindings} {
		if err := db.GORM.Table(table).Count(count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if sites != 1 || scales != 1 || bindings != 1 {
		t.Fatalf("legacy multi-site request changed local identity: sites=%d scales=%d bindings=%d", sites, scales, bindings)
	}

	// The existing Agent binding remains active without an API restart.
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	mockCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	mock := agentmock.New(agentmock.Config{Server: server.URL, ID: connection.AgentID, ScaleID: connection.ScaleID, Key: connection.AgentKey})
	control := httptest.NewServer(mock.Handler())
	t.Cleanup(control.Close)
	go func() { defer close(done); mock.Run(mockCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Error("mock Agent failed to stop")
		}
	})
	deadline := time.Now().Add(6 * time.Second)
	for !hub.Connected(1) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.Connected(1) {
		t.Fatal("local Agent binding did not connect")
	}
	input.Enabled = 0
	input.Version = updated.Version
	disabled := serveJSON(router, http.MethodPut, "/api/weighing/site-config/1", encode(input), admin)
	if disabled.Code != http.StatusOK || envelopeCode(t, disabled) != http.StatusBadRequest {
		t.Fatalf("online Agent should block disabling the local scale: %d %s", disabled.Code, disabled.Body.String())
	}
}
