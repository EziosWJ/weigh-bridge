package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyEnvironment = `# old local settings
WEIGHBRIDGE_SERVER_URL="http://old.local"
WEIGHBRIDGE_AGENT_ID="agent-01"
WEIGHBRIDGE_SCALE_ID=1
WEIGHBRIDGE_SITE_ID=1
WEIGHBRIDGE_SITE_CODE="SITE-01"
WEIGHBRIDGE_SITE_NAME="一厂"
WEIGHBRIDGE_SCALE_CODE="SCALE-01"
WEIGHBRIDGE_SCALE_NAME="一号磅"
APP_WEIGHING__AGENT_KEY="secret-key"
WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"
WEIGHBRIDGE_PRINT_WIDTH_MM=179
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyUSB0"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
WEIGHBRIDGE_CAMERA_FRONT_URL="rtsp://operator:camera-secret@10.0.0.10/front"
WEIGHBRIDGE_CAMERA_REAR_URL="rtsp://10.0.0.10/rear"
WEIGHBRIDGE_CAMERA_TIMEOUT_MS=2500
WEIGHBRIDGE_PLATE_RECOGNITION_URL="http://recognizer.local/plate"
CUSTOM_DEVICE_NOTE=keep-me
`

func TestNewEnvironmentNeedsOnlyServerAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := run(path, options{ServerURL: "http://127.0.0.1:18900"}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{"APP_WEIGHING__AGENT_KEY", "WEIGHBRIDGE_AGENT_ID", "WEIGHBRIDGE_SCALE_ID", "WEIGHBRIDGE_CAMERA_", "WEIGHBRIDGE_SITE_"} {
		if strings.Contains(string(contents), unexpected) {
			t.Fatalf("new keyless environment contains %s:\n%s", unexpected, contents)
		}
	}
	for _, expected := range []string{
		`WEIGHBRIDGE_SERVER_URL="http://127.0.0.1:18900"`,
		`WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"`,
		`WEIGHBRIDGE_SERIAL_PORT=""`,
		"WEIGHBRIDGE_SERIAL_BAUD=9600",
	} {
		if !strings.Contains(string(contents), expected) {
			t.Fatalf("new environment missing %q:\n%s", expected, contents)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("environment mode = %v err=%v", info.Mode().Perm(), err)
	}
}

func TestExistingEnvironmentRequiresExplicitUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(path, []byte(legacyEnvironment), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run(path, options{})
	if err == nil || !strings.Contains(err.Error(), "--update-connection") {
		t.Fatalf("existing file should require explicit update, got %v", err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != legacyEnvironment {
		t.Fatal("refused update modified existing agent.env")
	}
}

func TestUpgradeRemovesLegacyIdentityAndCameraButPreservesDeviceSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(path, []byte(legacyEnvironment), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(path, options{Update: true, ServerURL: "http://192.168.1.20:18900"}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, removed := range []string{
		"APP_WEIGHING__AGENT_KEY", "WEIGHBRIDGE_AGENT_ID", "WEIGHBRIDGE_SCALE_ID",
		"WEIGHBRIDGE_SITE_ID", "WEIGHBRIDGE_SITE_CODE", "WEIGHBRIDGE_SITE_NAME",
		"WEIGHBRIDGE_SCALE_CODE", "WEIGHBRIDGE_SCALE_NAME", "WEIGHBRIDGE_CAMERA_",
		"WEIGHBRIDGE_PLATE_RECOGNITION_URL", "WEIGHBRIDGE_SERIAL_ENABLED",
		"WEIGHBRIDGE_SERIAL_PROTOCOL", "WEIGHBRIDGE_SERIAL_WEIGHT_UNIT", "WEIGHBRIDGE_SERIAL_STALE_MS",
	} {
		if strings.Contains(text, removed) {
			t.Fatalf("deprecated field %s survived migration:\n%s", removed, text)
		}
	}
	for _, preserved := range []string{
		`WEIGHBRIDGE_SERVER_URL="http://192.168.1.20:18900"`,
		`WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"`,
		"WEIGHBRIDGE_PRINT_WIDTH_MM=179",
		`WEIGHBRIDGE_SERIAL_PORT="/dev/ttyUSB0"`,
		"WEIGHBRIDGE_SERIAL_BAUD=9600",
		"CUSTOM_DEVICE_NOTE=keep-me",
		"# old local settings",
	} {
		if !strings.Contains(text, preserved) {
			t.Fatalf("migration lost %q:\n%s", preserved, text)
		}
	}
	if strings.Contains(text, "camera-secret") {
		t.Fatalf("migration kept a camera credential in agent.env:\n%s", text)
	}

	backupPath := path + ".legacy-camera.json"
	info, err := os.Stat(backupPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("camera backup mode = %v err=%v", info.Mode().Perm(), err)
	}
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	var backup cameraBackup
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, item := range backup.Entries {
		values[item.Key] = item.Value
	}
	if values["WEIGHBRIDGE_CAMERA_FRONT_URL"] != "rtsp://operator:camera-secret@10.0.0.10/front" ||
		values["WEIGHBRIDGE_CAMERA_REAR_URL"] != "rtsp://10.0.0.10/rear" ||
		values["WEIGHBRIDGE_PLATE_RECOGNITION_URL"] != "http://recognizer.local/plate" {
		t.Fatalf("camera backup lost entries: %+v", backup.Entries)
	}
}

func TestMigrationKeepsAnExistingCameraBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.env")
	backupPath := path + ".legacy-camera.json"
	existing := `{"migratedAt":"2026-01-01T00:00:00Z","note":"earlier","entries":[{"key":"WEIGHBRIDGE_CAMERA_FRONT_URL","value":"rtsp://earlier/front"}]}`
	if err := os.WriteFile(backupPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("WEIGHBRIDGE_SERVER_URL=\"http://127.0.0.1:18900\"\nWEIGHBRIDGE_CAMERA_REAR_URL=\"rtsp://new/rear\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(path, options{Update: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	var backup cameraBackup
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatal(err)
	}
	if len(backup.Entries) != 2 {
		t.Fatalf("existing backup was lost instead of merged: %+v", backup.Entries)
	}
}

func TestResolveServerURLRequiresValidAddressForNewFiles(t *testing.T) {
	if _, err := resolveServerURL("", "", false); err == nil || !strings.Contains(err.Error(), "--server-url") {
		t.Fatalf("first provisioning without an address must be rejected, got %v", err)
	}
	if _, err := resolveServerURL("not-a-url", "", false); err == nil {
		t.Fatal("invalid explicit address must be rejected")
	}
	if _, err := resolveServerURL("", "http://old.local:8900/", true); err != nil {
		t.Fatalf("existing valid address should be preserved: %v", err)
	}
}

func TestResolveServerURLKeepsOrReplacesExistingAddress(t *testing.T) {
	kept, err := resolveServerURL("", "http://127.0.0.1:8900", true)
	if err != nil || kept != "http://127.0.0.1:8900" {
		t.Fatalf("existing address not preserved: %q %v", kept, err)
	}
	replaced, err := resolveServerURL("http://192.168.1.20:8900/", "http://127.0.0.1:8900", true)
	if err != nil || replaced != "http://192.168.1.20:8900" {
		t.Fatalf("explicit address not applied: %q %v", replaced, err)
	}
}
