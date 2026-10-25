package config

import (
	"strings"
	"testing"
)

func TestLocalConfigurationRejectsRetiredNodeSettings(t *testing.T) {
	for _, role := range []string{"central", "site", "standalone", ""} {
		t.Run(role, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", `database:
  url: postgres://localhost/base
  username: user
  password: password
jwt:
  secret: secret
node:
  role: "`+role+`"
  site_id: 1
`)
			t.Setenv("APP_ENV", "prod")
			_, err := LoadFromDir(dir)
			if err == nil || !strings.Contains(err.Error(), "node configuration has been removed") {
				t.Fatalf("expected retired setting error, got %v", err)
			}
		})
	}
}
func TestLocalConfigurationNeedsNoFactoryYAML(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `database:
  url: postgres://localhost/base
  username: user
  password: password
jwt:
  secret: secret
`)
	t.Setenv("APP_ENV", "prod")
	if _, err := LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredNodeEnvironmentIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `database:
  url: postgres://localhost/base
  username: user
  password: password
jwt:
  secret: secret
`)
	t.Setenv("APP_ENV", "prod")
	t.Setenv("APP_NODE__ROLE", "standalone")
	_, err := LoadFromDir(dir)
	if err == nil || !strings.Contains(err.Error(), "node configuration has been removed") {
		t.Fatalf("expected retired environment error, got %v", err)
	}
}
