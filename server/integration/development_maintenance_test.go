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
		"weighing":    map[string]any{"image_dir": filepath.Join(dir, "images")},
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
	// ADR 0033: identity is installed explicitly with the head-office codes.
	initializeRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/weighing/site-config/initialize", strings.NewReader(`{"siteCode":"DEV01","scaleCode":"DEV-SCALE","name":"开发测试厂","scaleName":"开发测试磅"}`))
	initializeRequest.Header.Set("Authorization", login.Data.Token)
	initializeRequest.Header.Set("Content-Type", "application/json")
	initializeResponse, err := client.Do(initializeRequest)
	if err != nil {
		t.Fatal(err)
	}
	initialized, _ := io.ReadAll(initializeResponse.Body)
	_ = initializeResponse.Body.Close()
	if initializeResponse.StatusCode != 200 || !strings.Contains(string(initialized), `"DEV01"`) {
		t.Fatalf("initialize local identity: %d %s", initializeResponse.StatusCode, initialized)
	}
	statusRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/weighing/local-status", nil)
	statusRequest.Header.Set("Authorization", login.Data.Token)
	statusResponse, err := client.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	statusPayload, _ := io.ReadAll(statusResponse.Body)
	_ = statusResponse.Body.Close()
	if statusResponse.StatusCode != 200 || !strings.Contains(string(statusPayload), `"configured":true`) {
		t.Fatalf("API did not report the installed local identity: %d %s", statusResponse.StatusCode, statusPayload)
	}
	deviceDB := openTemporaryDatabase(t, temporary.dsn)
	defer deviceDB.Close()
	deviceRepo := weighing.NewRepository(deviceDB.GORM)
	deviceConfig, err := deviceRepo.DevelopmentMaintenanceConfig(ctx, "DEV-SCALE", "http://"+address)
	if err != nil {
		t.Fatalf("read identity generated by running API: %v", err)
	}
	runner := &maintenanceIntegrationRunner{active: true}
	device, err := agentmaintenance.OpenDevice(t.TempDir(), "", deviceConfig, runner)
	if err != nil {
		t.Fatalf("prepare external Agent maintenance service: %v", err)
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
			Enabled       bool   `json:"maintenanceEnabled"`
			LocalSiteCode string `json:"localSiteCode"`
			Configured    bool   `json:"configured"`
			Initialized   bool   `json:"initialized"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get("/api/weighing/capabilities"), &caps); err != nil {
		t.Fatal(err)
	}
	if !caps.Data.Enabled || caps.Data.LocalSiteCode != "DEV01" || !caps.Data.Configured || !caps.Data.Initialized {
		t.Fatalf("initialized default-development API capabilities = %+v", caps.Data)
	}
	var status struct {
		Online bool `json:"agentProcessOnline"`
		Remote bool `json:"remote"`
	}
	if err := json.Unmarshal(get("/api/system/agent-maintenance/status"), &status); err != nil || status.Online || !status.Remote {
		t.Fatal("offline Agent status missing", err)
	}
	offlineRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/system/agent-maintenance/config", nil)
	offlineRequest.Header.Set("Authorization", login.Data.Token)
	offlineResponse, err := client.Do(offlineRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = offlineResponse.Body.Close()
	if offlineResponse.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("offline API must not synthesize a remote Agent config: %d", offlineResponse.StatusCode)
	}
	if err := runRemoteMaintenanceAgent(t, baseURL, nil, deviceConfig, device, runner); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(get("/api/system/agent-maintenance/status"), &status); err != nil || !status.Online || !status.Remote {
		t.Fatal("first-time DB identity was not available to the running API without restart", err)
	}
	maintenanceConfig := get("/api/system/agent-maintenance/config")
	if strings.Contains(string(maintenanceConfig), deviceConfig.AgentKey) || !strings.Contains(string(maintenanceConfig), `"hasAgentKey":true`) {
		t.Fatal("remote maintenance must redact the DB-generated Agent key")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent", "maintenance.json")); !os.IsNotExist(err) {
		t.Fatal("API must not create Agent-owned drafts")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent", "agent.env")); !os.IsNotExist(err) {
		t.Fatal("API must not create device environment files")
	}
}

func TestPostgresDevelopmentMaintenanceUsesExistingBindingWithoutChangingLocalIdentity(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	ctx := context.Background()
	repository := weighing.NewRepository(db.GORM)
	deps := testDependencies(t, db, t.TempDir())
	deps.Weighing = weighing.NewService(repository, nil)
	cfg := testAPIConfig()
	cfg.Environment = config.EnvironmentDev
	router, err := app.Build(cfg, db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	ensureLocalSite(t, router, token, 1, "DEV01", "开发测试厂", "DEV-SCALE", "开发测试磅", "existing-dev-agent")
	binding, err := repository.AgentConfig(ctx, "DEV-SCALE", "")
	if err != nil {
		t.Fatal(err)
	}
	capabilities := serveJSON(router, http.MethodGet, "/api/weighing/capabilities", "", token)
	var caps struct {
		Data struct {
			MaintenanceEnabled bool   `json:"maintenanceEnabled"`
			LocalSiteCode      string `json:"localSiteCode"`
			Configured         bool   `json:"configured"`
			Initialized        bool   `json:"initialized"`
		} `json:"data"`
	}
	if err := json.Unmarshal(capabilities.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if caps.Data.MaintenanceEnabled || caps.Data.LocalSiteCode != "DEV01" || !caps.Data.Configured || !caps.Data.Initialized {
		t.Fatalf("configured development API capabilities = %+v", caps.Data)
	}
	const endpoint = "/api/system/agent-maintenance/config"
	if response := serveJSON(router, http.MethodGet, endpoint, "", token); response.Code != 404 {
		t.Fatalf("unwired maintenance endpoint should not be mounted: %d", response.Code)
	}
	var identityCount int64
	if err := db.GORM.Table("wb_local_identity").Where("site_code = ? AND initialized = 0", "DEV01").Count(&identityCount).Error; err != nil || identityCount != 1 {
		t.Fatal("development maintenance must not change the local identity or write the legacy initialization flag")
	}
	agent, err := repository.AgentConfig(ctx, "DEV-SCALE", "")
	if err != nil || agent.Key != binding.Key {
		t.Fatal("development maintenance changed the database binding/key")
	}
	var siteURL string
	if err := db.GORM.Table("wb_site").Select("server_url").Where("site_code=?", "DEV01").Scan(&siteURL).Error; err != nil || siteURL != "" {
		t.Fatal("development API address must not be written into the legacy site address field")
	}
}
