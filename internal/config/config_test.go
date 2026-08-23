package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	saved := map[string]string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "REDGRES_") {
			continue
		}
		key, val, _ := strings.Cut(e, "=")
		saved[key] = val
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for key, val := range saved {
			_ = os.Setenv(key, val)
		}
	})
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Environment != EnvironmentDevelopment {
		t.Fatalf("Environment = %q", cfg.Environment)
	}
	if cfg.Address != DefaultAddress {
		t.Fatalf("Address = %q", cfg.Address)
	}
	if cfg.BaseURL != DefaultDevelopmentBaseURL {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.SQLitePath != DefaultSQLitePath {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Fatalf("SessionTTL = %s", cfg.SessionTTL)
	}
	if cfg.AbsoluteSessionTTL != DefaultAbsoluteSessionTTL {
		t.Fatalf("AbsoluteSessionTTL = %s", cfg.AbsoluteSessionTTL)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure should be false in development defaults")
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
}

func TestLoadFlagBeatsEnvironment(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:9000")

	cfg, err := Load([]string{"-address", "127.0.0.1:9100"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Address != "127.0.0.1:9100" {
		t.Fatalf("Address = %q, want flag value", cfg.Address)
	}
}

func TestLoadEnvironmentBeatsDotEnv(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_ADDRESS=127.0.0.1:9200\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:9300")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Address != "127.0.0.1:9300" {
		t.Fatalf("Address = %q, want process environment", cfg.Address)
	}
}

func TestLoadDotEnvBeatsDefault(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_ADDRESS=127.0.0.1:9400\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Address != "127.0.0.1:9400" {
		t.Fatalf("Address = %q, want dotenv value", cfg.Address)
	}
}

func TestLoadProductionFailClosed(t *testing.T) {
	absDB := filepath.Join(t.TempDir(), "redgres.db")
	cases := []struct {
		name    string
		env     map[string]string
		wantVar string
		secret  string
	}{
		{
			name: "non-loopback address",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "0.0.0.0:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_ADDRESS",
			secret:  "0.0.0.0:8790",
		},
		{
			name: "http base url",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "http://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_BASE_URL",
			secret:  "http://console.example.com",
		},
		{
			name: "empty base url",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             " ",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_BASE_URL",
		},
		{
			name: "relative sqlite path",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          "./redgres.db",
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_SQLITE_PATH",
			secret:  "./redgres.db",
		},
		{
			name: "insecure cookie",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "false",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_COOKIE_SECURE",
			secret:  "false",
		},
		{
			name: "absolute ttl shorter than idle",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "1h",
			},
			wantVar: "REDGRES_ABSOLUTE_SESSION_TTL",
			secret:  "1h",
		},
		{
			name: "idle ttl below minimum",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "1m",
				"REDGRES_ABSOLUTE_SESSION_TTL": "24h",
			},
			wantVar: "REDGRES_SESSION_TTL",
			secret:  "1m",
		},
		{
			name: "absolute ttl above maximum",
			env: map[string]string{
				"REDGRES_ENVIRONMENT":          "production",
				"REDGRES_ADDRESS":              "127.0.0.1:8790",
				"REDGRES_BASE_URL":             "https://console.example.com",
				"REDGRES_SQLITE_PATH":          absDB,
				"REDGRES_COOKIE_SECURE":        "true",
				"REDGRES_SESSION_TTL":          "12h",
				"REDGRES_ABSOLUTE_SESSION_TTL": "240h",
			},
			wantVar: "REDGRES_ABSOLUTE_SESSION_TTL",
			secret:  "240h",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfig(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := Load(nil)
			if err == nil {
				t.Fatal("expected production validation error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantVar) {
				t.Fatalf("error %q does not name %s", msg, tc.wantVar)
			}
			if tc.secret != "" && strings.Contains(msg, tc.secret) {
				t.Fatalf("error %q echoed value %q", msg, tc.secret)
			}
		})
	}
}

func TestLoadAcceptsProductionLoopback(t *testing.T) {
	isolateConfig(t)
	abs := filepath.Join(t.TempDir(), "redgres.db")
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", abs)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Production() {
		t.Fatal("expected production")
	}
	if cfg.SessionTTL != 12*time.Hour || cfg.AbsoluteSessionTTL != 24*time.Hour {
		t.Fatalf("ttls = %s / %s", cfg.SessionTTL, cfg.AbsoluteSessionTTL)
	}
}

func TestLoadRejectsBaseURLUserinfo(t *testing.T) {
	isolateConfig(t)
	abs := filepath.Join(t.TempDir(), "redgres.db")
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://canary-user:canary-secret@console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", abs)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected origin validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_BASE_URL") {
		t.Fatalf("error %q", msg)
	}
	if strings.Contains(msg, "canary-secret") || strings.Contains(msg, "canary-user") {
		t.Fatalf("error echoed userinfo: %q", msg)
	}
}

func TestLoadRejectsSQLiteURIInjection(t *testing.T) {
	isolateConfig(t)
	_, err := Load([]string{"-sqlite-path", "./redgres.db?mode=memory"})
	if err == nil {
		t.Fatal("expected sqlite path error")
	}
	if !strings.Contains(err.Error(), "REDGRES_SQLITE_PATH") {
		t.Fatalf("error %q", err)
	}
	if strings.Contains(err.Error(), "mode=memory") {
		t.Fatalf("error echoed path: %q", err)
	}
}

func TestLoadIncompletePostgresFails(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_HOST", "127.0.0.1")
	_, err := Load(nil)
	if err == nil || !strings.Contains(err.Error(), "REDGRES_POSTGRES_USER") && !strings.Contains(err.Error(), "REDGRES_POSTGRES_") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsInvalidProtectedList(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_POSTGRES_HOST", "127.0.0.1")
	t.Setenv("REDGRES_POSTGRES_PORT", "5432")
	t.Setenv("REDGRES_POSTGRES_DATABASE", "postgres")
	t.Setenv("REDGRES_POSTGRES_USER", "redgres_console")
	t.Setenv("REDGRES_POSTGRES_PASSWORD_FILE", "./secrets/pg")
	t.Setenv("REDGRES_POSTGRES_PROTECTED_DATABASES", "bad-name")
	_, err := Load(nil)
	if err == nil || !strings.Contains(err.Error(), "REDGRES_POSTGRES_PROTECTED_DATABASES") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "bad-name") {
		t.Fatalf("echoed identifier: %v", err)
	}
}

func TestLoadFlagErrorOmitsValue(t *testing.T) {
	isolateConfig(t)
	_, err := Load([]string{"-session-ttl", "not-a-duration"})
	if err == nil {
		t.Fatal("expected flag error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "-session-ttl") {
		t.Fatalf("error %q should name the flag", msg)
	}
	if strings.Contains(msg, "not-a-duration") {
		t.Fatalf("error echoed value: %q", msg)
	}
}
