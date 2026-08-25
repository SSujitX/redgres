package postgresadmin

import (
	"crypto/rand"
	"encoding/base64"
)

const passwordBytes = 24

func GeneratePassword() (string, error) {
	buf := make([]byte, passwordBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
