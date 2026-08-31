//go:build !windows

package postgresadmin

import (
	"io/fs"
	"syscall"
	"testing"
	"time"
)

type credentialFileInfo struct {
	mode fs.FileMode
	uid  uint32
	gid  uint32
}

func (i credentialFileInfo) Name() string       { return "legacy-vault-secret" }
func (i credentialFileInfo) Size() int64        { return 32 }
func (i credentialFileInfo) Mode() fs.FileMode  { return i.mode }
func (i credentialFileInfo) ModTime() time.Time { return time.Time{} }
func (i credentialFileInfo) IsDir() bool        { return false }
func (i credentialFileInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid, Gid: i.gid} }

func TestTrustedSystemdVaultCredentialRequiresExactRootOwnedRuntimeFile(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "/run/credentials/redgres.service")
	path := "/run/credentials/redgres.service/legacy-vault-secret"
	trusted := credentialFileInfo{mode: 0o440, uid: 0, gid: 0}
	if !trustedSystemdVaultCredential(path, "REDGRES_LEGACY_VAULT_SECRET_FILE", trusted) {
		t.Fatal("expected exact systemd credential to be trusted")
	}
	for name, test := range map[string]struct {
		path string
		env  string
		info credentialFileInfo
	}{
		"wrong path":   {"/tmp/legacy-vault-secret", "REDGRES_LEGACY_VAULT_SECRET_FILE", trusted},
		"wrong env":    {path, "REDGRES_POSTGRES_PASSWORD_FILE", trusted},
		"writable":     {path, "REDGRES_LEGACY_VAULT_SECRET_FILE", credentialFileInfo{mode: 0o640, uid: 0, gid: 0}},
		"non-root uid": {path, "REDGRES_LEGACY_VAULT_SECRET_FILE", credentialFileInfo{mode: 0o440, uid: 1000, gid: 0}},
		"non-root gid": {path, "REDGRES_LEGACY_VAULT_SECRET_FILE", credentialFileInfo{mode: 0o440, uid: 0, gid: 1000}},
	} {
		t.Run(name, func(t *testing.T) {
			if trustedSystemdVaultCredential(test.path, test.env, test.info) {
				t.Fatal("unexpected trust")
			}
		})
	}
}
