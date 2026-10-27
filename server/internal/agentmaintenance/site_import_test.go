package agentmaintenance

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestSiteConfigurationImportPreviewsAndPreservesLocalPrinting(t *testing.T) {
	dir := t.TempDir()
	original := Defaults()
	original.AgentKey = "old-key"
	original.Queue = "Local-Queue"
	original.PrintWidthMM = 170
	if err := BuildAgentEnvironment(dir, original); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	c := agentconfig.Connection{ServerURL: "http://server.internal:8900", SiteCode: "SITE02", SiteName: "二厂", ScaleCode: "SCALE02", ScaleName: "二厂地磅", AgentID: "agent-2", AgentKey: "new-connection-key"}
	encode := func(confirm bool) string {
		b, _ := json.Marshal(map[string]any{"connection": c, "confirm": confirm})
		return string(b)
	}
	w, _ := call(t, h, http.MethodPost, "/api/config/import", encode(false), cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "二厂") || strings.Contains(w.Body.String(), c.AgentKey) {
		t.Fatalf("preview failed %s", w.Body)
	}
	if s.draft != original || s.active != original {
		t.Fatal("preview mutated configuration")
	}
	w, _ = call(t, h, http.MethodPost, "/api/config/import", encode(true), cookie)
	if w.Code != 200 {
		t.Fatalf("import: %s", w.Body)
	}
	if s.draft.SiteName != "二厂" || s.draft.AgentKey != c.AgentKey || s.draft.Queue != original.Queue || s.draft.PrintWidthMM != original.PrintWidthMM || s.active != original {
		t.Fatal("import changed active configuration or local settings")
	}
	env, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil || env != original {
		t.Fatal("import overwrote running env")
	}
	if err := BuildAgentEnvironment(dir, s.draft); err != nil {
		t.Fatal(err)
	}
	env, err = agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	// Connection identity round-trips; display metadata is intentionally not
	// written to agent.env any more.
	if env.ServerURL != s.draft.ServerURL || env.AgentID != s.draft.AgentID || env.AgentKey != s.draft.AgentKey || env.ScaleCode != s.draft.ScaleCode || env.Queue != s.draft.Queue {
		t.Fatalf("connection identity did not round trip: %+v", env)
	}
	if env.SiteName != "" || env.ScaleName != "" || env.SiteCode != "" {
		t.Fatalf("display metadata must not be written to agent.env: %+v", env)
	}
	audit, err := os.ReadFile(filepath.Join(dir, "maintenance.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(audit), c.AgentKey) {
		t.Fatal("key was logged")
	}
}
