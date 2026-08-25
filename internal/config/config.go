package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"

	DefaultAddress            = "127.0.0.1:8790"
	DefaultDevelopmentBaseURL = "http://127.0.0.1:8790"
	DefaultSQLitePath         = "./redgres.db"
	DefaultSessionTTL         = 12 * time.Hour
	DefaultAbsoluteSessionTTL = 24 * time.Hour
	DefaultLogLevel           = "info"

	MinSessionTTL     = 5 * time.Minute
	MaxSessionTTL     = 24 * time.Hour
	MaxAbsoluteTTL    = 7 * 24 * time.Hour
	dotenvDefaultPath = ".env"
)

type Config struct {
	Environment        string
	Address            string
	BaseURL            string
	SQLitePath         string
	SessionTTL         time.Duration
	AbsoluteSessionTTL time.Duration
	CookieSecure       bool
	LogLevel           string
	DevAssetDir        string

	PostgresHost               string
	PostgresPort               string
	PostgresDatabase           string
	PostgresUser               string
	PostgresPasswordFile       string
	PostgresSSLMode            string
	PostgresSSLRootCert        string
	PostgresExpectedMajor      int
	PostgresProtectedDatabases []string
	PostgresProtectedRoles     []string
	PostgresPublicHost         string
	PostgresDirectPort         string
	PostgresPooledPort         string
	LegacyVaultSecretFile      string

	RedisAdminURLFile   string
	RedisAllowPlaintext bool
	RedisPublicHost     string
	RedisPublicPort     string

	PgAdminURL      string
	RedisInsightURL string
}

func Load(args []string) (Config, error) {
	dotenvApplied := false
	if environmentFromFlagsAndEnv(args) != EnvironmentProduction {
		applied, err := loadDotEnvFile(dotenvDefaultPath)
		if err != nil {
			return Config{}, err
		}
		dotenvApplied = applied
	}

	cfg := Config{
		Environment:        normalizeEnvironment(envOr("REDGRES_ENVIRONMENT", EnvironmentDevelopment)),
		Address:            envOr("REDGRES_ADDRESS", DefaultAddress),
		BaseURL:            envOr("REDGRES_BASE_URL", DefaultDevelopmentBaseURL),
		SQLitePath:         envOr("REDGRES_SQLITE_PATH", DefaultSQLitePath),
		SessionTTL:         DefaultSessionTTL,
		AbsoluteSessionTTL: DefaultAbsoluteSessionTTL,
		CookieSecure:       envBoolDefaultFalse("REDGRES_COOKIE_SECURE"),
		LogLevel:           strings.ToLower(envOr("REDGRES_LOG_LEVEL", DefaultLogLevel)),
	}

	var err error
	if cfg.SessionTTL, err = envDuration("REDGRES_SESSION_TTL", DefaultSessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.AbsoluteSessionTTL, err = envDuration("REDGRES_ABSOLUTE_SESSION_TTL", DefaultAbsoluteSessionTTL); err != nil {
		return Config{}, err
	}
	if v, err := envBool("REDGRES_COOKIE_SECURE"); err != nil {
		return Config{}, err
	} else if v != nil {
		cfg.CookieSecure = *v
	}

	fs := flag.NewFlagSet("redgres", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.Environment, "environment", cfg.Environment, "runtime environment")
	fs.StringVar(&cfg.Address, "address", cfg.Address, "listen address")
	fs.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "public base URL")
	fs.StringVar(&cfg.SQLitePath, "sqlite-path", cfg.SQLitePath, "SQLite database path")
	fs.DurationVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "idle session TTL")
	fs.DurationVar(&cfg.AbsoluteSessionTTL, "absolute-session-ttl", cfg.AbsoluteSessionTTL, "absolute session TTL")
	fs.BoolVar(&cfg.CookieSecure, "cookie-secure", cfg.CookieSecure, "set the session cookie Secure flag")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level")
	if err := fs.Parse(args); err != nil {
		return Config{}, sanitizeFlagError(err)
	}

	cfg.Environment = normalizeEnvironment(cfg.Environment)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if cfg.Environment == EnvironmentProduction && dotenvApplied {
		return Config{}, errors.New("REDGRES_ENVIRONMENT: production cannot be selected from a dotenv file")
	}
	if err := cfg.loadDevAssetDir(); err != nil {
		return Config{}, err
	}
	if err := cfg.loadPostgres(); err != nil {
		return Config{}, err
	}
	if err := cfg.loadRedis(); err != nil {
		return Config{}, err
	}
	if err := cfg.loadToolLinks(); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadDevAssetDir resolves the optional development frontend asset directory.
// An empty value selects the embedded assets. Production rejects any value so a
// running deployment can never serve frontend files from an unmanaged path.
func (c *Config) loadDevAssetDir() error {
	raw := strings.TrimSpace(os.Getenv("REDGRES_DEV_ASSET_DIR"))
	if raw == "" {
		c.DevAssetDir = ""
		return nil
	}
	if c.Production() {
		return errors.New("REDGRES_DEV_ASSET_DIR: not allowed in production")
	}
	if strings.ContainsAny(raw, "?#") || strings.ContainsRune(raw, 0) {
		return errors.New("REDGRES_DEV_ASSET_DIR: must not contain URI reserved characters")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return errors.New("REDGRES_DEV_ASSET_DIR: invalid path")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return errors.New("REDGRES_DEV_ASSET_DIR: path is unavailable")
	}
	if !info.IsDir() {
		return errors.New("REDGRES_DEV_ASSET_DIR: must be a directory")
	}
	c.DevAssetDir = abs
	return nil
}

func LoadDevelopmentDotEnv(args []string) error {
	if environmentFromFlagsAndEnv(args) == EnvironmentProduction {
		return nil
	}
	applied, err := loadDotEnvFile(dotenvDefaultPath)
	if err != nil {
		return err
	}
	if applied && normalizeEnvironment(os.Getenv("REDGRES_ENVIRONMENT")) == EnvironmentProduction {
		return errors.New("REDGRES_ENVIRONMENT: production cannot be selected from a dotenv file")
	}
	return nil
}

func environmentFromFlagsAndEnv(args []string) string {
	env := normalizeEnvironment(os.Getenv("REDGRES_ENVIRONMENT"))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-environment" || arg == "--environment":
			if i+1 < len(args) {
				return normalizeEnvironment(args[i+1])
			}
		case strings.HasPrefix(arg, "-environment="):
			return normalizeEnvironment(strings.TrimPrefix(arg, "-environment="))
		case strings.HasPrefix(arg, "--environment="):
			return normalizeEnvironment(strings.TrimPrefix(arg, "--environment="))
		}
	}
	if env == "" {
		return EnvironmentDevelopment
	}
	return env
}

func sanitizeFlagError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	msg := err.Error()
	const marker = "for flag "
	if i := strings.Index(msg, marker); i >= 0 {
		name, _, _ := strings.Cut(msg[i+len(marker):], ":")
		name = strings.TrimSpace(name)
		return fmt.Errorf("flag %s: invalid value", name)
	}
	return errors.New("flag: invalid arguments")
}

func (c Config) Production() bool {
	return c.Environment == EnvironmentProduction
}

func (c Config) validate() error {
	switch c.Environment {
	case EnvironmentDevelopment, EnvironmentProduction:
	default:
		return errors.New("REDGRES_ENVIRONMENT: must be development or production")
	}
	if err := validateLogLevel(c.LogLevel); err != nil {
		return err
	}
	if err := validateAddress(c.Address, c.Production()); err != nil {
		return err
	}
	if err := validateBaseURL(c.BaseURL, c.Production()); err != nil {
		return err
	}
	if err := validateSQLitePath(c.SQLitePath, c.Production()); err != nil {
		return err
	}
	if c.SessionTTL < MinSessionTTL || c.SessionTTL > MaxSessionTTL {
		return errors.New("REDGRES_SESSION_TTL: must be between 5m and 24h")
	}
	if c.AbsoluteSessionTTL < c.SessionTTL || c.AbsoluteSessionTTL > MaxAbsoluteTTL {
		return errors.New("REDGRES_ABSOLUTE_SESSION_TTL: must be at least the idle TTL and at most 168h")
	}
	if c.Production() && !c.CookieSecure {
		return errors.New("REDGRES_COOKIE_SECURE: must be true in production")
	}
	return nil
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return errors.New("REDGRES_LOG_LEVEL: must be debug, info, warn, or error")
	}
}

func validateAddress(address string, production bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return errors.New("REDGRES_ADDRESS: must be host:port")
	}
	if production && !isLoopbackHost(host) {
		return errors.New("REDGRES_ADDRESS: production bind must be loopback")
	}
	return nil
}

func validateBaseURL(raw string, production bool) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("REDGRES_BASE_URL: is required")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("REDGRES_BASE_URL: must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("REDGRES_BASE_URL: must be an origin (scheme://host[:port])")
	}
	if production && parsed.Scheme != "https" {
		return errors.New("REDGRES_BASE_URL: production origin must use https")
	}
	if !production && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("REDGRES_BASE_URL: must use http or https")
	}
	return nil
}

func validateSQLitePath(path string, production bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("REDGRES_SQLITE_PATH: is required")
	}
	if strings.ContainsAny(path, "?#") || strings.ContainsRune(path, 0) {
		return errors.New("REDGRES_SQLITE_PATH: must not contain URI reserved characters")
	}
	if production && !filepath.IsAbs(path) {
		return errors.New("REDGRES_SQLITE_PATH: production path must be absolute")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeEnvironment(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration", key)
	}
	return d, nil
}

func envBool(key string) (*bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		v := true
		return &v, nil
	case "0", "false", "no", "off":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("%s: must be true or false", key)
	}
}

func envBoolDefaultFalse(key string) bool {
	v, err := envBool(key)
	if err != nil || v == nil {
		return false
	}
	return *v
}
