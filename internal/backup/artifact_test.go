package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyPostgresDatabaseArtifactSuccess(t *testing.T) {
	dir, manifest := writeArtifactFixture(t, "appdb", "artifacts/appdb.dump", []byte("verified backup bytes"))

	if err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb"); err != nil {
		t.Fatalf("VerifyPostgresDatabaseArtifact: %v", err)
	}
}

func TestVerifyPostgresDatabaseArtifactRequiresExactlyOneMatch(t *testing.T) {
	dir, manifest := writeArtifactFixture(t, "otherdb", "artifacts/other.dump", []byte("other bytes"))

	t.Run("missing", func(t *testing.T) {
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
		if !errors.Is(err, ErrPostgresArtifactNotFound) {
			t.Fatalf("err = %v, want ErrPostgresArtifactNotFound", err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		duplicate := manifest.Artifacts[0]
		duplicate.Name = "appdb"
		manifest.Artifacts = []Artifact{duplicate, duplicate}
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
		if !errors.Is(err, ErrPostgresArtifactNotUnique) {
			t.Fatalf("err = %v, want ErrPostgresArtifactNotUnique", err)
		}
	})
}

func TestVerifyPostgresDatabaseArtifactRejectsMissingFileWithoutCreatingDirectories(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestForArtifact("appdb", "missing/nested/appdb.dump", []byte("absent"))

	err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
	if !errors.Is(err, ErrPostgresArtifactUnavailable) {
		t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("verification created a directory: %v", statErr)
	}
}

func TestVerifyPostgresDatabaseArtifactRejectsUnsafeMetadata(t *testing.T) {
	dir, manifest := writeArtifactFixture(t, "appdb", "artifact.dump", []byte("backup bytes"))
	cases := []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "traversal path", mutate: func(artifact *Artifact) { artifact.Path = "../escape.dump" }},
		{name: "absolute path", mutate: func(artifact *Artifact) { artifact.Path = filepath.Join(dir, "artifact.dump") }},
		{name: "uppercase hash", mutate: func(artifact *Artifact) { artifact.SHA256 = strings.Repeat("A", 64) }},
		{name: "negative size", mutate: func(artifact *Artifact) { artifact.SizeBytes = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			invalid := manifest
			invalid.Artifacts = append([]Artifact(nil), manifest.Artifacts...)
			tc.mutate(&invalid.Artifacts[0])
			err := VerifyPostgresDatabaseArtifact(context.Background(), dir, invalid, "appdb")
			if !errors.Is(err, ErrPostgresArtifactUnavailable) {
				t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
			}
		})
	}
}

func TestVerifyPostgresDatabaseArtifactRejectsSymlinks(t *testing.T) {
	payload := []byte("outside canary bytes")

	t.Run("final", func(t *testing.T) {
		dir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.dump")
		if err := os.WriteFile(outside, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "artifact.dump")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		manifest := manifestForArtifact("appdb", "artifact.dump", payload)
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
		if !errors.Is(err, ErrPostgresArtifactUnavailable) {
			t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
		}
	})

	t.Run("intermediate", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "artifact.dump"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		manifest := manifestForArtifact("appdb", "linked/artifact.dump", payload)
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
		if !errors.Is(err, ErrPostgresArtifactUnavailable) {
			t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
		}
	})
}

func TestVerifyPostgresDatabaseArtifactChecksActualSizeAndHash(t *testing.T) {
	dir, manifest := writeArtifactFixture(t, "appdb", "artifact.dump", []byte("actual backup bytes"))

	t.Run("wrong size", func(t *testing.T) {
		wrong := manifest
		wrong.Artifacts = append([]Artifact(nil), manifest.Artifacts...)
		wrong.Artifacts[0].SizeBytes++
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, wrong, "appdb")
		if !errors.Is(err, ErrPostgresArtifactSizeMismatch) {
			t.Fatalf("err = %v, want ErrPostgresArtifactSizeMismatch", err)
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		wrong := manifest
		wrong.Artifacts = append([]Artifact(nil), manifest.Artifacts...)
		wrong.Artifacts[0].SHA256 = strings.Repeat("0", 64)
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, wrong, "appdb")
		if !errors.Is(err, ErrPostgresArtifactChecksumMismatch) {
			t.Fatalf("err = %v, want ErrPostgresArtifactChecksumMismatch", err)
		}
	})
}

func TestVerifyPostgresDatabaseArtifactRejectsNonRegularAndReplacement(t *testing.T) {
	t.Run("non-regular", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "artifact.dump"), 0o700); err != nil {
			t.Fatal(err)
		}
		manifest := manifestForArtifact("appdb", "artifact.dump", nil)
		err := VerifyPostgresDatabaseArtifact(context.Background(), dir, manifest, "appdb")
		if !errors.Is(err, ErrPostgresArtifactUnavailable) {
			t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
		}
	})

	t.Run("replacement after open", func(t *testing.T) {
		dir, _ := writeArtifactFixture(t, "appdb", "artifact.dump", []byte("original"))
		file, path, err := openPostgresArtifactNoCreate(dir, "artifact.dump")
		if err != nil {
			t.Fatalf("openPostgresArtifactNoCreate: %v", err)
		}
		defer file.Close()

		if err := os.Rename(path, filepath.Join(dir, "moved.dump")); err != nil {
			t.Skipf("open-file replacement unavailable: %v", err)
		}
		if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyPostgresArtifactIdentity(file, path); !errors.Is(err, ErrPostgresArtifactUnavailable) {
			t.Fatalf("err = %v, want ErrPostgresArtifactUnavailable", err)
		}
	})
}

func TestVerifyPostgresDatabaseArtifactHonorsCancelledContext(t *testing.T) {
	dir, manifest := writeArtifactFixture(t, "appdb", "artifact.dump", []byte("backup bytes"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := VerifyPostgresDatabaseArtifact(ctx, dir, manifest, "appdb")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestVerifyPostgresDatabaseArtifactErrorsDoNotLeakCanaries(t *testing.T) {
	const (
		database = "database-canary-71f9"
		rel      = "path-canary-82e4/artifact.dump"
		content  = "content-canary-3ab6"
	)
	manifest := manifestForArtifact(database, rel, []byte(content))
	manifest.Artifacts[0].SHA256 = strings.Repeat("f", 64)

	err := VerifyPostgresDatabaseArtifact(context.Background(), t.TempDir(), manifest, database)
	if err == nil {
		t.Fatal("expected verification failure")
	}
	for _, canary := range []string{database, rel, content, manifest.Artifacts[0].SHA256} {
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("error leaked canary %q: %q", canary, err)
		}
	}
}

func writeArtifactFixture(t *testing.T, database, rel string, payload []byte) (string, Manifest) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, manifestForArtifact(database, rel, payload)
}

func manifestForArtifact(database, rel string, payload []byte) Manifest {
	sum := sha256.Sum256(payload)
	return Manifest{Artifacts: []Artifact{{
		Kind:      ArtifactKindPostgresDatabase,
		Name:      database,
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(payload)),
		Path:      rel,
	}}}
}
