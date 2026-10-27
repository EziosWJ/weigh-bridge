//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/app"
)

func TestManualDeviceBackupRepository(t *testing.T) {
	temporary := startBusinessPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	deps := testDependencies(t, database, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	ensureLocalSite(t, router, loginAdmin(t, router), 1)
	ctx := context.Background()
	var scale string
	if err := database.GORM.Raw(`SELECT code FROM wb_scale ORDER BY code LIMIT 1`).Scan(&scale).Error; err != nil {
		t.Fatal(err)
	}
	if scale == "" {
		t.Fatal("missing initialized scale")
	}
	repo := agentmaintenance.NewRepository(database.GORM)
	settings := p.DeviceSettings{Serial: p.SerialSettings{Port: "/dev/ttyUSB0", Baud: 9600}, Print: p.PrintSettings{Queue: "printer", FormLengthMM: 93.13, PrintWidthMM: 175, PrintScale: 1}}
	backup := p.ConfigBackup{ConfigVersion: 3, Digest: settings.Digest(), Settings: settings, AppliedAt: time.Now().UTC()}
	// Existing automatic records remain visible and recoverable after migration.
	if err := database.GORM.Exec(`INSERT INTO wb_agent_config_backup (scale_code,config_version,digest,settings,applied_at,source) VALUES (?,1,'legacy','{}',CURRENT_TIMESTAMP,'auto')`, scale).Error; err != nil {
		t.Fatal(err)
	}
	// Older restore points remain local after the business scale code changes.
	if err := database.GORM.Exec("UPDATE wb_agent_config_backup SET scale_code='OLD_SCALE' WHERE digest='legacy'").Error; err != nil {
		t.Fatal(err)
	}
	effective := p.EffectiveConfig{ConfigVersion: 3, Digest: backup.Digest, Serial: settings.Serial, Print: settings.Print, AppliedAt: backup.AppliedAt}
	if err := repo.SaveEffectiveConfig(ctx, scale, effective); err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Exec("UPDATE wb_agent_runtime SET scale_code='OLD_SCALE' WHERE local_device").Error; err != nil {
		t.Fatal(err)
	}
	runtime, found, err := repo.AgentRuntime(ctx, scale)
	if err != nil || !found || runtime.EffectiveConfigDigest != backup.Digest {
		t.Fatalf("historical runtime=%+v found=%v err=%v", runtime, found, err)
	}
	if err := repo.SaveEffectiveConfig(ctx, scale+"-other", effective); !errors.Is(err, agentmaintenance.ErrLocalScaleChanged) {
		t.Fatalf("stale device report accepted: %v", err)
	}
	id, err := repo.CreateConfigBackup(ctx, scale, backup, "现场校准完成", "五联纸已核对", 7)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.CreateConfigBackup(ctx, scale, backup, "再次确认", "", 7)
	if err != nil || second == id {
		t.Fatalf("distinct manual restore points: %d %v", second, err)
	}
	records, total, err := repo.ListConfigBackups(ctx, scale, 0, 10)
	if err != nil || total != 3 || len(records) != 3 {
		t.Fatalf("list=%+v total=%d err=%v", records, total, err)
	}
	record, err := repo.ConfigBackupByID(ctx, scale, id)
	if err != nil || record.Name != "现场校准完成" || record.Notes != "五联纸已核对" || record.CreatedBy == nil || *record.CreatedBy != 7 || record.Settings.Print.FormLengthMM != 93.13 || record.Source != "manual" {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if _, err := repo.ConfigBackupByID(ctx, scale+"-other", id); !errors.Is(err, agentmaintenance.ErrNoBackup) {
		t.Fatalf("cross-scale read=%v", err)
	}
	if err := repo.DeleteConfigBackup(ctx, scale+"-other", id); !errors.Is(err, agentmaintenance.ErrNoBackup) {
		t.Fatalf("cross-scale delete=%v", err)
	}
	if err := repo.DeleteConfigBackup(ctx, scale, id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfigBackupByID(ctx, scale, id); !errors.Is(err, agentmaintenance.ErrNoBackup) {
		t.Fatalf("deleted backup available=%v", err)
	}
	_, total, err = repo.ListConfigBackups(ctx, scale, 0, 10)
	if err != nil || total != 2 {
		t.Fatalf("delete total=%d err=%v", total, err)
	}
}
