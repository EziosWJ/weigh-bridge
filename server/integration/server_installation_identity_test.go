//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresInstallationIdentityIsPersistentAndDistinct(t *testing.T) {
	ctx := context.Background()
	ids := make([]string, 0, 2)
	for range 2 {
		pg := startBusinessPostgres(t)
		db := openTemporaryDatabase(t, pg.dsn)
		t.Cleanup(func() { _ = db.Close() })
		repo := weighing.NewRepository(db.GORM)
		service := weighing.NewService(repo, nil)
		if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
			t.Fatal(err)
		}
		first, err := repo.LocalAgentConfig(ctx, t.TempDir())
		if err != nil || first.InstallationID == "" {
			t.Fatalf("installation id missing: %+v err=%v", first, err)
		}
		again, err := weighing.NewRepository(db.GORM).LocalAgentConfig(ctx, t.TempDir())
		if err != nil || again.InstallationID != first.InstallationID {
			t.Fatalf("installation id changed: %q -> %q err=%v", first.InstallationID, again.InstallationID, err)
		}
		ids = append(ids, first.InstallationID)
	}
	if ids[0] == ids[1] {
		t.Fatalf("different databases shared installation id %q", ids[0])
	}
}
