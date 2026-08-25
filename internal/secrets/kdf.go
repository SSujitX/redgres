package secrets

import (
	"crypto/sha256"
	"encoding/base64"
)

// vaultKDFPrefix is the exact sibling _cipher prefix. It has no extra space or newline.
const vaultKDFPrefix = "database-console-vault-v1:"

// DeriveVaultKey returns the URL-safe Base64 Fernet key for SESSION_SECRET.
// The KDF is SHA-256(UTF-8(database-console-vault-v1: + secret)).
func DeriveVaultKey(sessionSecret string) string {
	sum := sha256.Sum256([]byte(vaultKDFPrefix + sessionSecret))
	return base64.URLEncoding.EncodeToString(sum[:])
}
