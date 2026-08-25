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
	dir := t.TempDir()
	for _, name := range []string{
		"canary-secret.db?mode=memory",
		"canary-secret.db#frag",
		"canary-secret%2f.db",
		"canary-secret%2F.db",
		"canary-secret%2e%2e.db",
		"canary-secret%25.db",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Open(filepath.Join(dir, name))
			if err == nil {
				t.Fatal("expected path rejection")
			}
			msg := err.Error()
			if strings.Contains(msg, "canary-secret") || strings.Contains(msg, "%2") || strings.Contains(msg, "mode=memory") || strings.Contains(msg, "#frag") {
				t.Fatalf("error echoed path: %q", err)
			}
		})
	}
}

func TestOpenRejectsSymlinkWithoutTouchingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "canary")
	const canary = "sqlite-target-canary"
	if err := os.WriteFile(target, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "redgres.db")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	db, err := Open(path)
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != canary {
		t.Fatalf("symlink target changed: %q", raw)
	}
}

func TestOpenRejectsNonRegularStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.db")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("expected non-regular file rejection")
	}
}

func TestOpenRejectsSymlinkedStateDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "canary-directory")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "state")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	db, err := Open(filepath.Join(link, "redgres.db"))
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("expected symlinked directory rejection")
	}
	if _, statErr := os.Stat(filepath.Join(target, "redgres.db")); !os.IsNotExist(statErr) {
		t.Fatalf("symlink target was touched: %v", statErr)
	}
}

func TestOpenRejectsIntermediateDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	jail := filepath.Join(root, "jail")
	if err := os.Mkdir(jail, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(jail, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	db, err := Open(filepath.Join(link, "sub", "redgres.db"))
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("expected intermediate directory symlink rejection")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was touched: %v", names(entries))
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestOpenRejectsSymlinkedWALWithoutTouchingTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redgres.db")
	target := filepath.Join(dir, "wal-canary")
	const canary = "wal-target-canary"
	if err := os.WriteFile(target, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path+"-wal"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	db, err := Open(path)
	if db != nil {
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("expected symlinked WAL rejection")
	}
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != canary {
		t.Fatalf("WAL symlink target changed: %q", raw)
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
