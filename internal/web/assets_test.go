package web

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestOpenWithoutDirectoryDelegatesToEmbeddedAssets asserts the contract of
// Open("") without requiring a frontend build, so it holds on a clean checkout.
// It deliberately does not stat the embed root: Assets() cannot do that when only
// dist/.gitkeep is embedded, which is a separate pre-existing defect.
func TestOpenWithoutDirectoryDelegatesToEmbeddedAssets(t *testing.T) {
	assets, release, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer release()
	embedded, err := Assets()
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	if readable(assets, "index.html") != readable(embedded, "index.html") {
		t.Fatal("Open(\"\") did not delegate to the embedded assets")
	}
}

func readable(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func TestOpenServesDevelopmentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	assets, release, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer release()
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "<!doctype html>" {
		t.Fatalf("index.html = %q", string(data))
	}
}

func TestOpenIgnoresEnvironment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("environment"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	t.Setenv("REDGRES_DEV_ASSET_DIR", dir)

	// Positive control: the marker is reachable when the directory is passed as
	// the argument, so the negative assertion below cannot pass vacuously.
	viaArgument, releaseArgument, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(dir): %v", err)
	}
	defer releaseArgument()
	if data, err := fs.ReadFile(viaArgument, "index.html"); err != nil || string(data) != "environment" {
		t.Fatalf("positive control failed: %q %v", string(data), err)
	}

	assets, release, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer release()
	if data, err := fs.ReadFile(assets, "index.html"); err == nil && string(data) == "environment" {
		t.Fatal("Open read REDGRES_DEV_ASSET_DIR instead of its argument")
	}
}

func TestOpenRejectsMissingDirectory(t *testing.T) {
	if _, _, err := Open(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected Open to reject a missing directory")
	}
}

// TestOpenDoesNotFollowSymlinkOutOfRoot proves the asset root contains link
// targets, not only file names. os.DirFS would serve the outside file.
func TestOpenDoesNotFollowSymlinkOutOfRoot(t *testing.T) {
	parent, root := assetRootWithOutsideCanary(t)
	link := filepath.Join(root, "assets", "escape.js")
	if err := os.Symlink(filepath.Join(parent, "outside.txt"), link); err != nil {
		t.Skipf("symlinks unavailable on this host: %v", err)
	}

	assertContained(t, root, "assets/escape.js")
}

// TestOpenDoesNotFollowJunctionOutOfRoot covers the Windows case that needs no
// symlink privilege, so it runs where TestOpenDoesNotFollowSymlinkOutOfRoot
// must skip.
func TestOpenDoesNotFollowJunctionOutOfRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("directory junctions are Windows-only")
	}
	parent, root := assetRootWithOutsideCanary(t)
	link := filepath.Join(root, "assets", "escape")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, parent).CombinedOutput()
	if err != nil {
		t.Skipf("mklink unavailable: %v: %s", err, out)
	}

	assertContained(t, root, "assets/escape/outside.txt")
}

func assetRootWithOutsideCanary(t *testing.T) (parent, root string) {
	t.Helper()
	parent = t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside-canary"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	root = filepath.Join(parent, "app")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatalf("create root: %v", err)
	}
	return parent, root
}

func assertContained(t *testing.T, root, name string) {
	t.Helper()
	assets, release, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer release()

	data, err := fs.ReadFile(assets, name)
	if err == nil {
		t.Fatalf("%s escaped the asset root: %q", name, string(data))
	}
}
