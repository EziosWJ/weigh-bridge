// Package database owns long-lived database infrastructure.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/EziosWJ/weighbridge/server/internal/config"
)

const DriverPostgres = "postgres"

// Database contains the single process-wide GORM handle and the database/sql
// connection pool underneath it. Neither API startup nor this package executes
// schema migrations or GORM AutoMigrate.
type Database struct {
	GORM *gorm.DB
	SQL  *sql.DB
}

// Open creates the selected GORM handle, configures its underlying pool, and
// verifies the connection before returning it. The caller remains responsible
// for running explicit schema and seed migrations before starting the API.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*Database, error) {
	dialector, err := newDialector(cfg)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open GORM database: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get database/sql pool: %w", err)
	}
	configurePool(sqlDB, cfg)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{GORM: gormDB, SQL: sqlDB}, nil
}

// Ready satisfies the HTTP readiness-check contract without exposing a driver
// concern to Handlers or Services.
func (d *Database) Ready(ctx context.Context) error {
	if d == nil || d.SQL == nil {
		return errors.New("database pool is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := d.SQL.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (d *Database) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

func newDialector(cfg config.DatabaseConfig) (gorm.Dialector, error) {
	switch cfg.Driver {
	case DriverPostgres:
		dsn, err := buildDSN(cfg)
		if err != nil {
			return nil, err
		}
		return postgres.Open(dsn), nil
	default:
		return nil, fmt.Errorf("database driver %q is not supported; only postgres is supported", cfg.Driver)
	}
}

func buildDSN(cfg config.DatabaseConfig) (string, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return "", errors.New("database.url is required")
	}

	endpoint, err := url.Parse(cfg.URL)
	if err != nil {
		return "", fmt.Errorf("parse database.url: %w", err)
	}
	if endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql" {
		return "", errors.New("database.url must use postgres or postgresql scheme")
	}
	if endpoint.Host == "" {
		return "", errors.New("database.url must include host and port")
	}
	if strings.Trim(endpoint.Path, "/") == "" {
		return "", errors.New("database.url must include database name")
	}
	if endpoint.User != nil {
		return "", errors.New("database.url must not include username or password")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return "", errors.New("database.username is required")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return "", errors.New("database.password is required")
	}

	endpoint.User = url.UserPassword(cfg.Username, cfg.Password)
	return endpoint.String(), nil
}

func configurePool(sqlDB *sql.DB, cfg config.DatabaseConfig) {
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
}

// GooseDialect returns the migration dialect name for a configured driver.
// The mapping stays in the database infrastructure so migration commands do
// not need to duplicate driver-specific knowledge.
func GooseDialect(driver string) (string, error) {
	switch driver {
	case DriverPostgres:
		return "postgres", nil
	default:
		return "", fmt.Errorf("database driver %q is not supported; only postgres is supported", driver)
	}
}

// Rebind changes the portable '?' placeholders used by migration-side SQL to
// the PostgreSQL placeholders expected by pgx.
func Rebind(driver, query string) (string, error) {
	if driver != DriverPostgres {
		return "", fmt.Errorf("database driver %q is not supported", driver)
	}
	var builder strings.Builder
	parameter := 0
	for _, character := range query {
		if character == '?' {
			parameter++
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(parameter))
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String(), nil
}
