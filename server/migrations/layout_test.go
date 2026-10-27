package migrations_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var (
	gooseUpMarker   = regexp.MustCompile(`(?m)^-- \+goose Up\s*$`)
	gooseDownMarker = regexp.MustCompile(`(?m)^-- \+goose Down\s*$`)
	schemaDML       = regexp.MustCompile(`(?im)^\s*(INSERT|UPDATE|DELETE|MERGE)\b`)
	seedDDL         = regexp.MustCompile(`(?im)^\s*(CREATE|ALTER|DROP|TRUNCATE)\b`)
	bcryptHash      = regexp.MustCompile(`\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}`)
)

func TestMigrationStreamsHaveValidGooseFiles(t *testing.T) {
	schemaFiles := migrationFiles(t, "schema")
	if len(schemaFiles) == 0 {
		t.Fatalf("PostgreSQL schema migration stream must contain a baseline")
	}
	assertGooseFiles(t, schemaFiles)
	assertGooseFiles(t, migrationFiles(t, "seed"))
}

func TestAdminSeedPasswordMatchesJavaContract(t *testing.T) {
	contents := readFile(t, filepath.Join("seed", "00001_auth_seed.sql"))
	hash := bcryptHash.Find(contents)
	if hash == nil {
		t.Fatalf("authentication seed must contain a BCrypt password hash")
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte("admin123")); err != nil {
		t.Fatalf("administrator seed password does not match admin123: %v", err)
	}
}

func TestSchemaAndSeedResponsibilitiesStaySeparate(t *testing.T) {
	for _, name := range migrationFiles(t, "schema") {
		contents := readFile(t, name)
		// Notification Down maps BUSINESS back to MANUAL.
		// Weighing workflow Down also clears cyclic FK pointers before
		// dropping its tables. Agent contract also rebuilds CHECK constraints
		// preserving existing rows; none of these migrations inserts seed data.
		if filepath.Base(name) == "00009_business_notification_source.sql" || filepath.Base(name) == "00011_weighing_workflow_schema.sql" || filepath.Base(name) == "00012_agent_contract.sql" {
			continue
		}
		if filepath.Base(name) == "00013_sample_without_timeout.sql" {
			// Restore the former NOT NULL constraint on rollback with a data backfill.
			// Keep rejecting any other DML, including seed inserts.
			preservation := regexp.MustCompile(`(?m)^(UPDATE wb_sample SET expires_at = captured_at \+ INTERVAL '30 seconds' WHERE expires_at IS NULL;)\s*$`)
			contents = preservation.ReplaceAll(contents, nil)
		}
		if filepath.Base(name) == "00022_local_user_access.sql" {
			// Preserve existing account grants when moving to local-only access.
			continue
		}
		if filepath.Base(name) == "00021_code_identity.sql" {
			// ADR 0033 replaces numeric site/scale ids with codes. Every DML
			// statement in this migration is a preservation backfill of an
			// existing row (UPDATE ... FROM); no seed data is inserted.
			continue
		}
		if filepath.Base(name) == "00018_local_device_contract.sql" {
			// Retire historical pairing identities in place. Deleting them would
			// break references from earlier device diagnostics; the UPDATE is a
			// preservation step, not seed data.
			preservation := regexp.MustCompile(`(?m)^UPDATE wb_agent_binding SET retired_at = COALESCE\(retired_at,CURRENT_TIMESTAMP\) WHERE retired_at IS NULL;\s*$`)
			contents = preservation.ReplaceAll(contents, nil)
		}
		if schemaDML.Match(contents) {
			t.Errorf("schema migration %s contains seed-data DML", name)
		}
	}

	for _, name := range migrationFiles(t, "seed") {
		contents := readFile(t, name)
		if seedDDL.Match(contents) {
			t.Errorf("seed migration %s contains schema DDL", name)
		}
	}
}

func assertGooseFiles(t *testing.T, files []string) {
	t.Helper()
	for _, name := range files {
		contents := readFile(t, name)
		up := gooseUpMarker.FindIndex(contents)
		down := gooseDownMarker.FindIndex(contents)
		if up == nil || down == nil {
			t.Errorf("migration %s must contain exact Goose Up and Down markers", name)
			continue
		}
		if up[0] >= down[0] {
			t.Errorf("migration %s places Goose Down before Goose Up", name)
		}
	}
}

func migrationFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration directory %s: %v", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return contents
}
