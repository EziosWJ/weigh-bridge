//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Exercise cmd/api as well as app.Build so a missing runtime maintenance
// constructor or validator cannot be masked by fixture dependency injection.
func TestPostgresDevelopmentMaintenanceProcessStartup(t *testing.T) {
	temporary := startBusinessPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "api")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/api")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build API: %v %s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0700); err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join(projectRoot(t), "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "config.yaml"), base, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	database := databaseConfigFromDSN(t, temporary.dsn)
	settings := map[string]any{
		"database":    map[string]any{"driver": "postgres", "url": database.URL, "username": database.Username, "password": database.Password},
		"jwt":         map[string]any{"secret": "isolated-runtime-maintenance-jwt"},
		"http":        map[string]any{"address": address},
		"file":        map[string]any{"storage_root": filepath.Join(dir, "uploads")},
		"maintenance": map[string]any{"data_dir": filepath.Join(dir, "agent")},
		"weighing":    map[string]any{"agent_id": "runtime-dev-agent", "scale_id": 1, "agent_key": "runtime-dev-key", "image_dir": filepath.Join(dir, "images")},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(dir, "configs", "config.dev.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "APP_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "APP_ENV=dev", "GIN_MODE=release")
	logPath := filepath.Join(dir, "runtime.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = logFile.Close()
	})
	client := &http.Client{Timeout: time.Second}
	baseURL := "http://" + address
	ready := false
	for range 60 {
		response, err := client.Get(baseURL + "/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		output, _ := os.ReadFile(logPath)
		t.Fatalf("development API did not start: %s", output)
	}
	response, err := client.Post(baseURL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	if err != nil {
		t.Fatal(err)
	}
	var login struct {
		Data struct {
			Token string `json:"tokenValue"`
		} `json:"data"`
	}
	err = json.NewDecoder(response.Body).Decode(&login)
	_ = response.Body.Close()
	if err != nil || login.Data.Token == "" {
		t.Fatal("runtime login failed")
	}
	get := func(path string) []byte {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		req.Header.Set("Authorization", login.Data.Token)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("runtime %s status=%d", path, response.StatusCode)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	var caps struct {
		Data struct {
			Enabled bool   `json:"maintenanceEnabled"`
			Role    string `json:"nodeRole"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get("/api/weighing/capabilities"), &caps); err != nil {
		t.Fatal(err)
	}
	if !caps.Data.Enabled || caps.Data.Role != "" {
		t.Fatal("actual default-development API did not expose maintenance")
	}
	var state struct {
		Config           agentmaintenance.Config `json:"config"`
		EnvironmentKnown bool                    `json:"environmentKnown"`
	}
	if err := json.Unmarshal(get("/api/system/agent-maintenance/config"), &state); err != nil {
		t.Fatal(err)
	}
	if state.Config.AgentID != "runtime-dev-agent" || state.Config.ServerURL != baseURL || state.EnvironmentKnown {
		t.Fatal("actual runtime did not prepare the current local identity as an unapplied draft")
	}
	state.Config.OffsetXMM = 2
	body, _ := json.Marshal(state.Config)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/api/system/agent-maintenance/config", strings.NewReader(string(body)))
	req.Header.Set("Authorization", login.Data.Token)
	req.Header.Set("Content-Type", "application/json")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("actual development validator did not permit saving local draft: %d", response.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent", "agent.env")); !os.IsNotExist(err) {
		t.Fatal("runtime must not apply device environment automatically")
	}
}

func TestPostgresDevelopmentMaintenanceUsesExistingBindingWithoutChangingNodeIdentity(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	ctx := context.Background()
	repository := weighing.NewRepository(db.GORM)
	if err := repository.ImportAgentBindings(ctx, []agentbridge.Config{{ID: "existing-dev-agent", ScaleID: 1, Key: "existing-dev-agent-key"}}); err != nil {
		t.Fatal(err)
	}
	initial, err := repository.DevelopmentMaintenanceConfig(ctx, 1, "http://127.0.0.1:8900")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	maintenance, err := agentmaintenance.OpenWithDraft(dir, &maintenanceIntegrationRunner{}, initial)
	if err != nil {
		t.Fatal(err)
	}
	deps := testDependencies(t, db, t.TempDir())
	deps.Maintenance = maintenance
	deps.MaintenanceBusinessValidator = func(ctx context.Context, candidate agentmaintenance.Config) error {
		return repository.ValidateDevelopmentMaintenanceConfig(ctx, candidate, 1, "http://127.0.0.1:8900")
	}
	deps.Weighing = weighing.NewService(repository, nil)
	cfg := testAPIConfig()
	cfg.Environment = config.EnvironmentDev
	cfg.Maintenance.DataDir = dir
	router, err := app.Build(cfg, db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	capabilities := serveJSON(router, http.MethodGet, "/api/weighing/capabilities", "", token)
	var caps struct {
		Data struct {
			MaintenanceEnabled bool   `json:"maintenanceEnabled"`
			NodeRole           string `json:"nodeRole"`
		} `json:"data"`
	}
	if err := json.Unmarshal(capabilities.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if !caps.Data.MaintenanceEnabled || caps.Data.NodeRole != "" {
		t.Fatal("development API must advertise actual maintenance availability without changing node role")
	}
	const endpoint = "/api/system/agent-maintenance/config"
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, endpoint, "", ""))
	response := serveJSON(router, http.MethodGet, endpoint, "", token)
	var state struct {
		Config           agentmaintenance.Config `json:"config"`
		EnvironmentKnown bool                    `json:"environmentKnown"`
		HasKey           bool                    `json:"hasAgentKey"`
	}
	if response.Code != 200 {
		t.Fatalf("read maintenance status=%d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.EnvironmentKnown || !state.HasKey || state.Config.AgentID != "existing-dev-agent" || state.Config.AgentKey != "" || state.Config.SiteID != 1 {
		t.Fatal("maintenance must expose the existing identity as a private unapplied draft")
	}
	state.Config.OffsetXMM = 3
	body, _ := json.Marshal(state.Config)
	if response := serveJSON(router, http.MethodPut, endpoint, string(body), token); response.Code != 200 {
		t.Fatalf("save development draft: %d %s", response.Code, response.Body.String())
	}
	state.Config.ScaleID = 2
	body, _ = json.Marshal(state.Config)
	if response := serveJSON(router, http.MethodPut, endpoint, string(body), token); response.Code != 400 {
		t.Fatalf("cross-scale rebind must be rejected: %d", response.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.env")); !os.IsNotExist(err) {
		t.Fatal("saving draft must not create effective device configuration")
	}
	var identities int64
	if err := db.GORM.Table("wb_node_identity").Count(&identities).Error; err != nil || identities != 0 {
		t.Fatal("development maintenance must not create or initialize a node identity")
	}
	agent, err := repository.AgentConfig(ctx, 1, "")
	if err != nil || agent.Key != "existing-dev-agent-key" {
		t.Fatal("development maintenance changed the database binding/key")
	}
	var siteURL string
	if err := db.GORM.Table("wb_site").Select("server_url").Where("id=1").Scan(&siteURL).Error; err != nil || siteURL != "" {
		t.Fatal("development API address must not rewrite the saved site address")
	}

	deps.Maintenance = nil
	deps.MaintenanceBusinessValidator = nil
	disabled, err := app.Build(cfg, db, deps)
	if err != nil {
		t.Fatal(err)
	}
	capabilities = serveJSON(disabled, http.MethodGet, "/api/weighing/capabilities", "", token)
	if err := json.Unmarshal(capabilities.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if caps.Data.MaintenanceEnabled {
		t.Fatal("configuration alone must not advertise an unmounted service")
	}
	if response := serveJSON(disabled, http.MethodGet, endpoint, "", token); response.Code != 404 {
		t.Fatal("disabled maintenance API should not be mounted")
	}
}
