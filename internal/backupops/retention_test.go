package backupops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// randomSnapshotName returns a producer-namespace operation directory name:
// "redgres-sqlite-" followed by 48 lowercase hex characters.
func randomSnapshotName(t *testing.T) string {
	t.Helper()
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		t.Fatal(err)
	}
	return "redgres-sqlite-" + hex.EncodeToString(bytes[:])
}

// createSnapshotDir creates a producer-shaped operation directory containing
// the single snapshot file and pins its ModTime.
func createSnapshotDir(t *testing.T, root, name string, mod time.Time) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "redgres-sqlite-snapshot.db"), []byte("snapshot-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRetentionRemovesOldestBeyondKeep(t *testing.T) {
	const total, keep = 9, 7
	root := t.TempDir()
	base := time.Now().Add(-48 * time.Hour)

	// names[i] is created with ModTime base + i hours; index 0 is oldest.
	names := make([]string, total)
	for i := 0; i < total; i++ {
		name := randomSnapshotName(t)
		names[i] = name
		createSnapshotDir(t, root, name, base.Add(time.Duration(i)*time.Hour))
	}
	wantRemoved := map[string]bool{}
	for i := 0; i < total-keep; i++ {
		wantRemoved[names[i]] = true
	}

	// Unrelated directory, unrelated file, and a regular file named like the
	// producer namespace must all be untouched.
	unrelatedDir := filepath.Join(root, "unrelated-dir")
	if err := os.Mkdir(unrelatedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryPath := filepath.Join(root, "keep-canary.txt")
	const canary = "keep-me"
	if err := os.WriteFile(canaryPath, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	producerNamedFile := filepath.Join(root, randomSnapshotName(t))
	if err := os.WriteFile(producerNamedFile, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := ApplyRetention(context.Background(), root, keep)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if len(removed) != total-keep {
		t.Fatalf("removed %d directories, want %d", len(removed), total-keep)
	}
	for _, name := range removed {
		if name != filepath.Base(name) {
			t.Fatalf("removed name %q is not relative", name)
		}
		if !wantRemoved[name] {
			t.Fatalf("removed %q which is not among the %d oldest snapshots", name, total-keep)
		}
		delete(wantRemoved, name)
	}
	if len(wantRemoved) != 0 {
		t.Fatalf("expected removals %v, got %v", wantRemoved, removed)
	}

	// The 7 newest survive; the newest is never removed.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	remaining := 0
	for _, entry := range entries {
		if !snapshotOperationName.MatchString(entry.Name()) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			remaining++
		}
	}
	if remaining != keep {
		t.Fatalf("remaining producer directories = %d, want %d", remaining, keep)
	}
	if _, err := os.Lstat(filepath.Join(root, names[total-1])); err != nil {
		t.Fatalf("newest snapshot directory was removed: %v", err)
	}

	// Unrelated directory, canary file, and producer-named regular file are untouched.
	if info, err := os.Lstat(unrelatedDir); err != nil || !info.IsDir() {
		t.Fatalf("unrelated directory changed: %v", err)
	}
	raw, err := os.ReadFile(canaryPath)
	if err != nil || string(raw) != canary {
		t.Fatalf("canary file changed: %q, %v", raw, err)
	}
	if info, err := os.Lstat(producerNamedFile); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("producer-named regular file changed: %v", err)
	}
}

// TestApplyRetentionIgnoresProducerNamedSymlink ensures a symlink that only
// looks like a producer operation directory is never followed or removed, and
// its target outside the staging root is untouched.
func TestApplyRetentionIgnoresProducerNamedSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-target")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	linkName := randomSnapshotName(t)
	if err := os.Symlink(outside, filepath.Join(root, linkName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// keep=1 with two real snapshots: retention removes only the oldest real
	// snapshot and must leave the symlink and its target untouched.
	base := time.Now().Add(-24 * time.Hour)
	older := randomSnapshotName(t)
	newer := randomSnapshotName(t)
	createSnapshotDir(t, root, older, base)
	createSnapshotDir(t, root, newer, base.Add(time.Hour))

	removed, err := ApplyRetention(context.Background(), root, 1)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if len(removed) != 1 || removed[0] != older {
		t.Fatalf("removed = %v, want exactly [%s]", removed, older)
	}

	linkInfo, err := os.Lstat(filepath.Join(root, linkName))
	if err != nil {
		t.Fatalf("producer-named symlink was touched: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("producer-named entry is no longer a symlink")
	}
	if info, err := os.Lstat(outside); err != nil || !info.IsDir() {
		t.Fatalf("symlink target outside the staging root changed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, newer)); err != nil {
		t.Fatalf("newest snapshot removed: %v", err)
	}
}

func TestApplyRetentionFailClosed(t *testing.T) {
	t.Run("keep zero", func(t *testing.T) {
		if _, err := ApplyRetention(context.Background(), t.TempDir(), 0); err == nil {
			t.Fatal("expected keep=0 rejection")
		}
	})
	t.Run("keep negative", func(t *testing.T) {
		if _, err := ApplyRetention(context.Background(), t.TempDir(), -3); err == nil {
			t.Fatal("expected negative keep rejection")
		}
	})
	t.Run("empty root", func(t *testing.T) {
		if _, err := ApplyRetention(context.Background(), "", 7); err == nil {
			t.Fatal("expected empty staging root rejection")
		}
	})
	t.Run("relative root", func(t *testing.T) {
		if _, err := ApplyRetention(context.Background(), "relative-staging-root", 7); err == nil {
			t.Fatal("expected relative staging root rejection")
		}
	})
	t.Run("missing root", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		if _, err := ApplyRetention(context.Background(), missing, 7); err == nil {
			t.Fatal("expected missing staging root failure")
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := ApplyRetention(ctx, t.TempDir(), 7); err == nil {
			t.Fatal("expected canceled context failure")
		}
	})
}
