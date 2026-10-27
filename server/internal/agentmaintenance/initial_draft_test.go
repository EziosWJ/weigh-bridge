package agentmaintenance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInitialDraftDoesNotApplyOrOverwriteDeviceConfiguration(t *testing.T) {
	dir := t.TempDir()
	initial := Defaults()
	initial.SiteCode, initial.AgentID, initial.AgentKey = "F002", "development-agent", "local-key"
	initial.ServerURL = "http://127.0.0.1:8900"
	runner := &controllableAgentRunner{}
	service, err := OpenWithDraft(dir, runner, initial)
	if err != nil {
		t.Fatal(err)
	}
	read := func(s *Service) Config {
		router := gin.New()
		s.RegisterBusinessRoutes(router.Group("/api/system"), businessAuthenticator{}, businessPermissions{PermissionView: true}, nil)
		r := httptest.NewRequest(http.MethodGet, "/api/system/agent-maintenance/config", nil)
		r.Header.Set("Authorization", "Bearer valid")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("read initial draft status=%d", w.Code)
		}
		var v struct {
			Config           Config `json:"config"`
			EnvironmentKnown bool   `json:"environmentKnown"`
			Saved            bool   `json:"saved"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if v.EnvironmentKnown || v.Saved {
			t.Fatal("prepared draft was presented as applied")
		}
		return v.Config
	}
	if read(service).AgentID != initial.AgentID {
		t.Fatal("database identity did not populate initial draft")
	}
	if runner.active {
		t.Fatal("preparing maintenance unexpectedly started Agent")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.env")); !os.IsNotExist(err) {
		t.Fatal("preparing draft must not write effective device environment")
	}
	other := initial
	other.AgentID = "replacement-agent"
	reopened, err := OpenWithDraft(dir, runner, other)
	if err != nil {
		t.Fatal(err)
	}
	if read(reopened).AgentID != initial.AgentID {
		t.Fatal("restart overwrote an existing maintenance draft")
	}

	activeDir := t.TempDir()
	if err := BuildAgentEnvironment(activeDir, initial); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(activeDir, "agent.env"))
	active, err := OpenWithDraft(activeDir, runner, other)
	if err != nil {
		t.Fatal(err)
	}
	if active.active.AgentID != initial.AgentID {
		t.Fatal("existing effective environment was overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(activeDir, "agent.env"))
	if string(after) != string(before) {
		t.Fatal("existing device environment changed on startup")
	}
}
