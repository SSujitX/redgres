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
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/securefile"
)

const (
	maxManifestBytes           = 8 * 1024 * 1024
	maxManifestArtifacts       = 1024
	maxArtifactKindBytes       = 64
	maxArtifactNameBytes       = 512
	maxArtifactPathBytes       = 4096
	maxSystemIdentifierBytes   = 20
	maxRestoreOutcomeBytes     = 64
	maxRestoreBackupSetIDBytes = 32
	maxRedgresIdentityBytes    = 512
	maxJSONDepth               = 32
	maxJSONTokens              = 32768
	jsonContextManifest        = "$manifest"
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
	raw, err := io.ReadAll(io.LimitReader(file, int64(maxManifestBytes)+1))
	if err != nil {
		return Manifest{}, errInvalidManifest
	}
	if len(raw) > maxManifestBytes {
		return Manifest{}, errInvalidManifest
	}
	return parseManifestBytes(catalogDir, raw)
}

func parseManifestBytes(catalogDir string, raw []byte) (Manifest, error) {
	if len(raw) > maxManifestBytes {
		return Manifest{}, errInvalidManifest
	}
	if !utf8.Valid(raw) || !strictJSONStrings(raw) {
		return Manifest{}, errInvalidManifest
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Manifest{}, errInvalidManifest
	}
	if err := validateJSONBounds(raw); err != nil {
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
	if !boundedText(manifest.Cluster.SystemIdentifier, maxSystemIdentifierBytes, false) ||
		!decimalString(manifest.Cluster.SystemIdentifier) {
		return errInvalidManifest
	}
	if len(manifest.Artifacts) > maxManifestArtifacts {
		return errInvalidManifest
	}
	for _, artifact := range manifest.Artifacts {
		if !boundedText(artifact.Kind, maxArtifactKindBytes, false) ||
			!boundedText(artifact.Name, maxArtifactNameBytes, false) ||
			!boundedText(artifact.Path, maxArtifactPathBytes, false) ||
			!lowerHex(artifact.SHA256, 64) || artifact.SizeBytes < 0 || !jailLocal(artifact.Path) {
			return errInvalidManifest
		}
	}
	if !boundedText(manifest.Restore.Outcome, maxRestoreOutcomeBytes, true) ||
		!boundedText(manifest.Restore.BackupSetID, maxRestoreBackupSetIDBytes, true) ||
		!boundedText(manifest.Redgres.Version, maxRedgresIdentityBytes, true) ||
		!boundedText(manifest.Redgres.CompatibilityPolicyRevision, maxRedgresIdentityBytes, true) {
		return errInvalidManifest
	}
	return nil
}

func boundedText(value string, maxBytes int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return len(value) <= maxBytes && utf8.ValidString(value)
}

type jsonBudget struct {
	tokens int
}

func validateJSONBounds(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	budget := jsonBudget{}
	if err := walkJSONValue(dec, &budget, 0, jsonContextManifest); err != nil {
		return errInvalidManifest
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errInvalidManifest
	}
	return nil
}

func walkJSONValue(dec *json.Decoder, budget *jsonBudget, depth int, field string) error {
	if depth > maxJSONDepth {
		return errInvalidManifest
	}
	token, err := nextJSONToken(dec, budget)
	if err != nil {
		return errInvalidManifest
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyToken, err := nextJSONToken(dec, budget)
			if err != nil {
				return errInvalidManifest
			}
			key, ok := keyToken.(string)
			if !ok {
				return errInvalidManifest
			}
			if _, duplicate := seen[key]; duplicate {
				return errInvalidManifest
			}
			if !allowedManifestJSONField(field, key) {
				return errInvalidManifest
			}
			seen[key] = struct{}{}
			if _, banned := secretKeyNames[strings.ToLower(key)]; banned {
				return errInvalidManifest
			}
			if err := walkJSONValue(dec, budget, depth+1, key); err != nil {
				return err
			}
		}
		end, err := nextJSONToken(dec, budget)
		if err != nil || end != json.Delim('}') {
			return errInvalidManifest
		}
		return nil
	case '[':
		count := 0
		for dec.More() {
			count++
			if field == "artifacts" && count > maxManifestArtifacts {
				return errInvalidManifest
			}
			if err := walkJSONValue(dec, budget, depth+1, field); err != nil {
				return err
			}
		}
		end, err := nextJSONToken(dec, budget)
		if err != nil || end != json.Delim(']') {
			return errInvalidManifest
		}
		return nil
	default:
		return errInvalidManifest
	}
}

func allowedManifestJSONField(context, key string) bool {
	switch context {
	case jsonContextManifest:
		switch key {
		case "schema_version", "backup_set_id", "completed_at", "cluster", "artifacts", "off_host", "restore", "redgres":
			return true
		}
	case "cluster":
		return key == "system_identifier"
	case "artifacts":
		switch key {
		case "kind", "name", "sha256", "size_bytes", "path":
			return true
		}
	case "off_host":
		switch key {
		case "completed", "copied_at":
			return true
		}
	case "restore":
		switch key {
		case "isolated", "outcome", "backup_set_id", "completed_at":
			return true
		}
	case "redgres":
		switch key {
		case "version", "compatibility_policy_revision":
			return true
		}
	}
	return false
}

func nextJSONToken(dec *json.Decoder, budget *jsonBudget) (json.Token, error) {
	budget.tokens++
	if budget.tokens > maxJSONTokens {
		return nil, errInvalidManifest
	}
	return dec.Token()
}

func strictJSONStrings(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		current := raw[i]
		if !inString {
			if current == '"' {
				inString = true
			}
			continue
		}
		if current == '"' {
			inString = false
			continue
		}
		if current < 0x20 {
			return false
		}
		if current != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		switch raw[i] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			continue
		case 'u':
			unit, ok := jsonHexUnit(raw, i+1)
			if !ok {
				return false
			}
			i += 4
			if unit >= 0xd800 && unit <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := jsonHexUnit(raw, i+3)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			} else if unit >= 0xdc00 && unit <= 0xdfff {
				return false
			}
		default:
			return false
		}
	}
	return !inString
}

func jsonHexUnit(raw []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, current := range raw[start : start+4] {
		value <<= 4
		switch {
		case current >= '0' && current <= '9':
			value += uint16(current - '0')
		case current >= 'a' && current <= 'f':
			value += uint16(current-'a') + 10
		case current >= 'A' && current <= 'F':
			value += uint16(current-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
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
