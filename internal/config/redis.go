package config

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
)

func (c *Config) loadRedis() error {
	c.RedisAdminURLFile = strings.TrimSpace(os.Getenv("REDGRES_REDIS_ADMIN_URL_FILE"))
	c.RedisPublicHost = strings.TrimSpace(os.Getenv("REDGRES_REDIS_PUBLIC_HOST"))
	c.RedisPublicPort = strings.TrimSpace(os.Getenv("REDGRES_REDIS_PUBLIC_PORT"))
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
	if err := validateRedisPublicHost(c.RedisPublicHost); err != nil {
		return err
	}
	if err := validateRedisPublicPort(c.RedisPublicPort); err != nil {
		return err
	}
	if !c.redisAnySet() {
		return nil
	}
	_, err := c.RedisAdminURL()
	return err
}

func validateRedisPublicHost(host string) error {
	if host == "" {
		return nil
	}
	if strings.ContainsAny(host, " \t\r\n@/") || strings.Contains(host, "://") {
		return errors.New("REDGRES_REDIS_PUBLIC_HOST: invalid value")
	}
	return nil
}

func validateRedisPublicPort(port string) error {
	if port == "" {
		return nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("REDGRES_REDIS_PUBLIC_PORT: invalid value")
	}
	return nil
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
	const envName = "REDGRES_REDIS_ADMIN_URL_FILE"
	raw, err := securefile.ReadRegular(path, func(mode fs.FileMode) error {
		if production && mode.Perm()&0o077 != 0 {
			return errors.New(envName + ": must not be group or world accessible")
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, securefile.ErrNotRegular) {
			return "", errors.New(envName + ": must be a regular file")
		}
		if strings.HasPrefix(err.Error(), envName+":") {
			return "", err
		}
		return "", errors.New(envName + ": is unavailable")
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", errors.New(envName + ": is empty")
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
