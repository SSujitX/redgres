package postgresadmin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
)

func TestLoadVaultKeyUnsetIsEmpty(t *testing.T) {
	key, err := loadVaultKey(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		t.Fatalf("key = %q", key)
	}
}

func TestLoadVaultKeyDerivesFernetKeyAndWipesRaw(t *testing.T) {
	fx := loadPython49(t)
	path := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(path, []byte(fx.SessionSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if key != fx.DerivedFernetKey {
		t.Fatalf("derived key mismatch")
	}
	if key == fx.SessionSecret {
		t.Fatal("must store derived Fernet key, not raw secret")
	}
	if secrets.DeriveVaultKey(fx.SessionSecret) != key {
		t.Fatal("key must match DeriveVaultKey")
	}
}

func TestLoadVaultKeyEmptyIsNamedEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-vault")
	if err := os.WriteFile(path, []byte("\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path})
	if err == nil {
		t.Fatal("expected empty file to fail")
	}
	if err.Error() != "REDGRES_LEGACY_VAULT_SECRET_FILE: is empty" {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLoadVaultKeyMissingIsNamedEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-canary-secret")
	_, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path})
	if err == nil {
		t.Fatal("expected missing file to fail")
	}
	if err.Error() != "REDGRES_LEGACY_VAULT_SECRET_FILE: is unavailable" {
		t.Fatalf("err = %q", err.Error())
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), path) {
		t.Fatalf("error echoed path: %q", err.Error())
	}
}

func TestLoadVaultKeyRejectsSymlinkWithoutReadingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vault-target")
	const canary = "vault-symlink-canary"
	if err := os.WriteFile(target, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "vault-secret")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path})
	if err == nil {
		t.Fatal("expected symlink to fail")
	}
	if err.Error() != "REDGRES_LEGACY_VAULT_SECRET_FILE: must be a regular file" {
		t.Fatalf("err = %q", err.Error())
	}
	if strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), path) {
		t.Fatalf("error exposed canary or path: %q", err.Error())
	}
}

func TestLoadVaultKeyRejectsNonRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path})
	if err == nil {
		t.Fatal("expected non-regular file to fail")
	}
	if err.Error() != "REDGRES_LEGACY_VAULT_SECRET_FILE: must be a regular file" {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLoadVaultKeyProductionRejectsGroupWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(path, []byte("session-secret-canary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path, Environment: config.EnvironmentProduction})
	if err == nil {
		t.Fatal("expected group/world-readable production file to fail")
	}
	if err.Error() != "REDGRES_LEGACY_VAULT_SECRET_FILE: must not be group or world accessible" {
		t.Fatalf("err = %q", err.Error())
	}
	if strings.Contains(err.Error(), "session-secret-canary") {
		t.Fatalf("error echoed contents: %q", err.Error())
	}
}

func TestLoadVaultKeyProductionAcceptsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not enforced on Windows")
	}
	fx := loadPython49(t)
	path := filepath.Join(t.TempDir(), "vault-secret")
	if err := os.WriteFile(path, []byte(fx.SessionSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := loadVaultKey(config.Config{LegacyVaultSecretFile: path, Environment: config.EnvironmentProduction})
	if err != nil {
		t.Fatal(err)
	}
	if key != fx.DerivedFernetKey {
		t.Fatal("derived key mismatch")
	}
}

func TestOpenWithoutPostgresIgnoresVaultSecretFile(t *testing.T) {
	svc, closer, err := Open(t.Context(), config.Config{
		LegacyVaultSecretFile: filepath.Join(t.TempDir(), "missing-canary-secret"),
	})
	if closer != nil {
		defer closer()
	}
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if svc == nil {
		t.Fatal("expected service")
	}
	if svc.vaultKey != "" {
		t.Fatal("unset postgres must not load vault key")
	}
	_, revealErr := svc.Reveal(t.Context(), "project_a")
	if revealErr == nil || !strings.Contains(revealErr.Error(), "unavailable") {
		t.Fatalf("reveal err = %v", revealErr)
	}
}
