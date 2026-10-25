//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
)

func TestPostgresTicketTitleUpgradeAndProtectedTextKey(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// The migration seeds an empty marker so the first API start can preserve
	// the previous YAML title without inventing a value in SQL.
	title, err := sysconfig.ReadTicketTitle(ctx, db.GORM, false)
	if err != nil || title != sysconfig.DefaultTicketTitle {
		t.Fatalf("uninstalled title=%q err=%v", title, err)
	}

	title, err = sysconfig.InstallLegacyTicketTitle(ctx, db.GORM, "示例酒厂过磅单")
	if err != nil || title != "示例酒厂过磅单" {
		t.Fatalf("upgrade title=%q err=%v", title, err)
	}
	// Re-running the upgrade handoff must not replace the preserved value.
	title, err = sysconfig.InstallLegacyTicketTitle(ctx, db.GORM, "另一个标题")
	if err != nil || title != "示例酒厂过磅单" {
		t.Fatalf("repeated upgrade title=%q err=%v", title, err)
	}

	var row sysconfig.Config
	if err := db.GORM.Where("config_key=?", sysconfig.TicketTitleKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.IsBuiltin != 1 || row.ValueType != "TEXT" {
		t.Fatalf("title row=%+v", row)
	}
	service := sysconfig.NewService(sysconfig.NewRepository(db.GORM))
	if err := service.Update(ctx, audit.Metadata{}, row.ID, sysconfig.Input{ConfigValue: "  新酒厂过磅单  ", ConfigName: "ignored"}); err != nil {
		t.Fatalf("update title=%v", err)
	}
	title, err = sysconfig.ReadTicketTitle(ctx, db.GORM, false)
	if err != nil || title != "新酒厂过磅单" {
		t.Fatalf("saved title=%q err=%v", title, err)
	}
	if err := service.Update(ctx, audit.Metadata{}, row.ID, sysconfig.Input{ConfigValue: " "}); !errors.Is(err, sysconfig.ErrInvalid) {
		t.Fatalf("blank title error=%v", err)
	}
	if err := service.Status(ctx, audit.Metadata{}, row.ID, 0); !errors.Is(err, sysconfig.ErrBuiltin) {
		t.Fatalf("title status error=%v", err)
	}
}
