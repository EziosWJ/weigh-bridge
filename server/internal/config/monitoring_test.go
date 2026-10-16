package config

import (
	"strings"
	"testing"
	"time"
)

func TestMonitoringConfigurationDefaultsAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\n")
	t.Setenv("APP_ENV", "test")
	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitoring.Enabled || cfg.Monitoring.Source != "native" || cfg.Monitoring.SocketPath != "/run/weighbridge/host-monitor.sock" || cfg.Monitoring.Timeout != 4*time.Second {
		t.Fatalf("monitoring defaults = %+v", cfg.Monitoring)
	}
	t.Setenv("APP_MONITORING__SOURCE", "unix")
	t.Setenv("APP_MONITORING__ENABLED", "true")
	t.Setenv("APP_MONITORING__SOCKET_PATH", "/tmp/test-monitor.sock")
	t.Setenv("APP_MONITORING__TIMEOUT", "5s")
	cfg, err = LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Monitoring.Enabled || cfg.Monitoring.Source != "unix" || cfg.Monitoring.SocketPath != "/tmp/test-monitor.sock" || cfg.Monitoring.Timeout != 5*time.Second {
		t.Fatalf("monitoring overrides = %+v", cfg.Monitoring)
	}
}

func TestMonitoringConfigurationRejectsInvalidSourcePathAndTimeout(t *testing.T) {
	t.Setenv("APP_MONITORING__ENABLED", "true")
	for _, test := range []struct {
		name, yaml, want string
	}{
		{"source", "source: container", "monitoring.source"},
		{"relative socket", "source: unix\n  socket_path: relative.sock", "monitoring.socket_path"},
		{"empty socket", "source: unix\n  socket_path: ''", "monitoring.socket_path"},
		{"zero timeout", "timeout: 0s", "monitoring.timeout"},
		{"negative timeout", "timeout: -1s", "monitoring.timeout"},
		{"excess timeout", "timeout: 5001ms", "monitoring.timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\nmonitoring:\n  "+test.yaml+"\n")
			t.Setenv("APP_ENV", "test")
			_, err := LoadFromDir(dir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want %s", err, test.want)
			}
		})
	}
}

func TestDisabledMonitoringDoesNotRequireCollectorConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost/base\n  username: user\n  password: password\njwt:\n  secret: secret\nmonitoring:\n  enabled: false\n  source: unix\n  socket_path: relative.sock\n  timeout: 0s\n")
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_MONITORING__ENABLED", "false")
	if _, err := LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_MONITORING__ENABLED", "true")
	if _, err := LoadFromDir(dir); err == nil {
		t.Fatal("enabled monitoring accepted an invalid collector configuration")
	}
}
