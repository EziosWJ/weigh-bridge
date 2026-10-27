package agentmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type businessAuthenticator struct{}

func (businessAuthenticator) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token != "Bearer valid" {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	return auth.Principal{UserID: 7, JTI: "session", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type businessPermissions map[string]bool

func (p businessPermissions) HasPermission(_ context.Context, userID int64, code string) (bool, error) {
	return userID == 7 && p[code], nil
}

type businessAudit struct{ events []audit.Event }

func (a *businessAudit) Record(_ context.Context, event audit.Event) error {
	a.events = append(a.events, event)
	return nil
}

type controllableAgentRunner struct{ active bool }

func (r *controllableAgentRunner) Run(args []string) Result {
	if len(args) == 4 && args[0] == "systemctl" && args[1] == "--user" {
		switch args[2] {
		case "start":
			r.active = true
			return Result{Output: "started"}
		case "stop":
			r.active = false
			return Result{Output: "stopped"}
		case "is-active":
			if r.active {
				return Result{Output: "active"}
			}
			return Result{Output: "inactive"}
		}
	}
	switch strings.Join(args, " ") {
	case "lpstat -r":
		return Result{Output: "scheduler is running"}
	case "lpstat -p":
		return Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
	case "lpstat -W not-completed -o":
		return Result{}
	}
	return Result{}
}

func TestBusinessRoutesUseJWTAndSeparateViewAndManagePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &fakeRunner{outputs: map[string]Result{
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "inactive"},
		"lpstat -r":                  {Output: "scheduler is running"},
		"lpstat -p":                  {Output: "printer Aisino-SK820-raw is idle. enabled"},
		"lpstat -W not-completed -o": {},
	}}
	service, err := Open(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	service.SetBusinessConfigValidator(func(context.Context, Config) error { return nil })
	permissions := businessPermissions{PermissionView: true}
	recorded := &businessAudit{}
	router := gin.New()
	service.RegisterBusinessRoutes(router.Group("/api/system"), businessAuthenticator{}, permissions, recorded)

	request := httptest.NewRequest(http.MethodGet, "/api/system/agent-maintenance/config", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("request without JWT status = %d, want 401", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/system/agent-maintenance/config", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "agentKey") && strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("JWT view failed or leaked config secret: %d %s", response.Code, response.Body)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/system/agent-maintenance/config", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer valid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("view-only caller can change config: %d %s", response.Code, response.Body)
	}
	if len(recorded.events) != 0 {
		t.Fatalf("denied request should not reach audit adapter: %+v", recorded.events)
	}

	permissions[PermissionManage] = true
	configBody, _ := json.Marshal(Defaults())
	request = httptest.NewRequest(http.MethodPut, "/api/system/agent-maintenance/config", strings.NewReader(string(configBody)))
	request.Header.Set("Authorization", "Bearer valid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("manage caller cannot save draft: %d %s", response.Code, response.Body)
	}
	if len(recorded.events) != 1 || recorded.events[0].Metadata.ActorID != 7 || recorded.events[0].Action != "maintenance.config.save" {
		t.Fatalf("business operation audit missing actor/action: %+v", recorded.events)
	}
}

func TestBusinessMaintenanceWorksWithoutOldMaintenanceProcessOrRunningAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &controllableAgentRunner{}
	service, err := Open(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	service.SetBusinessConfigValidator(func(context.Context, Config) error { return nil })
	router := gin.New()
	service.RegisterBusinessRoutes(router.Group("/api/system"), businessAuthenticator{}, businessPermissions{PermissionManage: true}, nil)
	callAPI := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer valid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	const configPath = "/api/system/agent-maintenance/config"
	response := callAPI(http.MethodGet, configPath, "")
	if response.Code != http.StatusOK {
		t.Fatalf("read config while Agent stopped: %d %s", response.Code, response.Body)
	}
	configBody, _ := json.Marshal(Defaults())
	response = callAPI(http.MethodPut, configPath, string(configBody))
	if response.Code != http.StatusOK {
		t.Fatalf("save config while Agent stopped: %d %s", response.Code, response.Body)
	}
	response = callAPI(http.MethodPost, "/api/system/agent-maintenance/action", `{"action":"start-agent"}`)
	if response.Code != http.StatusOK || !runner.active {
		t.Fatalf("start stopped Agent from business API: active=%t status=%d %s", runner.active, response.Code, response.Body)
	}
}

func TestCameraCredentialsStayHiddenAndBlankSavePreservesConfiguredURLs(t *testing.T) {
	dir := t.TempDir()
	service, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	handler := service.Handler()
	cookie := login(t, handler, dir)
	cfg := Defaults()
	cfg.AgentKey = "agent-test-key"
	cfg.CameraFrontURL = "rtsp://operator:camera-secret@10.0.0.10/live"
	cfg.CameraRearURL = "rtsp://operator:rear-secret@10.0.0.11/live"
	cfg.PlateRecognitionURL = "https://recognizer.local/api?token=recognition-secret"
	body, _ := json.Marshal(cfg)
	response, _ := call(t, handler, http.MethodPut, "/api/config", string(body), cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("save camera config: %d %s", response.Code, response.Body)
	}
	response, _ = call(t, handler, http.MethodGet, "/api/config", "", cookie)
	for _, secret := range []string{"camera-secret", "rear-secret", "recognition-secret", "rtsp://", "recognizer.local"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("camera endpoint leaked through config API (%s): %s", secret, response.Body)
		}
	}
	if !strings.Contains(response.Body.String(), `"hasCameraFrontUrl":true`) || !strings.Contains(response.Body.String(), `"hasPlateRecognitionUrl":true`) {
		t.Fatalf("camera presence metadata missing: %s", response.Body)
	}

	// The UI omits secret URLs on read and sends the visible configuration back.
	// Empty secret fields preserve the saved endpoints, matching AgentKey behavior.
	visible := Defaults()
	visible.CameraTimeoutMS = 2500
	body, _ = json.Marshal(visible)
	response, _ = call(t, handler, http.MethodPut, "/api/config", string(body), cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("save blank secret fields: %d %s", response.Code, response.Body)
	}
	service.mu.Lock()
	saved := service.draft
	service.mu.Unlock()
	if saved.CameraFrontURL != cfg.CameraFrontURL || saved.CameraRearURL != cfg.CameraRearURL || saved.PlateRecognitionURL != cfg.PlateRecognitionURL || saved.CameraTimeoutMS != 2500 {
		t.Fatalf("blank camera URLs did not preserve credentials/config: %+v", saved)
	}
	if err := BuildAgentEnvironment(dir, saved); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(dir, "agent.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `WEIGHBRIDGE_CAMERA_FRONT_URL="rtsp://operator:camera-secret@10.0.0.10/live"`) || !strings.Contains(string(env), "WEIGHBRIDGE_CAMERA_TIMEOUT_MS=2500") {
		t.Fatalf("camera values missing from agent environment: %s", env)
	}
	auditLog, err := os.ReadFile(filepath.Join(dir, "maintenance.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"camera-secret", "rear-secret", "recognition-secret"} {
		if strings.Contains(string(auditLog), secret) {
			t.Fatalf("camera secret leaked in maintenance audit: %s", auditLog)
		}
	}
}

func TestBusinessConfigMergeValidatesPreservedSecretsAndIdentity(t *testing.T) {
	dir := t.TempDir()
	service, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.AgentKey = "site-agent-key"
	cfg.CameraFrontURL = "rtsp://camera:secret@front.local/live"
	cfg.CameraRearURL = "rtsp://camera:rear-secret@rear.local/live"
	if err := validate(cfg); err != nil {
		t.Fatal(err)
	}
	service.draft = cfg
	service.active = cfg
	var sawMerged bool
	service.SetBusinessConfigValidator(func(_ context.Context, candidate Config) error {
		sawMerged = candidate.AgentKey == cfg.AgentKey && candidate.CameraRearURL == cfg.CameraRearURL
		if candidate.AgentID != cfg.AgentID {
			return errors.New("本站设备身份不可修改")
		}
		return nil
	})
	router := http.NewServeMux()
	router.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), businessRequestKey{}, true))
		service.configAPI(w, r)
	})
	changed := cfg
	changed.AgentID = "other-site-agent"
	changed.CameraFrontURL = "rtsp://camera:new-secret@new-front.local/live"
	changed.CameraRearURL = ""
	body, _ := json.Marshal(changed)
	request := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(string(body)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !sawMerged {
		t.Fatalf("identity validator did not see merged secrets/reject changed identity: status=%d body=%s sawMerged=%t", response.Code, response.Body, sawMerged)
	}
	service.mu.Lock()
	saved := service.draft
	service.mu.Unlock()
	if saved != cfg {
		t.Fatalf("rejected identity change mutated draft: %+v", saved)
	}
}

func TestBusinessImportRedactsAllSecretURLsAndRequiresIdentityValidator(t *testing.T) {
	service, err := Open(t.TempDir(), &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.AgentKey = "old-agent-key"
	cfg.CameraFrontURL = "rtsp://operator:front-secret@front.local/live"
	cfg.CameraRearURL = "rtsp://operator:rear-secret@rear.local/live"
	cfg.PlateRecognitionURL = "https://recognizer.local/api?token=recognizer-secret"
	service.draft, service.active = cfg, cfg
	connection := agentconfig.Connection{ServerURL: "http://127.0.0.1:18900", SiteCode: "SITE", SiteName: "Site", ScaleCode: "SCALE", ScaleName: "Scale", AgentID: cfg.AgentID, AgentKey: "new-agent-key"}
	body, _ := json.Marshal(map[string]any{"connection": connection})
	request := httptest.NewRequest(http.MethodPost, "/config/import", strings.NewReader(string(body)))
	request = request.WithContext(context.WithValue(request.Context(), businessRequestKey{}, true))
	response := httptest.NewRecorder()
	service.importConnection(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "身份校验未配置") {
		t.Fatalf("business import without validator must fail closed: %d %s", response.Code, response.Body)
	}
	service.SetBusinessConfigValidator(func(context.Context, Config) error { return nil })
	request = httptest.NewRequest(http.MethodPost, "/config/import", strings.NewReader(string(body)))
	request = request.WithContext(context.WithValue(request.Context(), businessRequestKey{}, true))
	response = httptest.NewRecorder()
	service.importConnection(response, request)
	for _, secret := range []string{"front-secret", "rear-secret", "recognizer-secret", "new-agent-key", "old-agent-key", "rtsp://", "recognizer.local"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("business import response leaked secret %q: %s", secret, response.Body)
		}
	}
	for _, flag := range []string{`"hasAgentKey":true`, `"hasCameraFrontUrl":true`, `"hasCameraRearUrl":true`, `"hasPlateRecognitionUrl":true`} {
		if !strings.Contains(response.Body.String(), flag) {
			t.Fatalf("business import response missing redaction flag %s: %s", flag, response.Body)
		}
	}
}

func TestBusinessApplyRestoreAndStartRejectIdentityMismatchBeforeRunnerCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &controllableAgentRunner{}
	service, err := Open(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	service.previous = Defaults()
	validatorCalls := 0
	service.SetBusinessConfigValidator(func(context.Context, Config) error {
		validatorCalls++
		return errors.New("本机身份不匹配")
	})
	router := gin.New()
	service.RegisterBusinessRoutes(router.Group("/api/system"), businessAuthenticator{}, businessPermissions{PermissionManage: true}, nil)
	for _, action := range []string{"apply", "restore", "start-agent"} {
		request := httptest.NewRequest(http.MethodPost, "/api/system/agent-maintenance/action", strings.NewReader(`{"action":"`+action+`"}`))
		request.Header.Set("Authorization", "Bearer valid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "身份不匹配") {
			t.Fatalf("%s must reject a mismatched local identity: %d %s", action, response.Code, response.Body)
		}
	}
	if validatorCalls != 3 || runner.active {
		t.Fatalf("identity mismatch reached operation runner: validatorCalls=%d active=%t", validatorCalls, runner.active)
	}
}

func TestStopAgentRequiresQueuePreviewAndLeavesAgentStopped(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{outputs: map[string]Result{
		"lpstat -W not-completed -o": {},
		"lpstat -p":                  {Output: "printer Aisino-SK820-raw is idle. enabled"},
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "inactive"},
		"systemctl --user stop weighbridge-agent-printer.service":      {},
	}}
	service, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	handler := service.Handler()
	cookie := login(t, handler, dir)
	response, _ := call(t, handler, http.MethodPost, "/api/action", `{"action":"stop-agent"}`, cookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("stop without queue preview status = %d, want rejection: %s", response.Code, response.Body)
	}
	response, _ = call(t, handler, http.MethodGet, "/api/jobs", "", cookie)
	var preview struct {
		Token string `json:"previewToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil || preview.Token == "" {
		t.Fatalf("job preview token missing: %s (%v)", response.Body, err)
	}
	body := `{"action":"stop-agent","previewToken":"` + preview.Token + `","disposition":"keep"}`
	response, _ = call(t, handler, http.MethodPost, "/api/action", body, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("stop agent: %d %s", response.Code, response.Body)
	}
	for _, call := range runner.calls {
		if strings.Join(call, " ") == "systemctl --user start weighbridge-agent-printer.service" {
			t.Fatal("stop action unexpectedly restarted the Agent")
		}
	}
}
