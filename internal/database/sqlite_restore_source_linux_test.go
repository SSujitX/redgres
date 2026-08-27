package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/migrations"
	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
)

func TestSQLiteRestoreSourceURIBindsPinnedInodeAcrossPathReplacement(t *testing.T) {
	pinnedSnapshot := captureNamedOwnerSnapshot(t, "pinned-owner")
	replacementSnapshot := captureNamedOwnerSnapshot(t, "replacement-owner")
	pinned, err := os.Open(pinnedSnapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	restoreSource, err := prepareSQLiteRestoreSource(context.Background(), pinnedSnapshot.Path, pinned)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreSource.close()

	savedPath := pinnedSnapshot.Path + ".saved"
	if err := os.Rename(pinnedSnapshot.Path, savedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementSnapshot.Path, pinnedSnapshot.Path); err != nil {
		t.Fatal(err)
	}
	if username := restoreOwnerFromPreparedSource(t, restoreSource); username != "pinned-owner" {
		t.Fatalf("restored owner = %q, want pinned inode", username)
	}
}

func TestSQLiteRestoreSourceSealsBytesAcrossInPlaceABA(t *testing.T) {
	pinnedSnapshot := captureNamedOwnerSnapshot(t, "pinned-owner")
	replacementSnapshot := captureNamedOwnerSnapshot(t, "other-owner")
	original, err := os.ReadFile(pinnedSnapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := os.ReadFile(replacementSnapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(original) != len(replacement) {
		t.Fatalf("fixture sizes differ: %d and %d", len(original), len(replacement))
	}
	pinned, err := os.Open(pinnedSnapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	restoreSource, err := prepareSQLiteRestoreSource(context.Background(), pinnedSnapshot.Path, pinned)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreSource.close()

	writer, err := os.OpenFile(pinnedSnapshot.Path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAt(replacement, 0); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if _, err := writer.WriteAt(original, 0); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if username := restoreOwnerFromPreparedSource(t, restoreSource); username != "pinned-owner" {
		t.Fatalf("restored owner = %q, want sealed original bytes", username)
	}
}

func TestSealAndVerifySQLiteRestoreSourceEnforcesSeals(t *testing.T) {
	file := newTestRestoreMemfd(t)
	defer file.Close()
	content := []byte("verified-restore-bytes")
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if err := sealAndVerifySQLiteRestoreSource(
		context.Background(), file, int64(len(content)), hex.EncodeToString(digest[:]),
	); err != nil {
		t.Fatal(err)
	}
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil {
		t.Fatal(err)
	}
	if seals&requiredSQLiteRestoreSeals != requiredSQLiteRestoreSeals {
		t.Fatalf("seals = %#x, want %#x", seals, requiredSQLiteRestoreSeals)
	}
	if _, err := file.WriteAt([]byte("X"), 0); err == nil {
		t.Fatal("sealed restore source accepted a write")
	}
	if err := file.Truncate(int64(len(content) + 1)); err == nil {
		t.Fatal("sealed restore source accepted growth")
	}
	if err := file.Truncate(int64(len(content) - 1)); err == nil {
		t.Fatal("sealed restore source accepted shrink")
	}
	if _, err := unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_WRITE); err == nil {
		t.Fatal("sealed restore source accepted another seal")
	}
}

func TestCopyBoundedSQLiteRestoreSourceRejectsStreamBeyondLimit(t *testing.T) {
	var destination bytes.Buffer
	_, _, err := copyBoundedSQLiteRestoreSource(
		context.Background(), &destination, strings.NewReader("123456"), 5,
	)
	if err == nil {
		t.Fatal("expected streamed source size rejection")
	}
	if destination.Len() > 5 {
		t.Fatalf("destination bytes = %d, want at most 5", destination.Len())
	}
}

func TestSealAndVerifySQLiteRestoreSourceRejectsPreSealTampering(t *testing.T) {
	file := newTestRestoreMemfd(t)
	defer file.Close()
	expected := []byte("expected-restore-bytes")
	if _, err := file.Write(expected); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(expected)
	if _, err := file.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	if err := sealAndVerifySQLiteRestoreSource(
		context.Background(), file, int64(len(expected)), hex.EncodeToString(digest[:]),
	); err == nil {
		t.Fatal("expected sealed-byte mismatch rejection")
	}
}

func newTestRestoreMemfd(t *testing.T) *os.File {
	t.Helper()
	fd, err := unix.MemfdCreate("redgres-sqlite-restore-test", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(fd), "redgres-sqlite-restore-test")
}

func restoreOwnerFromPreparedSource(t *testing.T, restoreSource *sqliteRestoreSource) string {
	t.Helper()
	destination, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	destination.SetMaxOpenConns(1)
	defer destination.Close()
	conn, err := destination.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.Raw(func(raw any) error {
		provider := raw.(interface {
			NewRestore(string) (*sqlite.Backup, error)
		})
		backup, err := provider.NewRestore(restoreSource.uri)
		if err != nil {
			return err
		}
		for {
			more, stepErr := backup.Step(snapshotStepPages)
			if stepErr != nil || !more {
				return errorsJoinFinish(stepErr, backup)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	var username string
	if err := conn.QueryRowContext(context.Background(), `SELECT username FROM owners`).Scan(&username); err != nil {
		t.Fatal(err)
	}
	return username
}

func errorsJoinFinish(stepErr error, backup *sqlite.Backup) error {
	finishErr := backup.Finish()
	if stepErr != nil {
		return stepErr
	}
	return finishErr
}

func captureNamedOwnerSnapshot(t *testing.T, username string) SQLiteSnapshot {
	t.Helper()
	source, err := Open(filepath.Join(t.TempDir(), "redgres.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := Migrate(source, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(
		`INSERT INTO owners(username, password_hash, created_at) VALUES (?, X'01', '2026-08-27T00:00:00Z')`,
		username,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureSQLiteSnapshot(context.Background(), source, secureStagingRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
