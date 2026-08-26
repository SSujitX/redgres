package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCurrentReadsJailRootCurrentJSON(t *testing.T) {
	dir := writeNamedCatalog(t, "valid-fresh.json", currentManifestName)
	got, err := LoadCurrent(dir)
	if err != nil {
		t.Fatalf("LoadCurrent: %v", err)
	}
	if got.Cluster.SystemIdentifier != "7439123456789012345" {
		t.Fatalf("system_identifier = %q", got.Cluster.SystemIdentifier)
	}
	if got.Artifacts[0].Name != "appdb" {
		t.Fatalf("artifact name = %q", got.Artifacts[0].Name)
	}
}

func TestLoadCurrentIgnoresSiblingManifestJSON(t *testing.T) {
	dir := writeNamedCatalog(t, "valid-fresh.json", "manifest.json")
	_, err := LoadCurrent(dir)
	if err == nil {
		t.Fatal("LoadCurrent must not read sibling manifest.json")
	}
	if !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
	if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("error echoed path: %v", err)
	}
}

func TestLoadCurrentMissingFileIsInvalidWithoutPath(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCurrent(dir)
	if err == nil {
		t.Fatal("expected missing current.json to fail")
	}
	if !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
	if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), currentManifestName) {
		t.Fatalf("error echoed path: %v", err)
	}
}

func TestLoadCurrentEmptyDirFailsClosed(t *testing.T) {
	_, err := LoadCurrent("")
	if err == nil {
		t.Fatal("empty catalog dir must fail closed")
	}
	if !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
}

func writeNamedCatalog(t *testing.T, fixture, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
