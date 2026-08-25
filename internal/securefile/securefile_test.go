package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRegularRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	file, err := OpenRegular(link, os.O_RDONLY, 0)
	if file != nil {
		_ = file.Close()
	}
	if !errors.Is(err, ErrNotRegular) {
		t.Fatalf("err = %v, want ErrNotRegular", err)
	}
}

func TestOpenRegularRejectsTrunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	const canary = "do-not-truncate"
	if err := os.WriteFile(path, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := OpenRegular(path, os.O_RDWR|os.O_TRUNC, 0)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("expected O_TRUNC rejection")
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != canary {
		t.Fatalf("file truncated: %q", raw)
	}
}

func TestOpenRegularRejectsIntermediateDirectorySymlink(t *testing.T) {
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

	file, err := OpenRegular(filepath.Join(link, "sub", "state"), os.O_CREATE|os.O_RDWR, 0o600)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("expected intermediate directory symlink rejection")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was touched: %v", entries)
	}
}

func TestOpenRegularUnderRejectsEscape(t *testing.T) {
	jail := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(jail, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	file, err := OpenRegularUnder(jail, filepath.Join(link, "sub", "state"), os.O_CREATE|os.O_RDWR, 0o600)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("expected jail escape rejection")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was touched: %v", entries)
	}
}

func TestEnsureRealDirRejectsIntermediateSymlink(t *testing.T) {
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
	if err := EnsureRealDir(filepath.Join(link, "sub"), 0o700); err == nil {
		t.Fatal("expected intermediate directory symlink rejection")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was touched: %v", entries)
	}
}

func TestVerifyRegularPathRejectsReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := OpenRegular(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := os.Rename(path, filepath.Join(dir, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRegularPath(path, file); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("err = %v, want ErrNotRegular", err)
	}
}
