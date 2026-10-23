package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/knadh/koanf/parsers/yaml"
)

func TestInitializeBackfillsMaintenanceWithoutChangingCredentialsOrRole(t *testing.T) {
	checkout(t)
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	original, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if original.Maintenance.Enabled == nil || !*original.Maintenance.Enabled {
		t.Fatal("new development config did not enable Agent maintenance")
	}
	path := filepath.Join("configs", "config.dev.yaml")
	contents, _ := os.ReadFile(path)
	doc, err := yaml.Parser().Unmarshal(contents)
	if err != nil {
		t.Fatal(err)
	}
	delete(doc, "maintenance")
	legacy, err := yaml.Parser().Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	updated, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Maintenance.Enabled == nil || !*updated.Maintenance.Enabled {
		t.Fatal("existing development config did not enable Agent maintenance")
	}
	if updated.JWT != original.JWT || updated.Weighing.AgentKey != original.Weighing.AgentKey || !reflect.DeepEqual(updated.Node, original.Node) || updated.Database != original.Database {
		t.Fatal("backfill changed existing credentials or deployment identity")
	}
	before, _ := os.ReadFile(path)
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("repeated initialization changed configured maintenance")
	}
	for _, text := range []string{"maintenance:\n  enabled: false\n", "maintenance:\n  data_dir: \"\"\n", "node:\n  role: site\n", "maintenance: {data_dir: /custom/agent}\n"} {
		if out, changed, err := withMaintenanceDirectory(text); err != nil || changed || out != text {
			t.Fatal("explicit disable/custom path or named deployment role must be preserved")
		}
	}
}

func TestNamedDeploymentInitDoesNotGenerateFallbackDeviceKey(t *testing.T) {
	for _, role := range []string{"standalone", "site", "central"} {
		contents := "node:\n  role: " + role + "\nweighing:\n  agent_key: \"\"\n"
		out, changed, err := withAgentKey(contents)
		if err != nil || changed || out != contents {
			t.Fatal("named deployment must retain its existing credential source")
		}
	}
}

func checkout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	backend := filepath.Join(root, "server")
	if err := os.MkdirAll(filepath.Join(backend, "configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.yaml", "config.dev.example.yaml"} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "configs", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backend, "configs", name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Tests never use real deployment configuration or environment credentials.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "APP_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_CONFIG_PROFILE", "")
	t.Chdir(backend)
	return root
}

func invoke(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := run(context.Background(), args, &output)
	if err != nil {
		output.WriteString(err.Error())
	}
	return output.String(), err
}

func TestInitializePostgresAndPreserveConfiguration(t *testing.T) {
	for _, driver := range []string{"postgres"} {
		t.Run(driver, func(t *testing.T) {
			root := checkout(t)
			args := []string{"init"}
			output, err := invoke(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Database.Driver != driver || cfg.File.StorageRoot != filepath.Join(root, ".data", "uploads") {
				t.Fatal("generated configuration has wrong database or upload directory")
			}
			if len(cfg.JWT.Secret) != 64 || strings.Contains(output, cfg.JWT.Secret) {
				t.Fatal("JWT must be randomly generated and excluded from output")
			}
			if len(cfg.Weighing.AgentKey) != 64 || cfg.Weighing.AgentKey == cfg.JWT.Secret || strings.Contains(output, cfg.Weighing.AgentKey) {
				t.Fatal("initialization must generate a separate Agent key without printing it")
			}
			if !strings.Contains(output, "task dev") {
				t.Fatal("initialization must include the matching startup task")
			}
			path := filepath.Join("configs", "config.dev.yaml")
			before, _ := os.ReadFile(path)
			if !bytes.Contains(before, []byte("# 生成时会填为本检出的绝对路径")) {
				t.Fatal("generated configuration must keep the template comments")
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm()&0o077 != 0 && os.PathSeparator != '\\' {
				t.Fatal("generated credentials must have restricted file permissions")
			}
			// Repeated initialization must preserve an existing file.
			output, err = invoke(t, "init", "--database", "postgres")
			if err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) || !strings.Contains(output, "[保留]") {
				t.Fatal("repeated initialization modified existing configuration")
			}
		})
	}
}

func TestInitializeRefusesUnexpectedWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	// A configs directory outside a server checkout: the template is
	// readable, but storage_root cannot be resolved to this project.
	elsewhere := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(filepath.Join(elsewhere, "configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.dev.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "configs", "config.dev.example.yaml"), template, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)
	output, err := invoke(t, "init")
	if err == nil || !strings.Contains(output, "无法定位项目根目录") {
		t.Fatalf("expected an unresolved project root error: %s", output)
	}
	if _, err := os.Stat(filepath.Join("configs", "config.dev.yaml")); !os.IsNotExist(err) {
		t.Fatal("an unresolved project root must not write configuration")
	}
}

func TestInitializePreservesMalformedExistingFile(t *testing.T) {
	checkout(t)
	path := filepath.Join("configs", "config.dev.yaml")
	before := []byte("custom-existing-configuration: [broken")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid existing configuration must also be preserved")
	}
}

func TestDoctorConfigurationErrorsDoNotExposeValues(t *testing.T) {
	checkout(t)
	credential := "very-private-development-credential"
	if err := os.WriteFile(filepath.Join("configs", "config.dev.yaml"), []byte("jwt: ["+credential+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := invoke(t, "doctor")
	if err == nil || strings.Contains(output, credential) || !strings.Contains(output, "YAML") {
		t.Fatal("doctor must report a sanitized configuration error")
	}
}

func TestDoctorPostgresFailureIsBoundedAndSanitized(t *testing.T) {
	checkout(t)
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_DATABASE__URL", "postgres://127.0.0.1:1/unavailable?sslmode=disable")
	t.Setenv("APP_DATABASE__USERNAME", "private-user")
	t.Setenv("APP_DATABASE__PASSWORD", "private-password")
	t.Setenv("APP_JWT__SECRET", "private-jwt-secret")
	started := time.Now()
	output, err := invoke(t, "doctor")
	if err == nil || !strings.Contains(output, "PostgreSQL 未就绪") || time.Since(started) > 7*time.Second {
		t.Fatal("unavailable PostgreSQL must produce a bounded diagnostic")
	}
	for _, value := range []string{"private-user", "private-password", "private-jwt-secret", "postgres://"} {
		if strings.Contains(output, value) {
			t.Fatal("doctor exposed connection or credential values")
		}
	}
}

func TestDoctorPostgresUnresponsiveServerHonorsDeadline(t *testing.T) {
	checkout(t)
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release); _ = listener.Close() })
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			defer connection.Close()
			<-release
		}
	}()
	t.Setenv("APP_DATABASE__URL", "postgres://"+listener.Addr().String()+"/test?sslmode=disable")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	started := time.Now()
	err = run(ctx, []string{"doctor"}, &output)
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatal("unresponsive database must not block beyond the command deadline")
	}
}

func TestInvalidArgumentsDoNotCreateConfiguration(t *testing.T) {
	checkout(t)
	for _, args := range [][]string{{}, {"unknown"}, {"init", "--database", "mysql"}, {"init", "--database", "sqlite"}, {"doctor", "--database", "sqlite"}, {"init", "unexpected"}, {"init", "--password", "private-value"}} {
		output, err := invoke(t, args...)
		if err == nil || strings.Contains(output, "private-value") {
			t.Fatal("invalid arguments must fail without echoing values")
		}
	}
	if _, err := os.Stat("configs/config.dev.yaml"); !os.IsNotExist(err) {
		t.Fatal("invalid arguments created configuration")
	}
}

func TestInitializeBackfillsAgentKeyAndPreservesExistingSettings(t *testing.T) {
	checkout(t)
	if _, err := invoke(t, "init", "--database", "postgres"); err != nil {
		t.Fatal(err)
	}
	original, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("configs", "config.dev.yaml")
	contents, _ := os.ReadFile(path)
	lines := strings.Split(string(contents), "\n")
	kept := []string{}
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "agent_key:") {
			kept = append(kept, line)
		}
	}
	legacy := strings.Join(kept, "\n") + "\n# Keep my local settings verbatim.\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := invoke(t, "init")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Weighing.AgentKey) != 64 {
		t.Fatal("existing local config still has no Agent key")
	}
	if updated.JWT.Secret != original.JWT.Secret || updated.Database != original.Database || updated.File != original.File {
		t.Fatal("backfill changed existing settings")
	}
	if strings.Contains(output, updated.Weighing.AgentKey) {
		t.Fatal("Agent key leaked into output")
	}
	before, _ := os.ReadFile(path)
	if !bytes.Contains(before, []byte("# Keep my local settings verbatim.")) {
		t.Fatal("backfill removed comments")
	}
	if _, err := invoke(t, "init"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("repeated init rotated a configured Agent key")
	}
}
