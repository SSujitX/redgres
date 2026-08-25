package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

func (c *Config) loadToolLinks() error {
	pg, err := validateToolLinkURL(os.Getenv("REDGRES_PGADMIN_URL"), "REDGRES_PGADMIN_URL", c.Production())
	if err != nil {
		return err
	}
	ri, err := validateToolLinkURL(os.Getenv("REDGRES_REDISINSIGHT_URL"), "REDGRES_REDISINSIGHT_URL", c.Production())
	if err != nil {
		return err
	}
	c.PgAdminURL = pg
	c.RedisInsightURL = ri
	return nil
}

func (c Config) ToolLinksConfigured() bool {
	return c.PgAdminURL != "" || c.RedisInsightURL != ""
}

func validateToolLinkURL(raw, key string, production bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" {
		return "", fmt.Errorf("%s: must be an absolute URL", key)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s: must use http or https", key)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s: must be an absolute URL", key)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%s: must not include userinfo", key)
	}
	if parsed.Fragment != "" || parsed.RawFragment != "" || strings.Contains(raw, "#") {
		return "", fmt.Errorf("%s: must not include a fragment", key)
	}
	if production && scheme != "https" {
		return "", fmt.Errorf("%s: production URL must use https", key)
	}
	return raw, nil
}
