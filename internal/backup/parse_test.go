package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseManifestAcceptsValidSchemaV1(t *testing.T) {
	dir := writeCatalog(t, "valid-fresh.json")

	got, err := ParseManifest(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	if got.BackupSetID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("backup_set_id = %q", got.BackupSetID)
	}
	wantCompleted := time.Date(2026, 8, 26, 4, 0, 0, 123456789, time.UTC)
	if !got.CompletedAt.Equal(wantCompleted) {
		t.Fatalf("completed_at = %s, want %s", got.CompletedAt.UTC().Format(time.RFC3339Nano), wantCompleted.Format(time.RFC3339Nano))
	}
	if got.Cluster.SystemIdentifier != "7439123456789012345" {
		t.Fatalf("system_identifier = %q", got.Cluster.SystemIdentifier)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(got.Artifacts))
	}
	if got.Artifacts[0].Kind != ArtifactKindPostgresDatabase {
		t.Fatalf("artifact kind = %q, want %q", got.Artifacts[0].Kind, ArtifactKindPostgresDatabase)
	}
	if got.Artifacts[0].Name != "appdb" {
		t.Fatalf("artifact name = %q", got.Artifacts[0].Name)
	}
	if !got.OffHost.Completed {
		t.Fatal("off_host.completed = false, want true")
	}
	if got.Restore.Outcome != RestoreOutcomeSucceeded {
		t.Fatalf("restore.outcome = %q", got.Restore.Outcome)
	}
	if got.Redgres.Version != "" || got.Redgres.CompatibilityPolicyRevision != "" {
		t.Fatalf("redgres identity = %+v", got.Redgres)
	}
}

func TestParseManifestRejectsInvalidSchemaAndPaths(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		leaks   []string
	}{
		{name: "schema_version not 1", fixture: "schema-version-2.json"},
		{name: "extra JSON field", fixture: "extra-field.json"},
		{name: "traversal dot-dot", fixture: "traversal-dotdot.json", leaks: []string{"..", "escape.dump"}},
		{name: "absolute path", fixture: "absolute-path.json", leaks: []string{"/tmp/escape.dump"}},
		{name: "percent in path", fixture: "percent-path.json", leaks: []string{"%2f", "appdb%2f.dump"}},
		{name: "question in path", fixture: "question-path.json", leaks: []string{"?mode=memory"}},
		{name: "hash in path", fixture: "hash-path.json", leaks: []string{"#frag"}},
		{name: "NUL in path", fixture: "nul-path.json", leaks: []string{"\x00"}},
		{name: "secret password key", fixture: "secret-password.json", leaks: []string{"canary-secret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeCatalog(t, tc.fixture)
			_, err := ParseManifest(dir, "manifest.json")
			if err == nil {
				t.Fatal("expected rejection")
			}
			if !errors.Is(err, errInvalidManifest) {
				t.Fatalf("err = %v, want errInvalidManifest", err)
			}
			msg := err.Error()
			for _, leak := range tc.leaks {
				if strings.Contains(msg, leak) {
					t.Fatalf("error echoed %q: %q", leak, msg)
				}
			}
		})
	}
}

func TestParseManifestRejectsUnsafeRelativePath(t *testing.T) {
	dir := writeCatalog(t, "valid-fresh.json")
	cases := []struct {
		name string
		rel  string
		leak string
	}{
		{name: "dot-dot", rel: "../manifest.json", leak: ".."},
		{name: "absolute", rel: filepath.Join(dir, "manifest.json"), leak: dir},
		{name: "percent", rel: "manifest%2f.json", leak: "%2f"},
		{name: "question", rel: "manifest.json?x", leak: "?"},
		{name: "hash", rel: "manifest.json#x", leak: "#"},
		{name: "NUL", rel: "manifest.json\x00x", leak: "\x00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest(dir, tc.rel)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if !errors.Is(err, errInvalidManifest) {
				t.Fatalf("err = %v, want errInvalidManifest", err)
			}
			if tc.leak != "" && strings.Contains(err.Error(), tc.leak) {
				t.Fatalf("error echoed %q: %q", tc.leak, err)
			}
		})
	}
}

func TestParseManifestRejectsSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	canary := filepath.Join(outside, "canary.dump")
	const payload = "outside-canary"
	if err := os.WriteFile(canary, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := writeCatalog(t, "valid-fresh.json")
	link := filepath.Join(dir, "artifacts")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := ParseManifest(dir, "manifest.json")
	if err == nil {
		t.Fatal("expected symlink escape rejection")
	}
	if !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
	raw, readErr := os.ReadFile(canary)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != payload {
		t.Fatalf("outside canary mutated: %q", raw)
	}
}

func writeCatalog(t *testing.T, fixture string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
