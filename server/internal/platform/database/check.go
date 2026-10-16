package database

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver used below

	"github.com/EziosWJ/weighbridge/server/internal/config"
)

// CheckConnection probes a configured database without migrations or creating a
// SQLite database. Errors are for the caller to classify, never print verbatim.
func CheckConnection(ctx context.Context, cfg config.DatabaseConfig) error {
	driver := "pgx"
	var dsn string
	var err error
	if cfg.Driver == DriverSQLite {
		if _, err := os.Stat(cfg.URL); err != nil {
			return err
		}
		path, err := filepath.Abs(cfg.URL)
		if err != nil {
			return err
		}
		endpoint := url.URL{Scheme: "file", Path: path}
		endpoint.RawQuery = "mode=ro&_query_only=1&_busy_timeout=1000"
		driver, dsn = "sqlite3", endpoint.String()
	} else {
		dsn, err = buildDSN(cfg)
		if err != nil {
			return err
		}
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if cfg.Driver == DriverSQLite {
		var tables int
		return db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&tables)
	}
	return nil
}
