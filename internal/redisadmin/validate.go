package redisadmin

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	MinUsernameLen = 3
	MaxUsernameLen = 48
	MinPrefixLen   = 2
	MaxPrefixLen   = 80
)

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,47}$`)
	prefixRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_:-]{0,78}[a-z0-9]$|^[a-z0-9]{2}$`)
)

func ValidateUsername(username string) error {
	if len(username) < MinUsernameLen || len(username) > MaxUsernameLen {
		return ErrInvalidUsername
	}
	if !usernameRe.MatchString(username) {
		return ErrInvalidUsername
	}
	return nil
}

func NormalizePrefix(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ErrInvalidPrefix
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", ErrInvalidPrefix
		}
	}
	prefix := strings.TrimSuffix(trimmed, ":*")
	prefix = strings.TrimSuffix(prefix, ":")
	if strings.ContainsAny(prefix, "*?[]") {
		return "", ErrInvalidPrefix
	}
	if len(prefix) < MinPrefixLen || len(prefix) > MaxPrefixLen {
		return "", ErrInvalidPrefix
	}
	if !prefixRe.MatchString(prefix) {
		return "", ErrInvalidPrefix
	}
	return prefix + ":*", nil
}
