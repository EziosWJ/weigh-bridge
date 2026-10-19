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
	if cfg.Weighing.SessionTTL != time.Minute || cfg.Weighing.SampleTTL != 30*time.Second {
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

func TestWeighingInvalidTestingTimeoutsAreRejected(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\n")
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_WEIGHING__SESSION_TTL", "0s")
	if _, err := LoadFromDir(dir); err == nil {
		t.Fatal("zero lease accepted")
	}
	t.Setenv("APP_WEIGHING__SESSION_TTL", "10s")
	t.Setenv("APP_WEIGHING__SAMPLE_TTL", "-1s")
	if _, err := LoadFromDir(dir); err == nil {
		t.Fatal("negative sample lifetime accepted")
	}
}
