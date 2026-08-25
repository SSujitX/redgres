package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setCompletePostgres(t *testing.T) {
	t.Helper()
	t.Setenv("REDGRES_POSTGRES_HOST", "127.0.0.1")
	t.Setenv("REDGRES_POSTGRES_PORT", "5432")
	t.Setenv("REDGRES_POSTGRES_DATABASE", "postgres")
	t.Setenv("REDGRES_POSTGRES_USER", "redgres_console")
	t.Setenv("REDGRES_POSTGRES_PASSWORD_FILE", "./secrets/pg")
}

func TestLoadAcceptsValidPooledPort(t *testing.T) {
	isolateConfig(t)
	setCompletePostgres(t)
	t.Setenv("REDGRES_POSTGRES_POOLED_PORT", "6432")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostgresPooledPort != "6432" {
		t.Fatalf("PostgresPooledPort = %q", cfg.PostgresPooledPort)
	}
}

func TestLoadRejectsInvalidPooledPort(t *testing.T) {
	isolateConfig(t)
	setCompletePostgres(t)
	for _, port := range []string{"0", "65536", "not-a-port"} {
		t.Setenv("REDGRES_POSTGRES_POOLED_PORT", port)
		_, err := Load(nil)
		if err == nil {
			t.Fatalf("port %q: expected invalid pooled port to fail", port)
		}
		if err.Error() != "REDGRES_POSTGRES_POOLED_PORT: invalid value" {
			t.Fatalf("port %q: error = %q", port, err.Error())
		}
	}
}

func TestLoadPooledPortWithoutPostgresFails(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_POOLED_PORT", "6432")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected pooled port without postgres to fail")
	}
	if !strings.Contains(err.Error(), "REDGRES_POSTGRES_") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadProductionDoesNotRequirePooledPort(t *testing.T) {
	isolateConfig(t)
	abs := filepath.Join(t.TempDir(), "redgres.db")
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", abs)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
	setCompletePostgres(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Production() {
		t.Fatal("expected production")
	}
	if cfg.PostgresPooledPort != "" {
		t.Fatalf("PostgresPooledPort = %q", cfg.PostgresPooledPort)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("SessionTTL = %s", cfg.SessionTTL)
	}
}
