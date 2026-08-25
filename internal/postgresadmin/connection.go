package postgresadmin

import (
	"errors"
	"net"
	"strings"
)

func MaskedProjectConnectionURL(host, port, owner, database string) (string, error) {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return "", errors.New("public PostgreSQL host and port are required")
	}
	hostport := net.JoinHostPort(strings.Trim(host, "[]"), port)
	return "postgresql://" + encodeRFC3986Unreserved(owner) + ":********@" + hostport + "/" + encodeRFC3986Unreserved(database) + "?sslmode=require", nil
}

func encodeRFC3986Unreserved(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if rfc3986Unreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexUpper[c>>4])
		b.WriteByte(hexUpper[c&0x0F])
	}
	return b.String()
}

const hexUpper = "0123456789ABCDEF"

func rfc3986Unreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '-' || c == '.' || c == '_' || c == '~':
		return true
	default:
		return false
	}
}
