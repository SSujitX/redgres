package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SSujitX/redgres/migrations"
	"modernc.org/sqlite"
)

func TestCaptureSQLiteSnapshotCapturesConsistentWALState(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "redgres.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := Migrate(source, migrations.FS); err != nil {
		t.Fatalf("Migrate source: %v", err)
	}

	if _, err := source.Exec(`
PRAGMA wal_autocheckpoint = 0;
	INSERT INTO owners(id, username, password_hash, created_at)
	VALUES (1, 'owner', X'0102', '2026-08-27T00:00:00Z');
	INSERT INTO sessions(id, owner_id, token_hash, csrf_hash, idle_expires_at, absolute_expires_at, created_at)
	VALUES (1, 1, X'0304', X'0506', '2026-08-27T01:00:00Z', '2026-08-27T02:00:00Z', '2026-08-27T00:00:00Z');
	INSERT INTO audit_events(id, actor, action, target, outcome, request_id, client_ip, metadata, created_at)
	VALUES (1, 'owner', 'test.snapshot', 'sqlite', 'success', '0123456789abcdef0123456789abcdef', '127.0.0.1', '{}', '2026-08-27T00:00:00Z');
	INSERT INTO operations(id, action, status, actor, accepted_request_id, target, created_at, updated_at)
	VALUES ('0123456789abcdef0123456789abcdef', 'postgres.database.duplicate', 'queued', 'owner', 'fedcba9876543210fedcba9876543210', 'database/project_copy', '2026-08-27T00:00:00Z', '2026-08-27T00:00:00Z');
	INSERT INTO operation_locks(resource_kind, resource_name, operation_id)
	VALUES ('postgres.database', 'project_copy', '0123456789abcdef0123456789abcdef');
CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE widget_events (widget_id INTEGER NOT NULL, event TEXT NOT NULL);
CREATE INDEX widgets_name_idx ON widgets(name);
CREATE TRIGGER widgets_insert_event AFTER INSERT ON widgets
BEGIN
  INSERT INTO widget_events(widget_id, event) VALUES (NEW.id, 'insert');
END;
INSERT INTO widgets(id, name) VALUES (1, 'alpha'), (2, 'beta');
`); err != nil {
		t.Fatalf("seed WAL source: %v", err)
	}
	assertSourceFilesExist(t, sourcePath)

	stagingRoot := secureStagingRoot(t)
	snapshot, err := CaptureSQLiteSnapshot(context.Background(), source, stagingRoot)
	if err != nil {
		t.Fatalf("CaptureSQLiteSnapshot: %v", err)
	}
	operationDir := filepath.Dir(snapshot.Path)
	if !filepath.IsAbs(snapshot.Path) || filepath.Dir(operationDir) != filepath.Clean(stagingRoot) {
		t.Fatalf("snapshot path %q is not in one operation directory under staging root", snapshot.Path)
	}
	if !strings.HasPrefix(filepath.Base(operationDir), "redgres-sqlite-") {
		t.Fatalf("operation directory %q is not module generated", filepath.Base(operationDir))
	}
	if filepath.Base(snapshot.Path) != "redgres-sqlite-snapshot.db" || filepath.Ext(snapshot.Path) != ".db" {
		t.Fatalf("snapshot name %q is not module generated", filepath.Base(snapshot.Path))
	}
	assertRestrictedFile(t, snapshot.Path)

	raw, err := os.ReadFile(snapshot.Path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	info, err := os.Stat(snapshot.Path)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	wantHash := sha256.Sum256(raw)
	if snapshot.SizeBytes != info.Size() || snapshot.SizeBytes != int64(len(raw)) {
		t.Fatalf("SizeBytes = %d, stat = %d, read = %d", snapshot.SizeBytes, info.Size(), len(raw))
	}
	if snapshot.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("SHA256 = %q, want independently computed digest", snapshot.SHA256)
	}
	if snapshot.SHA256 != strings.ToLower(snapshot.SHA256) || len(snapshot.SHA256) != 64 {
		t.Fatalf("SHA256 = %q, want 64 lowercase hex characters", snapshot.SHA256)
	}

	if _, err := source.Exec(`INSERT INTO widgets(id, name) VALUES (3, 'post-capture')`); err != nil {
		t.Fatalf("source remains usable after capture: %v", err)
	}
	var sourceCount int
	if err := source.QueryRow(`SELECT COUNT(*) FROM widgets`).Scan(&sourceCount); err != nil {
		t.Fatalf("query source after capture: %v", err)
	}
	if sourceCount != 3 {
		t.Fatalf("source widget count = %d, want 3", sourceCount)
	}
	assertSourceFilesExist(t, sourcePath)

	copyDB := openSnapshotReadOnly(t, snapshot.Path)
	var widgetCount, eventCount int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM widgets`).Scan(&widgetCount); err != nil {
		t.Fatalf("query snapshot data: %v", err)
	}
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM widget_events`).Scan(&eventCount); err != nil {
		t.Fatalf("query snapshot trigger data: %v", err)
	}
	if widgetCount != 2 || eventCount != 2 {
		t.Fatalf("snapshot counts widgets/events = %d/%d, want 2/2", widgetCount, eventCount)
	}
	for table, want := range map[string]int{
		"schema_migrations": 2,
		"owners":            1,
		"sessions":          1,
		"audit_events":      1,
		"operations":        1,
		"operation_locks":   1,
	} {
		var got int
		if err := copyDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
			t.Fatalf("count snapshot table %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("snapshot table %s count = %d, want %d", table, got, want)
		}
	}
	foreignKeyRows, err := copyDB.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("check snapshot foreign keys: %v", err)
	}
	if foreignKeyRows.Next() {
		_ = foreignKeyRows.Close()
		t.Fatal("snapshot contains a foreign-key violation")
	}
	if err := foreignKeyRows.Err(); err != nil {
		_ = foreignKeyRows.Close()
		t.Fatalf("read snapshot foreign-key results: %v", err)
	}
	if err := foreignKeyRows.Close(); err != nil {
		t.Fatalf("close snapshot foreign-key results: %v", err)
	}

	for _, schemaObject := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "widgets"},
		{kind: "table", name: "widget_events"},
		{kind: "index", name: "widgets_name_idx"},
		{kind: "trigger", name: "widgets_insert_event"},
	} {
		var found string
		if err := copyDB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = ? AND name = ?`,
			schemaObject.kind,
			schemaObject.name,
		).Scan(&found); err != nil {
			t.Fatalf("snapshot schema %s %s: %v", schemaObject.kind, schemaObject.name, err)
		}
	}
	assertIntegrityOK(t, copyDB)
}

func TestCaptureSQLiteSnapshotDoesNotOverwriteExistingFiles(t *testing.T) {
	source := openMemorySource(t)
	root := secureStagingRoot(t)
	canaryPath := filepath.Join(root, "redgres-sqlite-caller-name.db")
	const canary = "caller-owned-canary"
	if err := os.WriteFile(canaryPath, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := CaptureSQLiteSnapshot(context.Background(), source, root)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CaptureSQLiteSnapshot(context.Background(), source, root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path {
		t.Fatalf("two captures reused %q", first.Path)
	}
	afterFirst, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFirst) != string(firstBytes) {
		t.Fatal("second capture changed the first snapshot")
	}
	canaryBytes, err := os.ReadFile(canaryPath)
	if err != nil || string(canaryBytes) != canary {
		t.Fatalf("caller-owned file changed: %q, %v", canaryBytes, err)
	}
}

func TestCaptureSQLiteSnapshotRejectsInvalidSource(t *testing.T) {
	tests := []struct {
		name   string
		source func(t *testing.T) *sql.DB
	}{
		{
			name: "nil",
			source: func(*testing.T) *sql.DB {
				return nil
			},
		},
		{
			name: "closed",
			source: func(t *testing.T) *sql.DB {
				db, err := sql.Open("sqlite", ":memory:")
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				return db
			},
		},
		{
			name: "unsupported driver",
			source: func(t *testing.T) *sql.DB {
				db := sql.OpenDB(unsupportedConnector{})
				t.Cleanup(func() { _ = db.Close() })
				return db
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := secureStagingRoot(t)
			if _, err := CaptureSQLiteSnapshot(context.Background(), tt.source(t), root); err == nil {
				t.Fatal("expected source rejection")
			}
			assertDirectoryEmpty(t, root)
		})
	}
}

func TestCaptureSQLiteSnapshotRejectsUnsafeStagingRoot(t *testing.T) {
	source := openMemorySource(t)
	base := t.TempDir()

	t.Run("relative", func(t *testing.T) {
		if _, err := CaptureSQLiteSnapshot(context.Background(), source, "relative-staging-root"); err == nil {
			t.Fatal("expected relative root rejection")
		}
	})

	t.Run("missing", func(t *testing.T) {
		root := filepath.Join(base, "missing")
		if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
			t.Fatal("expected missing root rejection")
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing root was created: %v", err)
		}
	})

	t.Run("non-directory", func(t *testing.T) {
		root := filepath.Join(base, "canary-file")
		const canary = "do-not-touch"
		if err := os.WriteFile(root, []byte(canary), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
			t.Fatal("expected non-directory root rejection")
		}
		raw, err := os.ReadFile(root)
		if err != nil || string(raw) != canary {
			t.Fatalf("root canary changed: %q, %v", raw, err)
		}
	})

	t.Run("reserved pathname", func(t *testing.T) {
		root := filepath.Join(base, "reserved%root")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
			t.Fatal("expected reserved pathname rejection")
		}
		assertDirectoryEmpty(t, root)
	})

	t.Run("not private", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not expose Unix directory privacy bits")
		}
		root := filepath.Join(base, "permissive")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
			t.Fatal("expected permissive root rejection")
		}
		assertDirectoryEmpty(t, root)
	})
}

func TestCaptureSQLiteSnapshotRejectsSymlinkedStagingRoot(t *testing.T) {
	source := openMemorySource(t)
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "staging")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := CaptureSQLiteSnapshot(context.Background(), source, link); err == nil {
		t.Fatal("expected symlinked root rejection")
	}
	assertDirectoryEmpty(t, target)
}

func TestCaptureSQLiteSnapshotRejectsSymlinkedRootAncestor(t *testing.T) {
	source := openMemorySource(t)
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root := filepath.Join(link, "staging")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
		t.Fatal("expected symlinked ancestor rejection")
	}
	assertDirectoryEmpty(t, root)
}

func TestCaptureSQLiteSnapshotPreCanceledPreservesCanary(t *testing.T) {
	source := openMemorySource(t)
	root := t.TempDir()
	writeCanary(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := CaptureSQLiteSnapshot(ctx, source, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	assertOnlyCanary(t, root)
}

func TestCaptureSQLiteSnapshotCancellationCleansOnlyGeneratedPartial(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "redgres.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if _, err := source.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA wal_autocheckpoint = 0;
CREATE TABLE items (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
`); err != nil {
		t.Fatalf("seed cancel source: %v", err)
	}
	tx, err := source.Begin()
	if err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("x", 4096)
	for i := 0; i < 2048; i++ {
		if _, err := tx.Exec(`INSERT INTO items(value) VALUES (?)`, value); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertSourceFilesExist(t, sourcePath)

	root := secureStagingRoot(t)
	writeCanary(t, root)
	ctx := &cancelAfterErrChecksContext{Context: context.Background(), cancelAt: 4}
	if _, err := CaptureSQLiteSnapshot(ctx, source, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := ctx.calls.Load(); got < ctx.cancelAt {
		t.Fatalf("context Err checks = %d, want at least %d", got, ctx.cancelAt)
	}
	assertOnlyCanary(t, root)
	assertSourceFilesExist(t, sourcePath)
	if _, err := source.ExecContext(context.Background(), `INSERT INTO items(value) VALUES ('still-usable')`); err != nil {
		t.Fatalf("source unusable after canceled capture: %v", err)
	}
}

func TestCaptureSQLiteSnapshotRejectsReplacementBeforeFirstStep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing the open reserved target")
	}
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	victimPath := filepath.Join(t.TempDir(), "victim.db")
	const victim = "do-not-overwrite"
	if err := os.WriteFile(victimPath, []byte(victim), 0o600); err != nil {
		t.Fatal(err)
	}
	source := sql.OpenDB(&swapBackupConnector{dsn: sourcePath, victimPath: victimPath})
	source.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = source.Close() })
	if _, err := source.Exec(`CREATE TABLE source_data (id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO source_data(value) VALUES ('copy-me')`); err != nil {
		t.Fatal(err)
	}

	root := secureStagingRoot(t)
	if _, err := CaptureSQLiteSnapshot(context.Background(), source, root); err == nil {
		t.Fatal("expected replacement to fail closed before backup copy")
	}
	raw, err := os.ReadFile(victimPath)
	if err != nil || string(raw) != victim {
		t.Fatalf("replacement victim changed: %q, %v", raw, err)
	}
}

func TestCleanupTargetPreservesReplacement(t *testing.T) {
	rootPath := secureStagingRoot(t)
	root, err := openSnapshotRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.handle.Close()
	target, err := root.reserveTarget()
	if err != nil {
		t.Fatal(err)
	}
	if err := target.file.Close(); err != nil {
		t.Fatal(err)
	}
	target.file = nil
	if err := os.Remove(target.path); err != nil {
		t.Fatal(err)
	}
	const replacement = "caller-owned-replacement"
	if err := os.WriteFile(target.path, []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := root.cleanupTarget(target); err == nil {
		t.Fatal("expected identity-safe cleanup failure")
	}
	raw, err := os.ReadFile(target.path)
	if err != nil || string(raw) != replacement {
		t.Fatalf("cleanup changed replacement: %q, %v", raw, err)
	}
}

func TestVerifyStableSQLiteSnapshotRejectsSameSizeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	_, _, err = verifyStableSQLiteSnapshot(context.Background(), path, file, func() error {
		writer, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		if _, err := writer.WriteAt([]byte("X"), 4); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	})
	if err == nil || !strings.Contains(err.Error(), "changed during integrity verification") {
		t.Fatalf("error = %v, want stable-byte-state rejection", err)
	}
}

func openMemorySource(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE source_canary (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func openSnapshotReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("open snapshot independently: %v", err)
	}
	return db
}

func assertIntegrityOK(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA integrity_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "ok" {
		t.Fatalf("integrity_check = %q, want exactly [ok]", results)
	}
}

func assertSourceFilesExist(t *testing.T, sourcePath string) {
	t.Helper()
	for _, path := range []string{sourcePath, sourcePath + "-wal", sourcePath + "-shm"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("source artifact %s missing: %v", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("source artifact %s is not regular", filepath.Base(path))
		}
	}
}

func writeCanary(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "keep-canary.txt"), []byte("keep-me"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertOnlyCanary(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "keep-canary.txt" {
		t.Fatalf("staging entries = %v, want only canary", got)
	}
	raw, err := os.ReadFile(filepath.Join(root, "keep-canary.txt"))
	if err != nil || string(raw) != "keep-me" {
		t.Fatalf("canary changed: %q, %v", raw, err)
	}
}

func assertDirectoryEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory contains unexpected entries: %v", entries)
	}
}

type unsupportedConnector struct{}

func (unsupportedConnector) Connect(context.Context) (driver.Conn, error) {
	return unsupportedConn{}, nil
}

func (unsupportedConnector) Driver() driver.Driver { return unsupportedDriver{} }

type unsupportedDriver struct{}

func (unsupportedDriver) Open(string) (driver.Conn, error) { return unsupportedConn{}, nil }

type unsupportedConn struct{}

func (unsupportedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (unsupportedConn) Close() error              { return nil }
func (unsupportedConn) Begin() (driver.Tx, error) { return nil, errors.New("begin is unsupported") }

type cancelAfterErrChecksContext struct {
	context.Context
	calls    atomic.Int32
	cancelAt int32
}

func (c *cancelAfterErrChecksContext) Err() error {
	if c.calls.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

type swapBackupConnector struct {
	dsn        string
	victimPath string
}

func (c *swapBackupConnector) Connect(context.Context) (driver.Conn, error) {
	raw, err := (&sqlite.Driver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &swapBackupConn{Conn: raw, victimPath: c.victimPath}, nil
}

func (c *swapBackupConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type swapBackupConn struct {
	driver.Conn
	victimPath string
}

func (c *swapBackupConn) NewBackup(path string) (*sqlite.Backup, error) {
	provider, ok := c.Conn.(interface {
		NewBackup(string) (*sqlite.Backup, error)
	})
	if !ok {
		return nil, fmt.Errorf("modernc connection does not support online backup")
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("replace reserved target: %w", err)
	}
	if err := os.Symlink(c.victimPath, path); err != nil {
		return nil, fmt.Errorf("replace reserved target with symlink: %w", err)
	}
	return provider.NewBackup(path)
}
