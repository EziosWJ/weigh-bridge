package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
)

func TestResolveConnectionFromAgentEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	const secret = "env-file-test-secret"
	data := strings.Join([]string{
		"WEIGHBRIDGE_SERVER_URL=https://site.example",
		"WEIGHBRIDGE_SITE_ID=2",
		"WEIGHBRIDGE_SITE_CODE=F002",
		"WEIGHBRIDGE_SITE_NAME=测试厂",
		"WEIGHBRIDGE_SCALE_CODE=WB07",
		"WEIGHBRIDGE_SCALE_NAME=七号地磅",
		"WEIGHBRIDGE_AGENT_ID=agent-seven",
		"WEIGHBRIDGE_PRINT_QUEUE=Printer7",
		"APP_WEIGHING__AGENT_KEY=" + secret,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveConnection("", path, "http://ignored.example", "ignored-agent", "WB99", "ignored-secret", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerURL != "https://site.example" || got.SiteCode != "F002" || got.ScaleCode != "WB07" || got.AgentID != "agent-seven" || got.AgentKey != secret || got.SiteName != "测试厂" {
		t.Fatalf("Agent environment identity fields did not load correctly: server=%q site=%s scale=%s agent=%q siteName=%q", got.ServerURL, got.SiteCode, got.ScaleCode, got.AgentID, got.SiteName)
	}
	if _, err := resolveConnection("", path, "http://127.0.0.1:8900", "agent-seven", "WB99", "", true); err == nil || !strings.Contains(err.Error(), "不得同时指定") {
		t.Fatalf("env-file identity overrides should be rejected: %v", err)
	}
}

func TestResolveConnectionKeepsPortableJSONAndRejectsMixedIdentityFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connection.json")
	input := agentconfig.Connection{
		ServerURL: "https://site.example", SiteCode: "F001", SiteName: "测试厂", ScaleCode: "WB01", AgentID: "agent-one", AgentKey: "json-secret",
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveConnection(path, "", "http://ignored.example", "ignored-agent", "WB99", "", false)
	if err != nil || got != input {
		t.Fatalf("portable JSON connection did not load as expected: server=%q site=%s scale=%s agent=%q err=%v", got.ServerURL, got.SiteCode, got.ScaleCode, got.AgentID, err)
	}
	if _, err := resolveConnection(path, "", "http://other.example", "agent-one", "WB01", "", true); err == nil || !strings.Contains(err.Error(), "不得同时指定") {
		t.Fatalf("JSON identity overrides should be rejected: %v", err)
	}
	if _, err := resolveConnection(path, filepath.Join(t.TempDir(), "agent.env"), "", "", "", "", false); err == nil || !strings.Contains(err.Error(), "不能同时使用") {
		t.Fatalf("JSON and env file should be mutually exclusive: %v", err)
	}
}

func TestResolveConnectionAcceptsKeylessServerAddress(t *testing.T) {
	connection, err := resolveConnection("", "", "http://127.0.0.1:8900", "scale-agent-1", "WB01", "", false)
	if err != nil {
		t.Fatalf("keyless Server address must be accepted: %v", err)
	}
	if connection.ServerURL != "http://127.0.0.1:8900" || connection.AgentKey != "" {
		t.Fatalf("keyless connection should carry only the Server address: %+v", connection)
	}
	withLegacyKey, err := resolveConnection("", "", "http://127.0.0.1:8900", "scale-agent-1", "WB01", "mock-only-secret", false)
	if err != nil || withLegacyKey.AgentKey != "mock-only-secret" {
		t.Fatalf("legacy key remains accepted for diagnostics: server=%q agent=%q scale=%s err=%v", withLegacyKey.ServerURL, withLegacyKey.AgentID, withLegacyKey.ScaleCode, err)
	}
}

func TestResolveConnectionAcceptsKeylessEnvFileAndJSON(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(envPath, []byte("WEIGHBRIDGE_SERVER_URL=https://site.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveConnection("", envPath, "", "", "", "", false); err != nil {
		t.Fatalf("env file with only a Server address must be accepted: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "connection.json")
	if err := os.WriteFile(configPath, []byte(`{"serverUrl":"https://site.example"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveConnection(configPath, "", "", "", "", "", false); err != nil {
		t.Fatalf("portable JSON with only a Server address must be accepted: %v", err)
	}
}

func TestResolveConnectionEnvFileMustExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	if _, err := resolveConnection("", path, "", "", "", "", false); err == nil {
		t.Fatal("missing env-file must not silently fall back to default identity")
	}
}
