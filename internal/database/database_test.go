package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenEnablesWALAndConstraints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "redgres.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected sqlite file and parent directory: %v", err)
	}
	if db.Stats().MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d", db.Stats().MaxOpenConnections)
	}

	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journal) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}
	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	var busy int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busy)
	}

	assertRestrictedFile(t, path)
	assertRestrictedFile(t, filepath.Dir(path))
	assertRestrictedFile(t, path+"-wal")
}

func TestOpenRejectsURIInjection(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "redgres.db") + "?mode=memory")
	if err == nil {
		t.Fatal("expected path rejection")
	}
	if strings.Contains(err.Error(), "mode=memory") {
		t.Fatalf("error echoed path: %q", err)
	}
}

func assertRestrictedFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	if !unixPermsHonored(path) {
		t.Logf("skipping mode assertion on %s; filesystem does not honor Unix permission bits", path)
		return
	}
	want := os.FileMode(0o600)
	if info.IsDir() {
		want = 0o700
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func unixPermsHonored(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	original := info.Mode().Perm()
	if err := os.Chmod(path, 0o707); err != nil {
		return false
	}
	after, err := os.Stat(path)
	_ = os.Chmod(path, original)
	return err == nil && after.Mode().Perm() == 0o707
}
