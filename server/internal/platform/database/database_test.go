package database

import (
	"context"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/config"
)

func TestNewDialectorUsesPostgresWithoutConnecting(t *testing.T) {
	dialector, err := newDialector(config.DatabaseConfig{
		Driver:   DriverPostgres,
		URL:      "postgres://127.0.0.1:1/unused?sslmode=disable",
		Username: "unused",
		Password: "unused",
	})
	if err != nil {
		t.Fatalf("newDialector() error = %v", err)
	}
	if got := dialector.Name(); got != DriverPostgres {
		t.Fatalf("dialector name = %q, want %q", got, DriverPostgres)
	}
}

func TestNewDialectorRejectsUnsupportedDriverAndIncompleteConfiguration(t *testing.T) {
	_, err := newDialector(config.DatabaseConfig{Driver: "sqlite"})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported driver error = %v", err)
	}

	_, err = newDialector(config.DatabaseConfig{Driver: DriverPostgres})
	if err == nil || !strings.Contains(err.Error(), "database.url") {
		t.Fatalf("empty database URL error = %v", err)
	}
}

func TestBuildDSNCombinesURLAndCredentials(t *testing.T) {
	dsn, err := buildDSN(config.DatabaseConfig{
		URL:      "postgres://127.0.0.1:5432/weighbridge?sslmode=disable",
		Username: "api-user",
		Password: "p@ss word",
	})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	if want := "postgres://api-user:p%40ss%20word@127.0.0.1:5432/weighbridge?sslmode=disable"; dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestBuildDSNRejectsCredentialsInURL(t *testing.T) {
	_, err := buildDSN(config.DatabaseConfig{
		URL:      "postgres://api-user:password@127.0.0.1:5432/weighbridge",
		Username: "api-user",
		Password: "password",
	})
	if err == nil || !strings.Contains(err.Error(), "must not include username or password") {
		t.Fatalf("buildDSN() error = %v", err)
	}
}

func TestDatabaseDialectHelpers(t *testing.T) {
	if got, err := GooseDialect(DriverPostgres); err != nil || got != "postgres" {
		t.Fatalf("GooseDialect(postgres) = (%q, %v)", got, err)
	}

	if got, err := Rebind(DriverPostgres, "UPDATE table_name SET a = ?, b = ?"); err != nil || got != "UPDATE table_name SET a = $1, b = $2" {
		t.Fatalf("Rebind(postgres) = (%q, %v)", got, err)
	}

}

func TestReadyRejectsUninitializedPoolWithoutConnecting(t *testing.T) {
	var database Database
	err := database.Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
