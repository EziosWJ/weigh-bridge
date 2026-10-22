package config

import (
	"strings"
	"testing"
)

func TestStandaloneRoleRequiresStableLocalIdentityAndRejectsCentralSync(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `database:
  url: postgres://localhost/base
  username: user
  password: password
jwt:
  secret: secret
node:
  role: standalone
  site_id: 17
`)
	t.Setenv("APP_ENV", "prod")
	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Node.Role != "standalone" || cfg.Node.SiteID != 17 {
		t.Fatalf("node config = %+v", cfg.Node)
	}

	cfg.Node.CentralURL = "http://center.invalid"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "standalone node") {
		t.Fatalf("Validate() error = %v, want standalone central-config rejection", err)
	}

	cfg.Node.CentralURL = ""
	cfg.Node.SiteID = 256
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "standalone node") {
		t.Fatalf("Validate() error = %v, want site id range rejection", err)
	}
}
