package database

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// CheckConnection probes PostgreSQL without creating databases or migrating.
// Errors are for the caller to classify, never print verbatim.
func CheckConnection(ctx context.Context, cfg config.DatabaseConfig) error {
	if cfg.Driver != DriverPostgres {
		return fmt.Errorf("database driver %q is not supported; only postgres is supported", cfg.Driver)
	}
	dsn, err := buildDSN(cfg)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}
