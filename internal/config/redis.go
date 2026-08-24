package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

func (c *Config) loadRedis() error {
	c.RedisAdminURLFile = strings.TrimSpace(os.Getenv("REDGRES_REDIS_ADMIN_URL_FILE"))
	if v, err := envBool("REDGRES_REDIS_ALLOW_PLAINTEXT"); err != nil {
		return err
	} else if v != nil {
		c.RedisAllowPlaintext = *v
	}
	return c.validateRedis()
}

func (c Config) RedisConfigured() bool {
	return c.RedisAdminURLFile != ""
}

func (c Config) redisAnySet() bool {
	return c.RedisAdminURLFile != "" || c.RedisAllowPlaintext
}

func (c *Config) validateRedis() error {
	if !c.redisAnySet() {
		return nil
	}
	_, err := c.RedisAdminURL()
	return err
}

// RedisAdminURL reads the administrator URL file. The caller must discard the
// returned value after constructing the Redis client; it is never logged.
func (c Config) RedisAdminURL() (string, error) {
	if c.RedisAdminURLFile == "" {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: is required when Redis is configured")
	}
	if looksLikeRedisURL(c.RedisAdminURLFile) {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: must be a file path")
	}
	raw, err := readRedisURLFile(c.RedisAdminURLFile, c.Production())
	if err != nil {
		return "", err
	}
	if err := validateRedisAdminURL(raw, c.RedisAllowPlaintext); err != nil {
		return "", err
	}
	return raw, nil
}

func looksLikeRedisURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "redis://") || strings.HasPrefix(lower, "rediss://")
}

func readRedisURLFile(path string, production bool) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: is unavailable")
	}
	if production && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: must not be group or world accessible")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: is unavailable")
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", errors.New("REDGRES_REDIS_ADMIN_URL_FILE: is empty")
	}
	return value, nil
}

func validateRedisAdminURL(raw string, allowPlaintext bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "rediss":
	case "redis":
		if !(allowPlaintext || isLoopbackHost(parsed.Hostname())) {
			return errors.New("REDGRES_REDIS_ADMIN_URL_FILE: plain redis:// to a non-loopback host requires REDGRES_REDIS_ALLOW_PLAINTEXT")
		}
	default:
		return errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	if skipVerifyEnabled(parsed) {
		return errors.New("REDGRES_REDIS_ADMIN_URL_FILE: invalid value")
	}
	return nil
}

func skipVerifyEnabled(parsed *url.URL) bool {
	switch strings.ToLower(strings.TrimSpace(parsed.Query().Get("skip_verify"))) {
	case "true", "1":
		return true
	default:
		return false
	}
}
