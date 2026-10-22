//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
)

type temporaryPostgres struct {
	container string
	name      string
	dsn       string
}

// Resources belong to the test process, not the first test that requests them.
// Keep tests serial: existing Goose round-trip tests mutate package global state.
var integrationSuite struct {
	postgresOnce  sync.Once
	postgres      temporaryPostgres
	postgresErr   error
	databaseID    atomic.Uint64
	migrationOnce sync.Once
	migrationDir  string
	migrationPath string
	migrationErr  error
	templateOnce  sync.Once
	templateErr   error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if integrationSuite.postgres.container != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		output, err := exec.CommandContext(ctx, "docker", "rm", "--force", integrationSuite.postgres.container).CombinedOutput()
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "clean up temporary PostgreSQL: %v\n%s", err, output)
			code = 1
		}
	}
	if integrationSuite.migrationDir != "" {
		if err := os.RemoveAll(integrationSuite.migrationDir); err != nil {
			fmt.Fprintf(os.Stderr, "clean up migration executable: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

// startPostgres creates an empty database for migration and restore contracts.
func startPostgres(t *testing.T) temporaryPostgres {
	t.Helper()
	ensureSuitePostgres(t)
	return createTestDatabase(t, "template0")
}

// startBusinessPostgres clones the suite's migrated, connection-disabled template.
// The template lives until TestMain removes the container, never a test cleanup.
func startBusinessPostgres(t *testing.T) temporaryPostgres {
	t.Helper()
	ensureSuitePostgres(t)
	template := namedPostgres(t, "integration_template")
	root := projectRoot(t)
	executable := migrationExecutable(t, root)
	environment := integrationEnvironment(t, template.dsn, config.EnvironmentTest)
	integrationSuite.templateOnce.Do(func() {
		if err := executeAdminSQL("CREATE DATABASE " + template.name + " TEMPLATE template0"); err != nil {
			integrationSuite.templateErr = fmt.Errorf("create business template: %w", err)
			return
		}
		if err := executeMigrations(root, executable, environment); err != nil {
			integrationSuite.templateErr = err
			return
		}
		// The migration subprocess has closed its pool; prevent accidental clients
		// from connecting to the template and blocking subsequent database clones.
		integrationSuite.templateErr = executeAdminSQL("ALTER DATABASE " + template.name + " ALLOW_CONNECTIONS false")
	})
	if err := integrationSuite.templateErr; err != nil {
		t.Fatalf("prepare migrated business template: %v", err)
	}
	return createTestDatabase(t, template.name)
}

func ensureSuitePostgres(t *testing.T) {
	t.Helper()
	integrationSuite.postgresOnce.Do(func() {
		integrationSuite.postgresErr = startSuitePostgres()
	})
	if err := integrationSuite.postgresErr; err != nil {
		t.Fatalf("prepare shared temporary PostgreSQL: %v", err)
	}
}

func namedPostgres(t *testing.T, name string) temporaryPostgres {
	t.Helper()
	database := integrationSuite.postgres
	database.name = name
	parsed, err := url.Parse(database.dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	database.dsn = parsed.String()
	return database
}

func createTestDatabase(t *testing.T, template string) temporaryPostgres {
	t.Helper()
	database := namedPostgres(t, fmt.Sprintf("integration_%d", integrationSuite.databaseID.Add(1)))
	// Names are generated internally from constants and the monotonic counter.
	if err := executeAdminSQL("CREATE DATABASE " + database.name + " TEMPLATE " + template); err != nil {
		t.Fatalf("create isolated database from %s: %v", template, err)
	}
	t.Cleanup(func() {
		// FORCE also closes connections left behind by a failed test.
		if err := executeAdminSQL("DROP DATABASE " + database.name + " WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated database %s: %v", database.name, err)
		}
	})
	return database
}

func startSuitePostgres() error {
	container := fmt.Sprintf("server-integration-%d-%d", os.Getpid(), time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm", "--name", container,
		"--env", "POSTGRES_DB=integration", "--env", "POSTGRES_USER=integration",
		"--env", "POSTGRES_PASSWORD=integration-password", "--publish", "127.0.0.1::5432", postgresImage)
	// Register before starting so setup failures also pass through process cleanup.
	integrationSuite.postgres.container = container
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("start container: %w\n%s", err, output)
	}
	output, err := exec.CommandContext(ctx, "docker", "port", container, "5432/tcp").Output()
	if err != nil {
		return fmt.Errorf("resolve port: %w", err)
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(output)))
	if err != nil {
		return fmt.Errorf("parse port: %w", err)
	}
	integrationSuite.postgres.dsn = "postgres://integration:integration-password@127.0.0.1:" + port + "/integration?sslmode=disable"
	connection, err := sql.Open("pgx", integrationSuite.postgres.dsn)
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline := time.Now().Add(45 * time.Second)
	readyStreak := 0
	for time.Now().Before(deadline) {
		pingCtx, pingCancel := context.WithTimeout(ctx, time.Second)
		err = connection.PingContext(pingCtx)
		pingCancel()
		if err == nil {
			readyStreak++
			if readyStreak >= 2 {
				return nil
			}
		} else {
			readyStreak = 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("authenticated SQL connections not ready within 45 seconds: %v", err)
}

func executeAdminSQL(statement string) error {
	connection, err := sql.Open("pgx", integrationSuite.postgres.dsn)
	if err != nil {
		return err
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = connection.ExecContext(ctx, statement)
	return err
}

func migrationExecutable(t *testing.T, root string) string {
	t.Helper()
	integrationSuite.migrationOnce.Do(func() {
		dir, err := os.MkdirTemp("", "weighbridge-integration-migrate-")
		if err != nil {
			integrationSuite.migrationErr = err
			return
		}
		integrationSuite.migrationDir = dir
		name := "migrate"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		integrationSuite.migrationPath = filepath.Join(dir, name)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		// This disposable test binary needs no VCS stamp (including in linked worktrees).
		command := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", integrationSuite.migrationPath, "./cmd/migrate")
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			integrationSuite.migrationErr = fmt.Errorf("build migration executable: %w\n%s", err, output)
		}
	})
	if err := integrationSuite.migrationErr; err != nil {
		t.Fatal(err)
	}
	return integrationSuite.migrationPath
}

func TestPostgresSharedContainerDatabaseIsolation(t *testing.T) {
	first := startPostgres(t)
	firstDB := openTemporaryDatabase(t, first.dsn)
	defer firstDB.Close()
	var migratedTable sql.NullString
	if err := firstDB.SQL.QueryRow("SELECT to_regclass('public.sys_user')::text").Scan(&migratedTable); err != nil || migratedTable.Valid {
		t.Fatalf("migration fixture must be empty: %v, %v", migratedTable, err)
	}

	if err := firstDB.GORM.Exec("CREATE TABLE isolation_probe (value text); INSERT INTO isolation_probe VALUES ('preserved')").Error; err != nil {
		t.Fatal(err)
	}
	var second temporaryPostgres
	parent := t
	t.Run("independent_database", func(t *testing.T) {
		second = startPostgres(t)
		if first.container != second.container || first.name == second.name || first.dsn == second.dsn {
			t.Fatal("expected one container with distinct database names and DSNs")
		}
		secondDB := openTemporaryDatabase(t, second.dsn)
		// Parent cleanup runs after the subtest has dropped the database, exercising DROP FORCE.
		parent.Cleanup(func() { _ = secondDB.Close() })
		var table sql.NullString
		if err := secondDB.SQL.QueryRow("SELECT to_regclass('public.isolation_probe')::text").Scan(&table); err != nil || table.Valid {
			t.Fatalf("database leaked another test's table: %v, %v", table, err)
		}
		if err := secondDB.GORM.Exec("CREATE TABLE isolation_probe (value text)").Error; err != nil {
			t.Fatal(err)
		}
	})
	var exists bool
	if err := firstDB.SQL.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", second.name).Scan(&exists); err != nil || exists {
		t.Fatalf("subtest database was not dropped: exists=%v, err=%v", exists, err)
	}
	var value string
	if err := firstDB.SQL.QueryRow("SELECT value FROM isolation_probe").Scan(&value); err != nil || value != "preserved" {
		t.Fatalf("other database changed during cleanup: value=%q, err=%v", value, err)
	}
}

func TestPostgresBusinessTemplateIsolation(t *testing.T) {
	var first temporaryPostgres
	var nextUserID, schemaVersion, seedVersion int64
	t.Run("modify_first_clone", func(t *testing.T) {
		first = startBusinessPostgres(t)
		db := openTemporaryDatabase(t, first.dsn)
		defer db.Close()
		var username string
		if err := db.SQL.QueryRow("SELECT username FROM sys_user WHERE id=1").Scan(&username); err != nil || username != "admin" {
			t.Fatalf("business clone has no migrated seed user: %q, %v", username, err)
		}
		if err := db.SQL.QueryRow("SELECT MAX(version_id) FROM goose_schema_db_version WHERE is_applied").Scan(&schemaVersion); err != nil {
			t.Fatal(err)
		}
		if err := db.SQL.QueryRow("SELECT MAX(version_id) FROM goose_seed_db_version WHERE is_applied").Scan(&seedVersion); err != nil {
			t.Fatal(err)
		}
		if err := db.SQL.QueryRow("SELECT nextval(pg_get_serial_sequence('sys_user','id'))").Scan(&nextUserID); err != nil {
			t.Fatal(err)
		}
		if err := db.GORM.Exec("UPDATE sys_user SET nickname='修改后的用户' WHERE id=1").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GORM.Exec("CREATE TABLE business_clone_probe (value text)").Error; err != nil {
			t.Fatal(err)
		}
		// Change a clone's schema as well as its data and sequences.
		if err := db.GORM.Exec("ALTER TABLE sys_user ADD COLUMN business_clone_probe text").Error; err != nil {
			t.Fatal(err)
		}
	})
	if t.Failed() {
		return
	}
	// The first clone has been dropped. The suite template must still be usable.
	t.Run("second_clone_is_pristine", func(t *testing.T) {
		second := startBusinessPostgres(t)
		if second.container != first.container || second.name == first.name {
			t.Fatal("expected independent databases in the shared container")
		}
		db := openTemporaryDatabase(t, second.dsn)
		defer db.Close()
		var nickname, logClear string
		if err := db.SQL.QueryRow("SELECT nickname FROM sys_user WHERE id=1").Scan(&nickname); err != nil || nickname == "修改后的用户" {
			t.Fatalf("clone leaked another test's data: %q, %v", nickname, err)
		}
		if err := db.SQL.QueryRow("SELECT config_value FROM sys_config WHERE config_key=$1", sysconfig.LogClearEnabledKey).Scan(&logClear); err != nil || logClear != "false" {
			t.Fatalf("clone did not use the test environment seed: %q, %v", logClear, err)
		}
		var table sql.NullString
		if err := db.SQL.QueryRow("SELECT to_regclass('public.business_clone_probe')::text").Scan(&table); err != nil || table.Valid {
			t.Fatalf("clone leaked another test's table: %v, %v", table, err)
		}
		var columns int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_name='sys_user' AND column_name='business_clone_probe'").Scan(&columns); err != nil || columns != 0 {
			t.Fatalf("clone leaked another test's schema: %d, %v", columns, err)
		}
		var gotNext, gotSchema, gotSeed int64
		if err := db.SQL.QueryRow("SELECT nextval(pg_get_serial_sequence('sys_user','id'))").Scan(&gotNext); err != nil {
			t.Fatal(err)
		}
		if err := db.SQL.QueryRow("SELECT MAX(version_id) FROM goose_schema_db_version WHERE is_applied").Scan(&gotSchema); err != nil {
			t.Fatal(err)
		}
		if err := db.SQL.QueryRow("SELECT MAX(version_id) FROM goose_seed_db_version WHERE is_applied").Scan(&gotSeed); err != nil {
			t.Fatal(err)
		}
		if gotNext != nextUserID || gotSchema != schemaVersion || gotSeed != seedVersion {
			t.Fatalf("clone baseline changed: next=%d schema=%d seed=%d", gotNext, gotSchema, gotSeed)
		}
	})
}
