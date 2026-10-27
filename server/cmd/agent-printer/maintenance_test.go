package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type remoteAuth struct{}

func (remoteAuth) Authenticate(context.Context, string) (auth.Principal, error) {
	return auth.Principal{UserID: 11, JTI: "maintenance-test"}, nil
}

type remotePermissions struct{}

func (remotePermissions) HasPermission(context.Context, int64, string) (bool, error) {
	return true, nil
}

func TestAgentOwnsRemoteConfigurationAndKeepsConnectionWhenDevicesStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := agentprinter.Defaults()
	cfg.AgentID = "remote-agent"
	cfg.AgentKey = "test-device-key"
	hub := agentbridge.New(agentbridge.Config{ID: cfg.AgentID, ScaleCode: cfg.ScaleCode, Key: cfg.AgentKey, ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	cfg.ServerURL = server.URL
	agentDir := t.TempDir()
	environment := filepath.Join(agentDir, "actual-device.env")
	stateDir := t.TempDir()
	if err := agentmaintenance.BuildAgentEnvironmentAt(environment, cfg); err != nil {
		t.Fatal(err)
	}
	runtime := &deviceRuntime{parent: ctx, cfg: cfg, dir: stateDir, environment: environment}
	local, err := agentmaintenance.OpenDevice(stateDir, environment, cfg, deviceRunner{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	runtime.maintenance = local
	local.SetCameraTester(runtime.testCameras)
	if err = runtime.start(cfg); err != nil {
		t.Fatal(err)
	}
	defer runtime.stop()
	_, scale, _ := runtime.snapshot()
	done := make(chan error, 1)
	go func() {
		done <- runManaged(ctx, cfg, scale, t.TempDir(), filepath.Join(stateDir, "maintenance-status.json"), filepath.Join(stateDir, "maintenance.pause"), runtime)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Agent did not stop")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = hub.MaintenanceState(cfg.ScaleCode); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Agent maintenance handshake missing")
		}
		time.Sleep(time.Millisecond)
	}
	connection := hub.State("WB01").ConnectionID
	initialRevision := hub.State("WB01").Revision
	var maintenanceActive atomic.Bool
	remote := agentmaintenance.NewRemote(hub, func(context.Context) (agentprinter.Config, error) {
		identity := cfg
		return identity, nil
	}, func(context.Context) (func(), error) {
		if !maintenanceActive.CompareAndSwap(false, true) {
			return nil, errors.New("maintenance already active")
		}
		return func() { maintenanceActive.Store(false) }, nil
	})
	api := gin.New()
	remote.RegisterBusinessRoutes(api.Group("/api/system"), remoteAuth{}, remotePermissions{}, nil)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		request := httptest.NewRequest(method, "/api/system/agent-maintenance"+path, bytes.NewReader(data))
		request.Header.Set("Authorization", "Bearer user-token")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response
	}
	response := call("GET", "/config", nil)
	var read struct {
		Config agentprinter.Config `json:"config"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read.Config.AgentKey != "" {
		t.Fatal("device key exposed")
	}
	changed := read.Config
	changed.CameraMode = "mediamtx"
	changed.CameraEnabled = true
	changed.CameraRTSPBase = "rtsp://192.168.1.10:8554"
	changed.CameraWHEPBase = "http://192.168.1.10:8889"
	changed.CameraFrontPath = "front"
	changed.CameraRearPath = "rear"
	if got := call("PUT", "/config", changed); got.Code != 200 {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	before, _ := agentprinter.ReadEnvironment(environment, agentprinter.Defaults())
	if before.CameraMode != "" {
		t.Fatal("draft changed live device config")
	}
	if got := call("POST", "/action", map[string]string{"action": "apply"}); got.Code != 200 {
		t.Fatalf("apply camera without API-host CUPS: %d %s", got.Code, got.Body.String())
	}
	if hub.State("WB01").Revision == initialRevision {
		t.Fatal("configuration activation kept old sample revision")
	}
	active, err := agentprinter.ReadEnvironment(environment, agentprinter.Defaults())
	if err != nil || active.CameraFrontURL != "rtsp://192.168.1.10:8554/front" {
		t.Fatal("Agent environment not applied", err)
	}
	if _, err = os.Stat(filepath.Join(stateDir, "agent.env")); !os.IsNotExist(err) {
		t.Fatal("a second environment file was created instead of using --env-file")
	}
	response = call("GET", "/jobs", nil)
	var jobs struct {
		Token   string `json:"previewToken"`
		Unknown bool   `json:"unknown"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &jobs)
	disposition := "keep"
	if jobs.Unknown {
		disposition = "unknown-confirmed"
	}
	if got := call("POST", "/action", map[string]string{"action": "stop-agent", "previewToken": jobs.Token, "disposition": disposition}); got.Code != 200 {
		t.Fatalf("stop devices: %d %s", got.Code, got.Body.String())
	}
	if _, _, running := runtime.snapshot(); running {
		t.Fatal("device workers not stopped")
	}
	if hub.State("WB01").ConnectionID != connection {
		t.Fatal("stopping devices dropped control connection")
	}
	if got := call("GET", "/config", nil); got.Code != 200 {
		t.Fatal("stopped devices lost maintenance access")
	}
	if got := call("POST", "/action", map[string]string{"action": "start-agent"}); got.Code != 200 {
		t.Fatalf("start devices: %d %s", got.Code, got.Body.String())
	}
	if _, _, running := runtime.snapshot(); !running {
		t.Fatal("workers did not resume")
	}
	if hub.State("WB01").ConnectionID != connection {
		t.Fatal("apply/restart replaced the Agent connection")
	}
	// Hold an in-flight device job while apply drains it. Navigating away from
	// the browser must not reopen weighing before the Agent finishes applying.
	job, _, _, finishJob, err := runtime.acquire(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if finishJob != nil {
			finishJob()
		}
	}()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request := httptest.NewRequest("POST", "/api/system/agent-maintenance/action", bytes.NewBufferString(`{"action":"apply"}`)).WithContext(requestCtx)
	request.Header.Set("Authorization", "Bearer user-token")
	applyFinished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		applyFinished <- response
	}()
	select {
	case <-job.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("apply did not begin draining device execution")
	}
	cancelRequest()
	select {
	case <-applyFinished:
		t.Fatal("browser cancellation released an unfinished Agent apply")
	case <-time.After(50 * time.Millisecond):
	}
	if !maintenanceActive.Load() {
		t.Fatal("weighing guard released before Agent apply completed")
	}
	finishJob()
	finishJob = nil
	select {
	case response := <-applyFinished:
		if response.Code != 200 || maintenanceActive.Load() {
			t.Fatalf("apply did not finish and release guard: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Agent apply did not complete after device job finished")
	}
	audit, err := os.ReadFile(filepath.Join(stateDir, "maintenance.jsonl"))
	if err != nil || !bytes.Contains(audit, []byte(`"actorId":11`)) {
		t.Fatal("device maintenance audit lost actor")
	}
	// A disconnected API must fail rather than save its own copy.
	server.CloseClientConnections()
	cancel()
	deadline = time.Now().Add(time.Second)
	for hub.Connected("WB01") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := call("GET", "/config", nil); got.Code != 503 {
		t.Fatal("offline config unexpectedly available")
	}
	if got := call("GET", "/status", nil); got.Code != 200 || !bytes.Contains(got.Body.Bytes(), []byte(`"agentProcessOnline":false`)) {
		t.Fatal("offline Agent was not reported")
	}

}

func TestDeviceReloadDoesNotInterruptActivePrint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &deviceRuntime{parent: ctx, dir: t.TempDir()}
	if err := runtime.start(agentprinter.Defaults()); err != nil {
		t.Fatal(err)
	}
	job, _, _, finish, err := runtime.acquire(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.stop(); err == nil {
		t.Fatal("stopped during a print execution")
	}
	if job.Err() != nil {
		t.Fatal("maintenance cancelled active printing")
	}
	finish()
	if err = runtime.stop(); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedPrintCannotAdoptReloadedConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &deviceRuntime{parent: ctx, dir: t.TempDir()}
	cfg := agentprinter.Defaults()
	if err := runtime.start(cfg); err != nil {
		t.Fatal(err)
	}
	generation := runtime.printGeneration()
	if err := runtime.stop(); err != nil {
		t.Fatal(err)
	}
	cfg.OffsetXMM++
	if err := runtime.start(cfg); err != nil {
		t.Fatal(err)
	}
	defer runtime.stop()
	if _, _, _, finish, err := runtime.acquireGeneration(ctx, true, generation); err == nil {
		finish()
		t.Fatal("queued print was admitted with a different device configuration")
	}
}
