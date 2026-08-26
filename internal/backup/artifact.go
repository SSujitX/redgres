package backup

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
)

var (
	ErrPostgresArtifactNotFound         = errors.New("matching PostgreSQL backup artifact is missing")
	ErrPostgresArtifactNotUnique        = errors.New("matching PostgreSQL backup artifact is not unique")
	ErrPostgresArtifactUnavailable      = errors.New("PostgreSQL backup artifact cannot be verified")
	ErrPostgresArtifactSizeMismatch     = errors.New("PostgreSQL backup artifact size does not match")
	ErrPostgresArtifactChecksumMismatch = errors.New("PostgreSQL backup artifact checksum does not match")
)

// VerifyPostgresDatabaseArtifact streams and verifies the one manifest artifact
// for database. It opens only an existing regular file below catalogDir and
// returns errors that do not contain manifest or filesystem values.
func VerifyPostgresDatabaseArtifact(ctx context.Context, catalogDir string, manifest Manifest, database string) error {
	if ctx == nil {
		return ErrPostgresArtifactUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	artifact, err := postgresArtifactForDatabase(manifest, database)
	if err != nil {
		return err
	}
	if artifact.SizeBytes < 0 || !lowerHex(artifact.SHA256, sha256.Size*2) || !jailLocal(artifact.Path) {
		return ErrPostgresArtifactUnavailable
	}
	expectedHash, err := hex.DecodeString(artifact.SHA256)
	if err != nil {
		return ErrPostgresArtifactUnavailable
	}

	file, path, err := openPostgresArtifactNoCreate(catalogDir, artifact.Path)
	if err != nil {
		return ErrPostgresArtifactUnavailable
	}
	defer file.Close()

	actualSize, actualHash, err := streamPostgresArtifact(ctx, file)
	if err != nil {
		return err
	}
	if err := verifyPostgresArtifactIdentity(file, path); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if actualSize != artifact.SizeBytes {
		return ErrPostgresArtifactSizeMismatch
	}
	if subtle.ConstantTimeCompare(actualHash[:], expectedHash) != 1 {
		return ErrPostgresArtifactChecksumMismatch
	}
	return nil
}

func postgresArtifactForDatabase(manifest Manifest, database string) (Artifact, error) {
	var match Artifact
	matches := 0
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == ArtifactKindPostgresDatabase && artifact.Name == database {
			match = artifact
			matches++
		}
	}
	switch matches {
	case 0:
		return Artifact{}, ErrPostgresArtifactNotFound
	case 1:
		return match, nil
	default:
		return Artifact{}, ErrPostgresArtifactNotUnique
	}
}

func openPostgresArtifactNoCreate(catalogDir, relativePath string) (*os.File, string, error) {
	if catalogDir == "" || !jailLocal(relativePath) {
		return nil, "", ErrPostgresArtifactUnavailable
	}
	rel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(relativePath, `\`, "/")))
	if rel == "." || !filepath.IsLocal(rel) {
		return nil, "", ErrPostgresArtifactUnavailable
	}
	jail := filepath.Clean(catalogDir)
	path := filepath.Join(jail, rel)
	underJail, err := filepath.Rel(jail, path)
	if err != nil || !filepath.IsLocal(underJail) {
		return nil, "", ErrPostgresArtifactUnavailable
	}
	file, err := securefile.OpenRegular(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, "", ErrPostgresArtifactUnavailable
	}
	return file, path, nil
}

func streamPostgresArtifact(ctx context.Context, file *os.File) (int64, [sha256.Size]byte, error) {
	hasher := sha256.New()
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, [sha256.Size]byte{}, err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			size += int64(n)
			_, _ = hasher.Write(buffer[:n])
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return 0, [sha256.Size]byte{}, ErrPostgresArtifactUnavailable
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	return size, sum, nil
}

func verifyPostgresArtifactIdentity(file *os.File, path string) error {
	if err := securefile.VerifyRegularPath(path, file); err != nil {
		return ErrPostgresArtifactUnavailable
	}
	return nil
}
