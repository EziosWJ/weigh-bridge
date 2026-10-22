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
	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresSiteConfiguration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	checkSiteConfiguration(t, db, testDependencies(t, db, t.TempDir()))
}
func checkSiteConfiguration(t *testing.T, db *platformdatabase.Database, deps app.Dependencies) {
	ctx := context.Background()
	repo := weighing.NewRepository(db.GORM)
	images := t.TempDir()
	initial := []agentbridge.Config{{ID: "existing-agent", ScaleID: 1, Key: "existing-key", ImageDir: images}}
	if err := repo.ImportAgentBindings(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if err := repo.ImportAgentBindings(ctx, []agentbridge.Config{{ID: "must-not-replace", ScaleID: 1, Key: "different"}}); err != nil {
		t.Fatal(err)
	}
	bindings, err := repo.AgentConfigs(ctx, images)
	if err != nil {
		t.Fatal(err)
	}
	hub := agentbridge.NewMultiple(bindings)
	hub.SetResolver(func(ctx context.Context, id int64) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, id, images)
	})
	deps.Weighing = weighing.NewService(repo, hub)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	admin := loginAdmin(t, router)
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		s := ""
		if body != nil {
			s = encode(body)
		}
		return serveJSON(router, method, path, s, admin)
	}
	assertEnvelopeCode(t, call("PUT", "/api/weighing/agent-settings", map[string]string{"serverUrl": server.URL}), 200, 200, "success")
	old := weighingData[agentconfig.Connection](t, router, "GET", "/api/weighing/site-config/1/connection", "", admin)
	if old.AgentID != "existing-agent" || old.AgentKey != "existing-key" {
		t.Fatal("legacy identity was replaced")
	}
	input := weighing.SiteConfigurationInput{Code: "SITE03", Name: "三厂", ScaleCode: "SCALE03", ScaleName: "三厂地磅", AgentID: "site-agent-3", Enabled: 1}
	created := weighingData[weighing.SiteConfiguration](t, router, "POST", "/api/weighing/site-config", encode(input), admin)
	if created.ID <= 2 || created.ScaleID <= 2 {
		t.Fatalf("bad allocation %+v", created)
	}
	path := fmt.Sprintf("/api/weighing/site-config/%d", created.ID)
	connection := weighingData[agentconfig.Connection](t, router, "GET", path+"/connection", "", admin)
	again := weighingData[agentconfig.Connection](t, router, "GET", path+"/connection", "", admin)
	if connection != again || connection.SiteID != created.ID || connection.ScaleID != created.ScaleID || connection.AgentKey == "" {
		t.Fatal("configuration is not stable or is misbound")
	}
	if err := connection.Validate(); err != nil {
		t.Fatal(err)
	}
	input.Version = created.Version
	input.AgentID = "existing-agent"
	assertEnvelopeCode(t, call("PUT", path, input), 200, 400, "厂区、地磅及已有 Agent 编号不可修改，请保留原身份")
	input.AgentID = created.AgentID
	// Database-managed additions connect to the already-running Hub.
	mockCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	mock := agentmock.New(agentmock.Config{Server: server.URL, ID: connection.AgentID, ScaleID: connection.ScaleID, Key: connection.AgentKey})
	go func() { defer close(done); mock.Run(mockCtx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Error("mock did not exit")
		}
	}()
	deadline := time.Now().Add(6 * time.Second)
	for !hub.Connected(connection.ScaleID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.Connected(connection.ScaleID) {
		t.Fatal("newly configured Agent could not connect without restart")
	}
	input.Enabled = 0
	assertEnvelopeCode(t, call("PUT", path, input), 200, 400, "不能停用：Agent 尚未离线")
	// Ordinary downloads remain available while the Agent is connected.
	again = weighingData[agentconfig.Connection](t, router, "GET", path+"/connection", "", admin)
	if again != connection {
		t.Fatal("online export changed configuration")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatal("mock did not stop")
	}
	deadline = time.Now().Add(3 * time.Second)
	for hub.Connected(connection.ScaleID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	disabled := weighingData[weighing.SiteConfiguration](t, router, "PUT", path, encode(input), admin)
	request, _ := http.NewRequest("GET", server.URL+"/api/agent/ws", nil)
	request.Header.Set("Authorization", "Bearer "+connection.AgentKey)
	request.Header.Set("X-Agent-ID", connection.AgentID)
	request.Header.Set("X-Scale-ID", fmt.Sprint(connection.ScaleID))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("disabled site accepted Agent")
	}
	input.Enabled = 1
	input.Version = disabled.Version
	weighingData[weighing.SiteConfiguration](t, router, "PUT", path, encode(input), admin)
	if _, err := repo.AgentConfig(ctx, connection.ScaleID, images); err != nil {
		t.Fatal("reenabled mapping unavailable")
	}
	listing := call("GET", "/api/weighing/site-config", nil)
	if strings.Contains(listing.Body.String(), connection.AgentKey) {
		t.Fatal("ordinary list exposes connection key")
	}
}
