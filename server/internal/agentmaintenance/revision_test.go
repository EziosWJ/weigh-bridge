package agentmaintenance

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestEnvironmentRemainsAuthoritativeAcrossRestartAndExternalEdit(t *testing.T) {
	dir := t.TempDir()
	original := Defaults()
	original.AgentKey = "private-key"
	if err := BuildAgentEnvironment(dir, original); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	s.draft.PrintWidthMM = 170
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	updated := original
	updated.ServerURL, updated.Queue, updated.ScaleCode = "https://new.example", "new-queue", "WB08"
	if err := BuildAgentEnvironment(dir, updated); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if s.active != updated || s.draft.PrintWidthMM != 170 || s.draft.Queue != original.Queue {
		t.Fatal("restart ignored env or replaced saved draft")
	}
	cookie := login(t, s.Handler(), dir)
	updated.AgentID = "updated-outside-maintenance"
	if err := BuildAgentEnvironment(dir, updated); err != nil {
		t.Fatal(err)
	}
	w, _ := call(t, s.Handler(), http.MethodGet, "/api/config", "", cookie)
	var response struct {
		Active           Config
		EnvironmentKnown bool
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.EnvironmentKnown || response.Active.AgentID != updated.AgentID || response.Active.AgentKey != "" || strings.Contains(w.Body.String(), updated.AgentKey) {
		t.Fatalf("config sync or redaction failed: %s", w.Body)
	}
}

func TestRuntimeConfigurationReportsMismatchAndStaleFeedback(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, &fakeRunner{outputs: map[string]Result{"systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"}}})
	if err != nil {
		t.Fatal(err)
	}
	c := Defaults()
	for _, tc := range []struct {
		digest string
		at     time.Time
		want   string
	}{
		{agentprinter.ConfigDigest(c), time.Now(), "运行配置与 agent.env 一致"},
		{"different", time.Now(), "不一致"},
		{agentprinter.ConfigDigest(c), time.Now().Add(-time.Minute), "未知"},
	} {
		b, _ := json.Marshal(map[string]any{"at": tc.at, "configDigest": tc.digest})
		if err := os.WriteFile(filepath.Join(dir, "maintenance-status.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if got := s.runtimeConfigState(c); !strings.Contains(got, tc.want) {
			t.Fatalf("runtime state: %q, want %q", got, tc.want)
		}
	}
}

func TestTrustedLANHTTPAndOptionalTLS(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:18901", ":18901", "[::]:18901", "127.0.0.1:18901", "192.168.1.8:18901", "[::1]:18901"} {
		if err := ValidateListener(addr, "", ""); err != nil {
			t.Errorf("HTTP %s rejected: %v", addr, err)
		}
		if err := ValidateListener(addr, "cert.pem", "key.pem"); err != nil {
			t.Errorf("TLS %s rejected: %v", addr, err)
		}
	}
	for _, addr := range []string{"8.8.8.8:18901", "localhost:18901", "0.0.0.0:70000", "0.0.0.0:not-a-port"} {
		if ValidateListener(addr, "", "") == nil {
			t.Errorf("invalid listener accepted: %s", addr)
		}
	}
	if ValidateListener("0.0.0.0:18901", "cert.pem", "") == nil {
		t.Fatal("incomplete TLS accepted")
	}
}

func TestQueueListIncludesEachQueueState(t *testing.T) {
	s, err := Open(t.TempDir(), &fakeRunner{outputs: map[string]Result{"lpstat -p": {Output: "printer first is idle. enabled\nprinter second disabled since now"}}})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := call(t, s.Handler(), http.MethodGet, "/api/status", "", nil)
	var response struct {
		QueueDetails []struct{ Name, State string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.QueueDetails) != 2 || response.QueueDetails[0].State != "已启用" || response.QueueDetails[1].State != "暂停" {
		t.Fatalf("queue states: %s", w.Body)
	}
}
