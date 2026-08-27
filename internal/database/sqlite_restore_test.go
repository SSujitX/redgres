package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/migrations"
)

func TestVerifySQLiteSnapshotRestoreReturnsControlStateCounts(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "redgres.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := Migrate(source, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`
INSERT INTO owners(id, username, password_hash, created_at)
VALUES (1, 'owner', X'01', '2026-08-27T00:00:00Z');
INSERT INTO sessions(id, owner_id, token_hash, csrf_hash, idle_expires_at, absolute_expires_at, created_at)
VALUES (1, 1, X'02', X'03', '2026-08-27T01:00:00Z', '2026-08-27T02:00:00Z', '2026-08-27T00:00:00Z');
INSERT INTO login_attempts(id, username, client_ip, succeeded, attempted_at)
VALUES (1, 'owner', '127.0.0.1', 1, '2026-08-27T00:00:00Z');
INSERT INTO audit_events(id, actor, action, target, outcome, request_id, created_at)
VALUES (1, 'owner', 'test.restore', 'sqlite', 'success', '0123456789abcdef0123456789abcdef', '2026-08-27T00:00:00Z');
INSERT INTO operations(id, action, status, actor, accepted_request_id, target, created_at, updated_at)
VALUES ('0123456789abcdef0123456789abcdef', 'postgres.database.duplicate', 'queued', 'owner', 'fedcba9876543210fedcba9876543210', 'database/project_copy', '2026-08-27T00:00:00Z', '2026-08-27T00:00:00Z');
INSERT INTO operation_locks(resource_kind, resource_name, operation_id)
VALUES ('postgres.database', 'project_copy', '0123456789abcdef0123456789abcdef');
`); err != nil {
		t.Fatal(err)
	}

	snapshot, err := CaptureSQLiteSnapshot(context.Background(), source, secureStagingRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	check, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("VerifySQLiteSnapshotRestore: %v", err)
	}
	want := SQLiteRestoreCheck{
		SchemaVersion: 2,
		OwnerCount:    1, SessionCount: 1, LoginAttemptCount: 1,
		AuditEventCount: 1, OperationCount: 1, OperationLockCount: 1,
	}
	if check != want {
		t.Fatalf("restore check = %+v, want %+v", check, want)
	}
}

func TestVerifySQLiteSnapshotRestoreDoesNotMutateSource(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	before, err := os.ReadFile(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || beforeInfo.Size() != afterInfo.Size() || !os.SameFile(beforeInfo, afterInfo) {
		t.Fatal("restore verification changed the source snapshot")
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsWrongMetadata(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	tests := []struct {
		name   string
		mutate func(*SQLiteSnapshot)
	}{
		{name: "size", mutate: func(value *SQLiteSnapshot) { value.SizeBytes++ }},
		{name: "digest", mutate: func(value *SQLiteSnapshot) { value.SHA256 = strings.Repeat("0", 64) }},
		{name: "uppercase digest", mutate: func(value *SQLiteSnapshot) { value.SHA256 = strings.ToUpper(value.SHA256) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := snapshot
			test.mutate(&changed)
			_, err := VerifySQLiteSnapshotRestore(context.Background(), changed)
			if err == nil || err.Error() != "verify sqlite snapshot restore: snapshot metadata" {
				t.Fatalf("error = %v, want stage-only metadata error", err)
			}
		})
	}
}

func TestValidateSQLiteSnapshotMetadataBoundsSize(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"redgres-sqlite-"+strings.Repeat("a", 48),
		"redgres-sqlite-snapshot.db",
	)
	valid := SQLiteSnapshot{Path: path, SizeBytes: maxSQLiteRestoreSnapshotBytes, SHA256: strings.Repeat("0", 64)}
	if err := validateSQLiteSnapshotMetadata(valid); err != nil {
		t.Fatalf("maximum accepted size rejected: %v", err)
	}
	valid.SizeBytes++
	if err := validateSQLiteSnapshotMetadata(valid); err == nil {
		t.Fatal("expected oversized snapshot rejection")
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsActualOversizedSource(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	if err := os.Truncate(snapshot.Path, maxSQLiteRestoreSnapshotBytes+1); err != nil {
		t.Fatal(err)
	}
	_, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err == nil || err.Error() != "verify sqlite snapshot restore: prepare restore source" {
		t.Fatalf("error = %v, want bounded source rejection", err)
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsSnapshotMutation(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	file, err := os.OpenFile(snapshot.Path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{0xff}, 128); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err == nil || err.Error() != "verify sqlite snapshot restore: snapshot metadata" {
		t.Fatalf("error = %v, want changed-byte metadata rejection", err)
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsCorruptSnapshot(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	file, err := os.OpenFile(snapshot.Path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt(make([]byte, 100), 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	refreshSQLiteSnapshotMetadata(t, &snapshot)

	if _, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot); err == nil {
		t.Fatal("expected corrupt snapshot rejection")
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsForeignKeyViolation(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, func(db *sql.DB) {
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
INSERT INTO sessions(id, owner_id, token_hash, csrf_hash, idle_expires_at, absolute_expires_at, created_at)
VALUES (1, 999, X'01', X'02', '2026-08-27T01:00:00Z', '2026-08-27T02:00:00Z', '2026-08-27T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
	})

	_, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err == nil || err.Error() != "verify sqlite snapshot restore: foreign key check" {
		t.Fatalf("error = %v, want stage-only foreign-key error", err)
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsMigrationDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{
			name: "checksum mismatch",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`UPDATE schema_migrations SET checksum = 'canary-secret' WHERE version = 1`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing embedded version",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 2`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "newer version",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (99, 'future', 'canary-secret', '2026-08-27T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "excessive versions",
			mutate: func(t *testing.T, db *sql.DB) {
				for version := 3; version <= 100; version++ {
					if _, err := db.Exec(
						`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, 'future', 'canary-secret', '2026-08-27T00:00:00Z')`,
						version,
					); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := captureSQLiteRestoreFixture(t, func(db *sql.DB) { test.mutate(t, db) })
			_, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
			if err == nil || err.Error() != "verify sqlite snapshot restore: migration check" {
				t.Fatalf("error = %v, want stage-only migration error", err)
			}
			if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), snapshot.Path) {
				t.Fatalf("error leaked canary or path: %v", err)
			}
		})
	}
}

func TestVerifySQLiteSnapshotRestorePreservesCancellation(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifySQLiteSnapshotRestore(ctx, snapshot); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRunSQLiteRestoreUsesBoundedStepsAndFinishesExactlyOnce(t *testing.T) {
	tests := []struct {
		name      string
		configure func(context.Context, context.CancelFunc, *fakeSQLiteRestoreBackup) func() error
		wantError bool
	}{
		{
			name: "complete",
			configure: func(_ context.Context, _ context.CancelFunc, backup *fakeSQLiteRestoreBackup) func() error {
				backup.more = []bool{true, false}
				return func() error { return nil }
			},
		},
		{
			name: "step failure",
			configure: func(_ context.Context, _ context.CancelFunc, backup *fakeSQLiteRestoreBackup) func() error {
				backup.stepErr = errors.New("step failed")
				return func() error { return nil }
			},
			wantError: true,
		},
		{
			name: "identity failure",
			configure: func(_ context.Context, _ context.CancelFunc, _ *fakeSQLiteRestoreBackup) func() error {
				return func() error { return errors.New("identity changed") }
			},
			wantError: true,
		},
		{
			name: "cancellation",
			configure: func(_ context.Context, cancel context.CancelFunc, backup *fakeSQLiteRestoreBackup) func() error {
				backup.more = []bool{true}
				backup.afterStep = cancel
				return func() error { return nil }
			},
			wantError: true,
		},
		{
			name: "finish failure",
			configure: func(_ context.Context, _ context.CancelFunc, backup *fakeSQLiteRestoreBackup) func() error {
				backup.finishErr = errors.New("finish failed")
				return func() error { return nil }
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backup := &fakeSQLiteRestoreBackup{}
			verify := test.configure(ctx, cancel, backup)
			err := runSQLiteRestore(ctx, func() (sqliteRestoreBackup, error) { return backup, nil }, verify)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
			if backup.finishCalls != 1 {
				t.Fatalf("Finish calls = %d, want exactly 1", backup.finishCalls)
			}
			for _, pages := range backup.stepPages {
				if pages != snapshotStepPages {
					t.Fatalf("Step pages = %d, want bounded %d", pages, snapshotStepPages)
				}
			}
		})
	}
}

func TestVerifySQLiteSnapshotRestoreRejectsNonProducerNamespace(t *testing.T) {
	snapshot := captureSQLiteRestoreFixture(t, nil)
	operationPath := filepath.Dir(snapshot.Path)
	nonProducerPath := filepath.Join(filepath.Dir(operationPath), "caller-controlled-canary")
	if err := os.Rename(operationPath, nonProducerPath); err != nil {
		t.Fatal(err)
	}
	snapshot.Path = filepath.Join(nonProducerPath, filepath.Base(snapshot.Path))

	_, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err == nil || err.Error() != "verify sqlite snapshot restore: snapshot metadata" {
		t.Fatalf("error = %v, want producer namespace rejection", err)
	}
	if strings.Contains(err.Error(), "caller-controlled-canary") {
		t.Fatalf("error leaked path canary: %v", err)
	}
}

func TestVerifySQLiteSnapshotRestoreMissingSourceIsNotCreated(t *testing.T) {
	root := filepath.Join(t.TempDir(), "path-canary-secret")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := captureSQLiteRestoreFixtureAt(t, root, nil)
	if err := os.Remove(snapshot.Path); err != nil {
		t.Fatal(err)
	}

	_, err := VerifySQLiteSnapshotRestore(context.Background(), snapshot)
	if err == nil || err.Error() != "verify sqlite snapshot restore: pin snapshot" {
		t.Fatalf("error = %v, want stage-only pin error", err)
	}
	if strings.Contains(err.Error(), "path-canary-secret") {
		t.Fatalf("error leaked path canary: %v", err)
	}
	if _, statErr := os.Lstat(snapshot.Path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing source was created: %v", statErr)
	}
}

type fakeSQLiteRestoreBackup struct {
	more        []bool
	stepErr     error
	finishErr   error
	afterStep   func()
	stepPages   []int32
	finishCalls int
}

func (b *fakeSQLiteRestoreBackup) Step(pages int32) (bool, error) {
	b.stepPages = append(b.stepPages, pages)
	if b.afterStep != nil {
		b.afterStep()
		b.afterStep = nil
	}
	if b.stepErr != nil {
		return false, b.stepErr
	}
	if len(b.more) == 0 {
		return false, nil
	}
	more := b.more[0]
	b.more = b.more[1:]
	return more, nil
}

func (b *fakeSQLiteRestoreBackup) Finish() error {
	b.finishCalls++
	return b.finishErr
}

func captureSQLiteRestoreFixture(t *testing.T, mutate func(*sql.DB)) SQLiteSnapshot {
	t.Helper()
	return captureSQLiteRestoreFixtureAt(t, secureStagingRoot(t), mutate)
}

func captureSQLiteRestoreFixtureAt(t *testing.T, stagingRoot string, mutate func(*sql.DB)) SQLiteSnapshot {
	t.Helper()
	source, err := Open(filepath.Join(t.TempDir(), "redgres.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := Migrate(source, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(source)
	}
	snapshot, err := CaptureSQLiteSnapshot(context.Background(), source, stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func refreshSQLiteSnapshotMetadata(t *testing.T, snapshot *SQLiteSnapshot) {
	t.Helper()
	raw, err := os.ReadFile(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	snapshot.SizeBytes = int64(len(raw))
	snapshot.SHA256 = hex.EncodeToString(digest[:])
}
