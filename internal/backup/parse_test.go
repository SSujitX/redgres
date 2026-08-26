package backup

import (
	"bytes"
	"encoding/json"
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

func TestParseManifestRejectsManifestLargerThanEightMiB(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "valid-fresh.json"))
	if err != nil {
		t.Fatal(err)
	}
	oversized := strings.Replace(
		string(raw),
		`"version": ""`,
		`"version": "`+strings.Repeat("x", 8*1024*1024)+`"`,
		1,
	)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(oversized), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = ParseManifest(dir, "manifest.json")
	if !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
}

func TestParseManifestRejectsInvalidUTF8AndMalformedSurrogates(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "valid-fresh.json"))
	if err != nil {
		t.Fatal(err)
	}
	replaceVersion := func(value []byte) []byte {
		replacement := append([]byte(`"version": "`), value...)
		replacement = append(replacement, '"')
		return bytes.Replace(raw, []byte(`"version": ""`), replacement, 1)
	}
	cases := []struct {
		name  string
		value []byte
	}{
		{name: "invalid raw UTF-8", value: []byte{0xff}},
		{name: "lone high surrogate", value: []byte(`\ud800`)},
		{name: "lone low surrogate", value: []byte(`\udc00`)},
		{name: "high surrogate followed by text", value: []byte(`\ud800x`)},
		{name: "high surrogate followed by non-low unit", value: []byte(`\ud800\u0041`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseManifestBytes(t.TempDir(), replaceVersion(tc.value))
			if !errors.Is(err, errInvalidManifest) {
				t.Fatalf("err = %v, want errInvalidManifest", err)
			}
		})
	}

	manifest, err := parseManifestBytes(t.TempDir(), replaceVersion([]byte(`\ud83d\ude00`)))
	if err != nil {
		t.Fatalf("valid surrogate pair rejected: %v", err)
	}
	if manifest.Redgres.Version != "😀" {
		t.Fatalf("version = %q, want emoji", manifest.Redgres.Version)
	}
}

func TestParseManifestRejectsExcessiveJSONDepthAndTokensBeforeTypedDecode(t *testing.T) {
	tooDeep := []byte(`{"artifacts":[{"kind":` + strings.Repeat("[", maxJSONDepth+1) + "0" + strings.Repeat("]", maxJSONDepth+1) + `}]}`)
	if _, err := parseManifestBytes(t.TempDir(), tooDeep); !errors.Is(err, errInvalidManifest) {
		t.Fatalf("deep err = %v, want errInvalidManifest", err)
	}

	tooManyTokens := []byte(`{"artifacts":[{"kind":[` + strings.Repeat("0,", maxJSONTokens) + `0]}]}`)
	if _, err := parseManifestBytes(t.TempDir(), tooManyTokens); !errors.Is(err, errInvalidManifest) {
		t.Fatalf("token err = %v, want errInvalidManifest", err)
	}
}

func TestParseManifestRejectsCaseInsensitiveFieldAliasesBeforeTypedDecode(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "valid-fresh.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		raw  []byte
	}{
		{
			name: "root alias",
			raw:  bytes.Replace(raw, []byte(`"artifacts"`), []byte(`"Artifacts"`), 1),
		},
		{
			name: "nested alias",
			raw:  bytes.Replace(raw, []byte(`"system_identifier"`), []byte(`"System_Identifier"`), 1),
		},
		{
			name: "mixed-case semantic duplicate",
			raw:  bytes.Replace(raw, []byte(`"backup_set_id":`), []byte(`"BACKUP_SET_ID":"00000000000000000000000000000000","backup_set_id":`), 1),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateJSONBounds(tc.raw); !errors.Is(err, errInvalidManifest) {
				t.Fatalf("validateJSONBounds err = %v, want errInvalidManifest", err)
			}
			if _, err := parseManifestBytes(t.TempDir(), tc.raw); !errors.Is(err, errInvalidManifest) {
				t.Fatalf("parseManifestBytes err = %v, want errInvalidManifest", err)
			}
		})
	}
}

func TestValidateJSONBoundsRejectsOversizedCaseAliasArtifactsArray(t *testing.T) {
	raw := []byte(`{"Artifacts":[` + strings.Repeat(`{},`, maxManifestArtifacts) + `{}` + `]}`)
	if err := validateJSONBounds(raw); !errors.Is(err, errInvalidManifest) {
		t.Fatalf("err = %v, want errInvalidManifest", err)
	}
}

func TestValidateJSONBoundsRejectsCanonicalArtifactOverflowAndExactDuplicate(t *testing.T) {
	tooManyArtifacts := []byte(`{"artifacts":[` + strings.Repeat(`{},`, maxManifestArtifacts) + `{}` + `]}`)
	if err := validateJSONBounds(tooManyArtifacts); !errors.Is(err, errInvalidManifest) {
		t.Fatalf("artifact overflow err = %v, want errInvalidManifest", err)
	}
	if err := validateJSONBounds([]byte(`{"schema_version":1,"schema_version":1}`)); !errors.Is(err, errInvalidManifest) {
		t.Fatalf("duplicate err = %v, want errInvalidManifest", err)
	}
}

func TestParseManifestRejectsExcessArtifactsAndOverlongText(t *testing.T) {
	base := parseFixture(t, "valid-fresh.json")
	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "1025 artifacts",
			mutate: func(manifest *Manifest) {
				artifact := manifest.Artifacts[0]
				manifest.Artifacts = make([]Artifact, 1025)
				for i := range manifest.Artifacts {
					manifest.Artifacts[i] = artifact
				}
			},
		},
		{
			name: "artifact kind over 64 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Artifacts[0].Kind = strings.Repeat("k", 65)
			},
		},
		{
			name: "artifact name over 512 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Artifacts[0].Name = strings.Repeat("n", 513)
			},
		},
		{
			name: "artifact path over 4096 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Artifacts[0].Path = strings.Repeat("p", 4097)
			},
		},
		{
			name: "system identifier over 20 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Cluster.SystemIdentifier = strings.Repeat("1", 21)
			},
		},
		{
			name: "restore outcome over 64 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Restore.Outcome = strings.Repeat("o", 65)
			},
		},
		{
			name: "restore set ID over 32 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Restore.BackupSetID = strings.Repeat("a", 33)
			},
		},
		{
			name: "Redgres version over 512 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Redgres.Version = strings.Repeat("v", 513)
			},
		},
		{
			name: "policy revision over 512 bytes",
			mutate: func(manifest *Manifest) {
				manifest.Redgres.CompatibilityPolicyRevision = strings.Repeat("r", 513)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			manifest.Artifacts = append([]Artifact(nil), base.Artifacts...)
			tc.mutate(&manifest)
			dir := writeManifestValue(t, manifest)
			_, err := ParseManifest(dir, "manifest.json")
			if !errors.Is(err, errInvalidManifest) {
				t.Fatalf("err = %v, want errInvalidManifest", err)
			}
		})
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

func parseFixture(t *testing.T, fixture string) Manifest {
	t.Helper()
	dir := writeCatalog(t, fixture)
	manifest, err := ParseManifest(dir, "manifest.json")
	if err != nil {
		t.Fatalf("ParseManifest(%s): %v", fixture, err)
	}
	return manifest
}

func writeManifestValue(t *testing.T, manifest Manifest) string {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
