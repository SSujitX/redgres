package backupops

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
)

// seedControlState opens a migrated real SQLite database and inserts one row
// into every control table counted by SQLiteRestoreCheck.
func seedControlState(t *testing.T) *sql.DB {
	t.Helper()
	source, err := database.Open(filepath.Join(t.TempDir(), "redgres.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := database.Migrate(source, migrations.FS); err != nil {
		t.Fatalf("migrate source: %v", err)
	}
	if _, err := source.Exec(`
INSERT INTO owners(id, username, password_hash, created_at)
VALUES (1, 'owner', X'0102', '2026-08-27T00:00:00Z');
INSERT INTO sessions(id, owner_id, token_hash, csrf_hash, idle_expires_at, absolute_expires_at, created_at)
VALUES (1, 1, X'0304', X'0506', '2026-08-27T01:00:00Z', '2026-08-27T02:00:00Z', '2026-08-27T00:00:00Z');
INSERT INTO login_attempts(id, username, client_ip, succeeded, attempted_at)
VALUES (1, 'owner', '127.0.0.1', 1, '2026-08-27T00:00:00Z');
INSERT INTO audit_events(id, actor, action, target, outcome, request_id, client_ip, metadata, created_at)
VALUES (1, 'owner', 'test.backupops', 'sqlite', 'success', '0123456789abcdef0123456789abcdef', '127.0.0.1', '{}', '2026-08-27T00:00:00Z');
INSERT INTO operations(id, action, status, actor, accepted_request_id, target, created_at, updated_at)
VALUES ('0123456789abcdef0123456789abcdef', 'postgres.database.duplicate', 'queued', 'owner', 'fedcba9876543210fedcba9876543210', 'database/project_copy', '2026-08-27T00:00:00Z', '2026-08-27T00:00:00Z');
INSERT INTO operation_locks(resource_kind, resource_name, operation_id)
VALUES ('postgres.database', 'project_copy', '0123456789abcdef0123456789abcdef');
`); err != nil {
		t.Fatalf("seed control state: %v", err)
	}
	return source
}

func TestCaptureRunsAndVerifiesControlState(t *testing.T) {
	source := seedControlState(t)
	stagingRoot := t.TempDir()

	result, err := Capture(context.Background(), source, stagingRoot)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if len(result.Snapshot.SHA256) != 64 || result.Snapshot.SHA256 != strings.ToLower(result.Snapshot.SHA256) {
		t.Fatalf("SHA256 = %q, want 64 lowercase hex characters", result.Snapshot.SHA256)
	}
	if _, err := hex.DecodeString(result.Snapshot.SHA256); err != nil {
		t.Fatalf("SHA256 %q is not valid hex: %v", result.Snapshot.SHA256, err)
	}
	if result.Snapshot.SizeBytes <= 0 {
		t.Fatalf("SizeBytes = %d, want > 0", result.Snapshot.SizeBytes)
	}
	if !filepath.IsAbs(result.Snapshot.Path) || filepath.Dir(filepath.Dir(result.Snapshot.Path)) != filepath.Clean(stagingRoot) {
		t.Fatalf("snapshot path %q is not inside one operation directory under the staging root", result.Snapshot.Path)
	}
	operationName := filepath.Base(filepath.Dir(result.Snapshot.Path))
	if !regexp.MustCompile(`^redgres-sqlite-[0-9a-f]{48}$`).MatchString(operationName) {
		t.Fatalf("operation directory %q is not in the producer namespace", operationName)
	}
	if filepath.Base(result.Snapshot.Path) != "redgres-sqlite-snapshot.db" {
		t.Fatalf("snapshot file name %q is not module generated", filepath.Base(result.Snapshot.Path))
	}
	info, err := os.Stat(result.Snapshot.Path)
	if err != nil {
		t.Fatalf("stat captured snapshot: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("captured snapshot is not a regular file")
	}
	if result.Snapshot.SizeBytes != info.Size() {
		t.Fatalf("SizeBytes = %d, stat size = %d", result.Snapshot.SizeBytes, info.Size())
	}

	var wantSchemaVersion int
	if err := source.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&wantSchemaVersion); err != nil {
		t.Fatalf("read source schema version: %v", err)
	}
	if result.Check.SchemaVersion != wantSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d (newest embedded migration)", result.Check.SchemaVersion, wantSchemaVersion)
	}

	checks := []struct {
		name string
		got  int64
		want int64
	}{
		{"OwnerCount", result.Check.OwnerCount, 1},
		{"SessionCount", result.Check.SessionCount, 1},
		{"LoginAttemptCount", result.Check.LoginAttemptCount, 1},
		{"AuditEventCount", result.Check.AuditEventCount, 1},
		{"OperationCount", result.Check.OperationCount, 1},
		{"OperationLockCount", result.Check.OperationLockCount, 1},
	}
	for _, check := range checks {
		if check.got < 0 {
			t.Fatalf("%s = %d, want non-negative", check.name, check.got)
		}
		if check.got != check.want {
			t.Fatalf("%s = %d, want %d", check.name, check.got, check.want)
		}
	}

	// The source remains usable after capture.
	if _, err := source.Exec(`INSERT INTO owners(id, username, password_hash, created_at) VALUES (2, 'second', X'0a0b', '2026-08-27T01:00:00Z')`); err != nil {
		t.Fatalf("source unusable after capture: %v", err)
	}
}

func TestCaptureRejectsRelativeStagingRoot(t *testing.T) {
	source := seedControlState(t)
	if _, err := Capture(context.Background(), source, "relative-staging-root"); err == nil {
		t.Fatal("expected relative staging root rejection")
	}
}

func TestCaptureRejectsCanceledContext(t *testing.T) {
	source := seedControlState(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Capture(ctx, source, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// TestCaptureKeepsSnapshotWhenVerificationFails covers the contract that a
// verification failure returns the error without deleting the snapshot:
// retention/operator owns cleanup. An unmigrated source captures cleanly but
// fails the isolated restore migration check, leaving the snapshot behind.
func TestCaptureKeepsSnapshotWhenVerificationFails(t *testing.T) {
	source, err := database.Open(filepath.Join(t.TempDir(), "redgres.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if _, err := source.Exec(`CREATE TABLE bare (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed bare source: %v", err)
	}

	stagingRoot := t.TempDir()
	if _, err := Capture(context.Background(), source, stagingRoot); err == nil {
		t.Fatal("expected verification failure for unmigrated source")
	}

	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		t.Fatalf("read staging root: %v", err)
	}
	var operationDirs []string
	for _, entry := range entries {
		if snapshotOperationName.MatchString(entry.Name()) {
			operationDirs = append(operationDirs, entry.Name())
		}
	}
	if len(operationDirs) != 1 {
		t.Fatalf("surviving producer operation directories = %v, want exactly 1", operationDirs)
	}
	dirInfo, err := os.Lstat(filepath.Join(stagingRoot, operationDirs[0]))
	if err != nil {
		t.Fatalf("lstat surviving operation directory: %v", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("surviving operation directory is not a real directory")
	}
	snapshotInfo, err := os.Stat(filepath.Join(stagingRoot, operationDirs[0], "redgres-sqlite-snapshot.db"))
	if err != nil {
		t.Fatalf("snapshot file missing after verification failure: %v", err)
	}
	if !snapshotInfo.Mode().IsRegular() {
		t.Fatalf("surviving snapshot is not a regular file")
	}
}
