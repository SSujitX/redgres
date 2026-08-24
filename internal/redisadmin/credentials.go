package redisadmin

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"strings"
)

const passwordBytes = 24

func GeneratePassword() (string, error) {
	buf := make([]byte, passwordBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ProjectConnectionURL(host, port, username, password string) (string, error) {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return "", errors.New("public Redis host and port are required")
	}
	u := url.URL{
		Scheme: "rediss",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(strings.Trim(host, "[]"), port),
		Path:   "/0",
	}
	return u.String(), nil
}
