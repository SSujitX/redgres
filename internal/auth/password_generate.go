package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// GeneratePassword returns a 32-character URL-safe random password built from
// 24 bytes of crypto/rand entropy. It always satisfies the owner password
// strength policy (minimum length, maximum length, and it does not equal the
// username in practice).
func GeneratePassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
