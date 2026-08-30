package config

import (
	"fmt"
	"net"
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

func (c *Config) loadToolGate() error {
	c.PgAdminEmail = strings.TrimSpace(strings.ToLower(os.Getenv("REDGRES_PGADMIN_EMAIL")))
	c.PgAdminPasswordFile = strings.TrimSpace(os.Getenv("REDGRES_PGADMIN_PASSWORD_FILE"))
	if err := validateOptionalSecretFilePath(c.PgAdminPasswordFile, "REDGRES_PGADMIN_PASSWORD_FILE", c.Production()); err != nil {
		return err
	}
	pgListen, err := validateLoopbackListen(os.Getenv("REDGRES_TOOL_GATE_PGADMIN_LISTEN"), "REDGRES_TOOL_GATE_PGADMIN_LISTEN")
	if err != nil {
		return err
	}
	pgUp, err := validateToolUpstream(os.Getenv("REDGRES_TOOL_GATE_PGADMIN_UPSTREAM"), "REDGRES_TOOL_GATE_PGADMIN_UPSTREAM")
	if err != nil {
		return err
	}
	riListen, err := validateLoopbackListen(os.Getenv("REDGRES_TOOL_GATE_REDISINSIGHT_LISTEN"), "REDGRES_TOOL_GATE_REDISINSIGHT_LISTEN")
	if err != nil {
		return err
	}
	riUp, err := validateToolUpstream(os.Getenv("REDGRES_TOOL_GATE_REDISINSIGHT_UPSTREAM"), "REDGRES_TOOL_GATE_REDISINSIGHT_UPSTREAM")
	if err != nil {
		return err
	}
	c.ToolGatePgAdminListen = pgListen
	c.ToolGatePgAdminUpstream = pgUp
	c.ToolGateRedisListen = riListen
	c.ToolGateRedisUpstream = riUp
	return nil
}

func validateLoopbackListen(raw, key string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || port == "" {
		return "", fmt.Errorf("%s: must be host:port on loopback", key)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("%s: must bind loopback", key)
	}
	return raw, nil
}

func validateToolUpstream(raw, key string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%s: must be a loopback URL", key)
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("%s: must be loopback", key)
	}
	return parsed.String(), nil
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
