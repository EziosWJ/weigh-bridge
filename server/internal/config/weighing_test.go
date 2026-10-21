package config

import (
	"testing"
	"time"
)

func TestWeighingAgentKeyRequiresExplicitConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\n")
	t.Setenv("APP_ENV", "prod")
	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Weighing.SessionTTL != time.Minute {
		t.Fatalf("weighing defaults=%+v", cfg.Weighing)
	}
	if cfg.Weighing.AgentKey != "" {
		t.Fatal("Agent connection must require explicit key")
	}
	t.Setenv("APP_WEIGHING__AGENT_KEY", "integration-agent-key")
	cfg, err = LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Weighing.AgentKey != "integration-agent-key" {
		t.Fatal("Agent key configuration override was ignored")
	}
}

func TestWeighingInvalidSessionTimeoutIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\n")
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_WEIGHING__SESSION_TTL", "0s")
	if _, err := LoadFromDir(dir); err == nil {
		t.Fatal("zero lease accepted")
	}
}

func TestMultipleAgentBindings(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `database:
  url: postgres://localhost/base
  username: user
  password: password
jwt:
  secret: secret
weighing:
  image_dir: /tmp/agent-test-images
  agents:
    - id: site-agent-1
      scale_id: 1
      key: test-only-key-1
    - id: site-agent-2
      scale_id: 2
      key: test-only-key-2
`)
	t.Setenv("APP_ENV", "prod")
	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Weighing.Agents) != 2 || cfg.Weighing.Agents[1].ScaleID != 2 {
		t.Fatalf("bindings: %+v", cfg.Weighing.Agents)
	}
	cfg.Weighing.Agents[1].ScaleID = 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("duplicate scale accepted")
	}
	cfg.Weighing.Agents[1].ScaleID = 2
	cfg.Weighing.Agents[1].Key = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("empty binding key accepted")
	}
}
