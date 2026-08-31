package postgresadmin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func trustedSystemdVaultCredential(path, envName string, info fs.FileInfo) bool {
	if envName != "REDGRES_LEGACY_VAULT_SECRET_FILE" || info.Mode().Perm() != 0o440 || !rootOwnedFile(info) {
		return false
	}
	dir := filepath.Clean(strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY")))
	if dir == "." || !filepath.IsAbs(dir) {
		return false
	}
	runtimeRoot := filepath.Clean("/run/credentials") + string(filepath.Separator)
	if !strings.HasPrefix(dir+string(filepath.Separator), runtimeRoot) {
		return false
	}
	return filepath.Clean(path) == filepath.Join(dir, "legacy-vault-secret")
}
