package agentmaintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

func applyRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]Result{
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"},
		"systemctl --user stop weighbridge-agent-printer.service":      {},
		"systemctl --user start weighbridge-agent-printer.service":     {},
		"lpstat -p":                  {Output: "printer Aisino-SK820-raw is idle. enabled\nprinter backup is idle. enabled"},
		"lpstat -p Aisino-SK820-raw": {Output: "printer Aisino-SK820-raw is idle. enabled"},
		"lpstat -d":                  {Output: "system default destination: Aisino-SK820-raw"},
	}}
}

func deviceSettings(port string) p.DeviceSettings {
	return p.DeviceSettings{
		Serial: p.SerialSettings{Port: port, Baud: 19200},
		Print: p.PrintSettings{
			Queue: "backup", FormLengthMM: 93.13, PrintWidthMM: 175,
			PrintScale: 1.02, OffsetXMM: 4, OffsetYMM: 2,
		},
	}
}

func TestApplyDeviceSettingsIsSingleVerifiedOperationAndReportsBackup(t *testing.T) {
	dir := t.TempDir()
	runner := applyRunner()
	s, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.ConfigBackup()
	if before.ConfigVersion < 1 {
		t.Fatalf("existing configuration has no version: %+v", before)
	}
	wanted := deviceSettings("/dev/ttyUSB0")
	result := s.ApplySettings(context.Background(), p.ConfigApplyRequest{BaseVersion: before.ConfigVersion, Settings: wanted})
	if result.Status != p.ConfigApplyApplied || result.Effective == nil {
		t.Fatalf("apply status=%s code=%s message=%s", result.Status, result.Code, result.Message)
	}
	if result.ConfigVersion != before.ConfigVersion+1 || result.Digest != wanted.Digest() {
		t.Fatalf("apply did not record one new version/digest: %+v", result)
	}
	stored, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), agentprinter.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if stored.SerialPort != "/dev/ttyUSB0" || stored.SerialBaud != 19200 || stored.Queue != "backup" || stored.FormLengthMM != 93.13 || stored.PrintScale != 1.02 {
		t.Fatalf("applied settings not persisted: %+v", stored)
	}
	backup, ok := s.ConfigBackup()
	if !ok || backup.Digest != wanted.Digest() || backup.ConfigVersion != result.ConfigVersion {
		t.Fatalf("success backup not updated: %+v", backup)
	}
	if result.Effective.Serial.Port != "/dev/ttyUSB0" || result.Effective.Print.Queue != "backup" || result.Effective.DeviceState != "running" {
		t.Fatalf("effective report is not the applied truth: %+v", result.Effective)
	}
	// Reloading from disk keeps the applied version and backup, so the Server
	// can compare it after a reconnect.
	reopened, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, ok := reopened.ConfigBackup()
	if !ok || reloaded.ConfigVersion != result.ConfigVersion || reloaded.Digest != wanted.Digest() {
		t.Fatalf("reopen lost applied version/backup: %+v", reloaded)
	}
}

func TestApplyRejectsStaleVersionAndInvalidSettingsWithoutChangingDevice(t *testing.T) {
	dir := t.TempDir()
	runner := applyRunner()
	s, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := s.ConfigBackup()
	stale := s.ApplySettings(context.Background(), p.ConfigApplyRequest{BaseVersion: original.ConfigVersion + 5, Settings: deviceSettings("/dev/ttyUSB0")})
	if stale.Status != p.ConfigApplyRejected || stale.Code != p.ErrorConfigConflict {
		t.Fatalf("stale version accepted: %+v", stale)
	}
	invalid := deviceSettings("/dev/ttyUSB0")
	invalid.Serial.Baud = 1234
	rejected := s.ApplySettings(context.Background(), p.ConfigApplyRequest{Settings: invalid})
	if rejected.Status != p.ConfigApplyRejected || rejected.Code != p.ErrorConfigInvalid {
		t.Fatalf("invalid baud accepted: %+v", rejected)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.env")); !os.IsNotExist(err) {
		t.Fatal("rejected configuration wrote the device environment")
	}
	after, _ := s.ConfigBackup()
	if after.ConfigVersion != original.ConfigVersion || after.Digest != original.Digest {
		t.Fatalf("rejection changed the applied fact: %+v", after)
	}
}

type failStartOnceRunner struct {
	*fakeRunner
	failed bool
}

func (r *failStartOnceRunner) Run(args []string) Result {
	if !r.failed && strings.Join(args, " ") == "systemctl --user start weighbridge-agent-printer.service" {
		r.failed = true
		return Result{Err: errors.New("start failed")}
	}
	return r.fakeRunner.Run(args)
}

func TestApplyFailureRollsBackAndKeepsMaintenanceConnection(t *testing.T) {
	dir := t.TempDir()
	runner := &failStartOnceRunner{fakeRunner: applyRunner()}
	s, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := s.ConfigBackup()
	result := s.ApplySettings(context.Background(), p.ConfigApplyRequest{Settings: deviceSettings("/dev/ttyUSB0")})
	if result.Status != p.ConfigApplyRolledBack || result.Code != p.ErrorRollbackFailed {
		t.Fatalf("failed apply did not report rollback: %+v", result)
	}
	if result.ConfigVersion != original.ConfigVersion {
		t.Fatalf("failed apply advanced the version: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.env")); !os.IsNotExist(err) {
		t.Fatal("failed apply left the candidate environment in place")
	}
	backup, _ := s.ConfigBackup()
	if backup.Digest != original.Digest {
		t.Fatalf("rollback did not restore the last success backup: %+v", backup)
	}
}

func TestRestoreAppliesServerHeldBackup(t *testing.T) {
	dir := t.TempDir()
	runner := applyRunner()
	s, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := s.ConfigBackup()
	result := s.RestoreSettings(context.Background(), p.ConfigRestoreRequest{
		BackupID: 9, Backup: p.ConfigBackup{ConfigVersion: original.ConfigVersion, Digest: original.Digest, Settings: original.Settings, AppliedAt: original.AppliedAt},
	})
	if result.Status != p.ConfigApplyApplied {
		t.Fatalf("restore failed: %+v", result)
	}
	effective := s.EffectiveConfig()
	if effective.Digest != original.Digest {
		t.Fatalf("restore did not report the restored settings: %+v", effective)
	}
}

func TestQueuesComeFromAgentCUPSAndMarkDefault(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, applyRunner())
	if err != nil {
		t.Fatal(err)
	}
	result := s.Queues()
	if result.Code != "" || len(result.Queues) != 2 {
		t.Fatalf("queues = %+v", result)
	}
	if result.Queues[0].Name != "Aisino-SK820-raw" || !result.Queues[0].IsDefault {
		t.Fatalf("default queue not marked: %+v", result.Queues)
	}
	if result.Queues[1].Name != "backup" || result.Queues[1].IsDefault {
		t.Fatalf("non-default queue marked: %+v", result.Queues)
	}
}

func TestQueuesUnavailableIsExplicit(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{outputs: map[string]Result{"lpstat -p": {Err: errors.New("cups down")}}}
	s, err := Open(dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	result := s.Queues()
	if result.Code != p.ErrorQueuesUnavailable || !strings.Contains(result.Error, "cups down") {
		t.Fatalf("queue failure not explicit: %+v", result)
	}
}
