package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
)

var errInvalidManifest = errors.New("backup manifest is invalid")

var secretKeyNames = map[string]struct{}{
	"password":   {},
	"token":      {},
	"url":        {},
	"secret":     {},
	"credential": {},
}

func ParseManifest(catalogDir, relativePath string) (Manifest, error) {
	if !jailLocal(relativePath) {
		return Manifest{}, errInvalidManifest
	}
	file, err := securefile.OpenRegularUnder(catalogDir, filepath.Join(catalogDir, filepath.FromSlash(relativePath)), os.O_RDONLY, 0)
	if err != nil {
		return Manifest{}, errInvalidManifest
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return Manifest{}, errInvalidManifest
	}
	return parseManifestBytes(catalogDir, raw)
}

func parseManifestBytes(catalogDir string, raw []byte) (Manifest, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Manifest{}, errInvalidManifest
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return Manifest{}, errInvalidManifest
	}
	if err := rejectSecretKeys(tree); err != nil {
		return Manifest{}, errInvalidManifest
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, errInvalidManifest
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errInvalidManifest
	}
	if err := validateParsedManifest(catalogDir, manifest); err != nil {
		return Manifest{}, errInvalidManifest
	}
	return manifest, nil
}

func validateParsedManifest(catalogDir string, manifest Manifest) error {
	if err := validateManifestStructure(manifest); err != nil {
		return err
	}
	for _, artifact := range manifest.Artifacts {
		if err := rejectSymlinkInJail(catalogDir, artifact.Path); err != nil {
			return errInvalidManifest
		}
	}
	return nil
}

func validateManifestStructure(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return errInvalidManifest
	}
	if !lowerHex(manifest.BackupSetID, 32) {
		return errInvalidManifest
	}
	if manifest.CompletedAt.IsZero() {
		return errInvalidManifest
	}
	if !decimalString(manifest.Cluster.SystemIdentifier) {
		return errInvalidManifest
	}
	for _, artifact := range manifest.Artifacts {
		if !lowerHex(artifact.SHA256, 64) || artifact.SizeBytes < 0 || !jailLocal(artifact.Path) {
			return errInvalidManifest
		}
	}
	return nil
}

func rejectSecretKeys(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, banned := secretKeyNames[strings.ToLower(key)]; banned {
				return errInvalidManifest
			}
			if err := rejectSecretKeys(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectSecretKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectSymlinkInJail(jail, rel string) error {
	acc := jail
	for _, part := range strings.FieldsFunc(filepath.FromSlash(rel), isPathSeparator) {
		if part == "" || part == "." {
			continue
		}
		acc = filepath.Join(acc, part)
		info, err := os.Lstat(acc)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errInvalidManifest
		}
	}
	return nil
}

func lowerHex(value string, n int) bool {
	if len(value) != n {
		return false
	}
	for i := 0; i < n; i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func decimalString(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
