package config

import (
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
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
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

func TestLoadOptionalPostgresPublicHostDirectPort(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_PUBLIC_HOST", "db.example.com")
	t.Setenv("REDGRES_POSTGRES_DIRECT_PORT", "5432")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostgresPublicHost != "db.example.com" || cfg.PostgresDirectPort != "5432" {
		t.Fatalf("public = %q %q", cfg.PostgresPublicHost, cfg.PostgresDirectPort)
	}
	if cfg.PostgresConfigured() {
		t.Fatal("public host/port must not mark PostgreSQL configured")
	}
}

func TestLoadProductionDoesNotRequirePostgresPublicHostDirectPort(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
	setCompletePostgres(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostgresPublicHost != "" || cfg.PostgresDirectPort != "" {
		t.Fatalf("public = %q %q", cfg.PostgresPublicHost, cfg.PostgresDirectPort)
	}
}

func TestLoadRejectsInvalidPostgresPublicHost(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_PUBLIC_HOST", "postgresql://canary:secret@127.0.0.1/db")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid public host to fail")
	}
	if err.Error() != "REDGRES_POSTGRES_PUBLIC_HOST: invalid value" {
		t.Fatalf("error = %q", err.Error())
	}
	if strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error echoed canary: %q", err.Error())
	}
}

func TestLoadRejectsInvalidPostgresDirectPort(t *testing.T) {
	isolateConfig(t)
	for _, port := range []string{"0", "65536", "not-a-port"} {
		t.Setenv("REDGRES_POSTGRES_DIRECT_PORT", port)
		_, err := Load(nil)
		if err == nil {
			t.Fatalf("port %q: expected invalid direct port to fail", port)
		}
		if err.Error() != "REDGRES_POSTGRES_DIRECT_PORT: invalid value" {
			t.Fatalf("port %q: error = %q", port, err.Error())
		}
	}
}

func TestLoadOptionalLegacyVaultSecretFile(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_LEGACY_VAULT_SECRET_FILE", "./secrets/session")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LegacyVaultSecretFile != "./secrets/session" {
		t.Fatalf("LegacyVaultSecretFile = %q", cfg.LegacyVaultSecretFile)
	}
	if cfg.PostgresConfigured() {
		t.Fatal("vault secret file must not mark PostgreSQL configured")
	}
	if cfg.postgresAnySet() {
		t.Fatal("vault secret file must not be part of postgresAnySet")
	}
}

func TestLoadLegacyVaultSecretFileDoesNotCompletePostgres(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_HOST", "127.0.0.1")
	t.Setenv("REDGRES_LEGACY_VAULT_SECRET_FILE", "./secrets/session-canary")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected incomplete PostgreSQL to fail")
	}
	if !strings.Contains(err.Error(), "REDGRES_POSTGRES_") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "session-canary") || strings.Contains(err.Error(), "LEGACY_VAULT") {
		t.Fatalf("error must not treat vault file as postgresAnySet or echo path: %v", err)
	}
}

func TestLoadProductionDoesNotRequireLegacyVaultSecretFile(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
	setCompletePostgres(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LegacyVaultSecretFile != "" {
		t.Fatalf("LegacyVaultSecretFile = %q", cfg.LegacyVaultSecretFile)
	}
	if !cfg.PostgresConfigured() {
		t.Fatal("expected PostgreSQL configured without vault secret file")
	}
}

func TestLoadAcceptsPostgresExpectedMajor(t *testing.T) {
	for _, major := range []string{"17", "18"} {
		t.Run(major, func(t *testing.T) {
			isolateConfig(t)
			setCompletePostgres(t)
			t.Setenv("REDGRES_POSTGRES_EXPECTED_MAJOR", major)

			cfg, err := Load(nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if major == "17" && cfg.PostgresExpectedMajor != 17 {
				t.Fatalf("PostgresExpectedMajor = %d", cfg.PostgresExpectedMajor)
			}
			if major == "18" && cfg.PostgresExpectedMajor != 18 {
				t.Fatalf("PostgresExpectedMajor = %d", cfg.PostgresExpectedMajor)
			}
		})
	}
}

func TestLoadRejectsInvalidPostgresExpectedMajor(t *testing.T) {
	denied := []string{"latest", "latest-tested", "18.6", "17.11", "16", "19"}
	for _, major := range denied {
		t.Run(major, func(t *testing.T) {
			isolateConfig(t)
			setCompletePostgres(t)
			t.Setenv("REDGRES_POSTGRES_EXPECTED_MAJOR", major)

			_, err := Load(nil)
			if err == nil {
				t.Fatal("expected invalid major to fail")
			}
			if err.Error() != "REDGRES_POSTGRES_EXPECTED_MAJOR: must be 17 or 18" {
				t.Fatalf("error = %q", err.Error())
			}
		})
	}
}

func TestLoadPostgresExpectedMajorWithoutConnectionFailsClosed(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_EXPECTED_MAJOR", "18")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected major without PostgreSQL connection to fail")
	}
	if !strings.Contains(err.Error(), "REDGRES_POSTGRES_") {
		t.Fatalf("err = %v", err)
	}
}
