package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *Config) loadPostgres() error {
	c.PostgresHost = strings.TrimSpace(envOr("REDGRES_POSTGRES_HOST", ""))
	c.PostgresPort = strings.TrimSpace(envOr("REDGRES_POSTGRES_PORT", ""))
	c.PostgresDatabase = strings.TrimSpace(envOr("REDGRES_POSTGRES_DATABASE", ""))
	c.PostgresUser = strings.TrimSpace(envOr("REDGRES_POSTGRES_USER", ""))
	c.PostgresPasswordFile = strings.TrimSpace(envOr("REDGRES_POSTGRES_PASSWORD_FILE", ""))
	c.PostgresSSLMode = strings.ToLower(strings.TrimSpace(envOr("REDGRES_POSTGRES_SSLMODE", "")))
	c.PostgresSSLRootCert = strings.TrimSpace(envOr("REDGRES_POSTGRES_SSLROOTCERT", ""))
	if err := parseExpectedMajor(envOr("REDGRES_POSTGRES_EXPECTED_MAJOR", ""), &c.PostgresExpectedMajor); err != nil {
		return err
	}
	var err error
	if c.PostgresProtectedDatabases, err = parseProtectedList("REDGRES_POSTGRES_PROTECTED_DATABASES"); err != nil {
		return err
	}
	if c.PostgresProtectedRoles, err = parseProtectedList("REDGRES_POSTGRES_PROTECTED_ROLES"); err != nil {
		return err
	}
	return c.validatePostgres()
}

func (c Config) PostgresConfigured() bool {
	return c.PostgresHost != "" && c.PostgresDatabase != "" && c.PostgresUser != "" && c.PostgresPasswordFile != ""
}

func (c Config) postgresAnySet() bool {
	return c.PostgresHost != "" || c.PostgresPort != "" || c.PostgresDatabase != "" || c.PostgresUser != "" ||
		c.PostgresPasswordFile != "" || c.PostgresSSLMode != "" || c.PostgresSSLRootCert != "" || c.PostgresExpectedMajor != 0
}

func (c *Config) validatePostgres() error {
	if !c.postgresAnySet() {
		return nil
	}
	if c.PostgresHost == "" {
		return errors.New("REDGRES_POSTGRES_HOST: is required when PostgreSQL is configured")
	}
	if c.PostgresDatabase == "" {
		return errors.New("REDGRES_POSTGRES_DATABASE: is required when PostgreSQL is configured")
	}
	if c.PostgresUser == "" {
		return errors.New("REDGRES_POSTGRES_USER: is required when PostgreSQL is configured")
	}
	if c.PostgresPasswordFile == "" {
		return errors.New("REDGRES_POSTGRES_PASSWORD_FILE: is required when PostgreSQL is configured")
	}
	if c.PostgresPort == "" {
		c.PostgresPort = "5432"
	}
	if _, err := strconv.ParseUint(c.PostgresPort, 10, 16); err != nil {
		return errors.New("REDGRES_POSTGRES_PORT: invalid value")
	}
	if containsKeywordUnsafe(c.PostgresHost) {
		return errors.New("REDGRES_POSTGRES_HOST: invalid value")
	}
	if containsKeywordUnsafe(c.PostgresUser) {
		return errors.New("REDGRES_POSTGRES_USER: invalid value")
	}
	if containsKeywordUnsafe(c.PostgresDatabase) {
		return errors.New("REDGRES_POSTGRES_DATABASE: invalid value")
	}
	if c.PostgresSSLMode == "" {
		if c.Production() {
			c.PostgresSSLMode = "require"
		} else {
			c.PostgresSSLMode = "prefer"
		}
	}
	if c.Production() {
		switch c.PostgresSSLMode {
		case "require", "verify-ca", "verify-full":
		default:
			return errors.New("REDGRES_POSTGRES_SSLMODE: production must be require, verify-ca, or verify-full")
		}
	} else {
		switch c.PostgresSSLMode {
		case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		default:
			return errors.New("REDGRES_POSTGRES_SSLMODE: invalid value")
		}
	}
	if c.PostgresSSLRootCert != "" && (containsCertPathUnsafe(c.PostgresSSLRootCert) || strings.ContainsAny(c.PostgresSSLRootCert, "?#")) {
		return errors.New("REDGRES_POSTGRES_SSLROOTCERT: invalid value")
	}
	return nil
}

func parseExpectedMajor(raw string, dest *int) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*dest = 0
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || (n != 17 && n != 18) {
		return errors.New("REDGRES_POSTGRES_EXPECTED_MAJOR: must be 17 or 18")
	}
	*dest = n
	return nil
}

func parseProtectedList(key string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !validProtectedIdent(part) {
			return nil, fmt.Errorf("%s: invalid identifier", key)
		}
		out = append(out, part)
	}
	return out, nil
}

func validProtectedIdent(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if i == 0 {
			if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
				return false
			}
			continue
		}
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func containsKeywordUnsafe(value string) bool {
	return strings.ContainsAny(value, " ='\"\\\t\r\n") || strings.ContainsRune(value, 0)
}

func containsCertPathUnsafe(value string) bool {
	return strings.ContainsAny(value, "='\"\t\r\n") || strings.ContainsRune(value, 0)
}
