package database

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/SSujitX/redgres/migrations"
)

func openMigrated(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(db, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return path
}

func TestMigrateCreatesControlStateSchema(t *testing.T) {
	path := openMigrated(t)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	required := []string{"schema_migrations", "owners", "sessions", "login_attempts", "audit_events", "operations", "operation_locks"}
	for _, name := range required {
		var found string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
		if err != nil || found != name {
			t.Fatalf("missing table %s: %v", name, err)
		}
	}
	indexes := []string{
		"sessions_owner_id_idx",
		"sessions_idle_expires_at_idx",
		"login_attempts_lookup_idx",
		"audit_events_created_at_idx",
		"audit_events_request_id_idx",
		"operations_status_idx",
		"operations_created_at_idx",
		"operation_locks_operation_id_idx",
	}
	for _, name := range indexes {
		var found string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&found)
		if err != nil || found != name {
			t.Fatalf("missing index %s: %v", name, err)
		}
	}
	var version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("schema_migrations count = %d", version)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}

	var firstCount int
	var firstApplied string
	if err := db.QueryRow(`SELECT COUNT(*), applied_at FROM schema_migrations WHERE version = 1`).Scan(&firstCount, &firstApplied); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(db, migrations.FS); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var secondCount int
	var secondApplied string
	if err := db.QueryRow(`SELECT COUNT(*), applied_at FROM schema_migrations WHERE version = 1`).Scan(&secondCount, &secondApplied); err != nil {
		t.Fatal(err)
	}
	if firstCount != secondCount || firstApplied != secondApplied {
		t.Fatalf("migration was reapplied: count %d/%d applied %q/%q", firstCount, secondCount, firstApplied, secondApplied)
	}
}

func TestMigrateChecksumMismatchFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum = '00' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}

	err = Migrate(db, migrations.FS)
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "version 1") {
		t.Fatalf("error %q should name the version", err)
	}
	var checksum string
	if err := db.QueryRow(`SELECT checksum FROM schema_migrations WHERE version = 1`).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if checksum != "00" {
		t.Fatalf("checksum was rewritten to %q", checksum)
	}
}

func TestMigrateNewerSchemaFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (99, 'future', 'abc', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	err = Migrate(db, migrations.FS)
	if err == nil {
		t.Fatal("expected downgrade error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error %q should name version 99", err)
	}
}

func TestMigrateRollsBackFailedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sources := fstest.MapFS{
		"001_broken.sql": {Data: []byte("CREATE TABLE ephemeral (id INTEGER PRIMARY KEY);\nCREATE TABLE ephemeral (id INTEGER PRIMARY KEY);\n")},
	}
	err = Migrate(db, sources)
	if err == nil {
		t.Fatal("expected migration failure")
	}

	var name string
	scanErr := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'ephemeral'`).Scan(&name)
	if scanErr == nil {
		t.Fatal("rolled-back table still exists")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("schema_migrations count = %d", count)
	}
}

func TestMigrateRejectsInvalidFilenames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	err = Migrate(db, fstest.MapFS{"notes.sql": {Data: []byte("SELECT 1;")}})
	if err == nil || !strings.Contains(err.Error(), "NNN_name.sql") {
		t.Fatalf("filename error = %v", err)
	}

	err = Migrate(db, fstest.MapFS{
		"001_one.sql": {Data: []byte("CREATE TABLE a (id INTEGER PRIMARY KEY);")},
		"001_two.sql": {Data: []byte("CREATE TABLE b (id INTEGER PRIMARY KEY);")},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestEmbeddedMigrationsAreValidFS(t *testing.T) {
	if err := fs.WalkDir(migrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
