package config

import (
	"testing"
	"time"
)

func TestWeighingSessionTimeoutDefaultsAndRejectsInvalidValue(t *testing.T) {
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
	t.Setenv("APP_WEIGHING__SESSION_TTL", "0s")
	if _, err := LoadFromDir(dir); err == nil {
		t.Fatal("zero lease accepted")
	}
}
