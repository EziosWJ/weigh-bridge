package agentmaintenance

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestFractionalCalibrationSaveApplyReopenAndRestore(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{outputs: map[string]Result{
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"},
		"lpstat -p": {Output: "printer Aisino-SK820-raw is idle. enabled"},
	}}
	initial := Defaults()
	initial.AgentKey = "calibration-test-key"
	if err := BuildAgentEnvironment(dir, initial); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, f)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	c := initial
	c.PrintScale, c.FormLengthMM, c.OffsetXMM, c.OffsetYMM = 1.02, 93.13, 4, -3
	b, _ := json.Marshal(c)
	w, _ := call(t, h, http.MethodPut, "/api/config", string(b), cookie)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	before, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil || before != initial {
		t.Fatalf("draft changed effective settings: %+v %v", before, err)
	}
	w, _ = call(t, h, http.MethodPost, "/api/action", `{"action":"apply"}`, cookie)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	effective, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil || effective != c {
		t.Fatalf("precision lost during apply: %+v %v", effective, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "agent.env"))
	if !strings.Contains(string(env), "WEIGHBRIDGE_FORM_LENGTH_MM=93.13\n") || !strings.Contains(string(env), "WEIGHBRIDGE_PRINT_SCALE=1.02\n") {
		t.Fatalf("wrong calibration encoding")
	}
	reopened, err := Open(dir, f)
	if err != nil || reopened.active != c || reopened.draft != c {
		t.Fatalf("reopen lost calibration: %v", err)
	}
	w, _ = call(t, h, http.MethodPost, "/api/action", `{"action":"restore"}`, cookie)
	if w.Code != 200 {
		t.Fatalf("restore: %d %s", w.Code, w.Body)
	}
	effective, err = agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil || effective != initial {
		t.Fatalf("A4 settings not restored: %+v %v", effective, err)
	}
}

func TestLegacyCalibrationDefaultsToUnitScale(t *testing.T) {
	dir := t.TempDir()
	c := Defaults()
	c.FormLengthMM = 99 // Existing A4-era environment, independent of current new-install defaults.
	c.AgentKey = "legacy-key"
	if err := BuildAgentEnvironment(dir, c); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent.env")
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(b), "WEIGHBRIDGE_PRINT_SCALE=1\n", "")), 0600); err != nil {
		t.Fatal(err)
	}
	// A prior-version draft has no printScale field.
	legacy := `{"draft":{"serverUrl":"http://127.0.0.1:18900","agentId":"scale-agent-1","scaleId":1,"queue":"Aisino-SK820-raw","agentKey":"legacy-key","printWidthMm":175,"formLengthMm":99},"passwordHash":"existing-hash"}`
	if err := os.WriteFile(filepath.Join(dir, "maintenance.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if s.draft.PrintScale != 1 || s.active.PrintScale != 1 || s.draft != s.active {
		t.Fatal("legacy calibration acquired an unsaved scaling change")
	}
}
