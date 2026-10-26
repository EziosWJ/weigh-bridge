//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
)

func TestPostgresTicketTitleDefaultAndProtectedTextKey(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// Fresh installs seed the default title directly in the database.
	title, err := sysconfig.ReadTicketTitle(ctx, db.GORM, false)
	if err != nil || title != sysconfig.DefaultTicketTitle {
		t.Fatalf("default title=%q err=%v", title, err)
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
