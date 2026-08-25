package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDotEnvSkippedInProduction(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_ADDRESS=127.0.0.1:9500\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Address != "127.0.0.1:8790" {
		t.Fatalf("Address = %q, dotenv must not be read in production", cfg.Address)
	}
}

func TestDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_LOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDGRES_LOG_LEVEL", "warn")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want process environment", cfg.LogLevel)
	}
}

func TestDotEnvParsesCommentsBlanksAndQuotes(t *testing.T) {
	isolateConfig(t)
	content := strings.Join([]string{
		"# comment",
		"",
		"  REDGRES_LOG_LEVEL = \"debug\"  ",
		"REDGRES_ADDRESS='127.0.0.1:9600'",
	}, "\n") + "\n"
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.Address != "127.0.0.1:9600" {
		t.Fatalf("Address = %q", cfg.Address)
	}
}

func TestDotEnvMalformedLineFailsClosed(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_LOG_LEVEL=info\nSUPER_SECRET=should-not-appear\nbad line without delimiter\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected malformed dotenv error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "line 3") {
		t.Fatalf("error %q should name the line", msg)
	}
	if strings.Contains(msg, "should-not-appear") || strings.Contains(msg, "SUPER_SECRET") || strings.Contains(msg, "bad line") {
		t.Fatalf("error %q echoed dotenv content", msg)
	}
}

func TestLoadRepositoryEnvExample(t *testing.T) {
	isolateConfig(t)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	example := filepath.Join(filepath.Dir(thisFile), "..", "..", ".env.example")
	raw, err := os.ReadFile(example)
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	if err := os.WriteFile(".env", raw, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load .env.example: %v", err)
	}
	if cfg.Environment != EnvironmentDevelopment {
		t.Fatalf("Environment = %q", cfg.Environment)
	}
	if cfg.Address != "127.0.0.1:8790" {
		t.Fatalf("Address = %q", cfg.Address)
	}
	if cfg.BaseURL != "http://127.0.0.1:8790" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.SQLitePath != "./redgres.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.SessionTTL.String() != "12h0m0s" {
		t.Fatalf("SessionTTL = %s", cfg.SessionTTL)
	}
	if cfg.AbsoluteSessionTTL.String() != "24h0m0s" {
		t.Fatalf("AbsoluteSessionTTL = %s", cfg.AbsoluteSessionTTL)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure should be false in the development example")
	}
}

func TestProductionFlagIgnoresDotEnv(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_ADDRESS=127.0.0.1:9500\nHTTP_PROXY=http://attacker.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")

	cfg, err := Load([]string{"-environment", "production", "-address", "127.0.0.1:8790"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Address != "127.0.0.1:8790" {
		t.Fatalf("Address = %q", cfg.Address)
	}
	if _, ok := os.LookupEnv("HTTP_PROXY"); ok && os.Getenv("HTTP_PROXY") == "http://attacker.example" {
		t.Fatal("non-REDGRES dotenv key was applied")
	}
}

func TestLoadDevelopmentDotEnvCannotSelectProduction(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("REDGRES_ENVIRONMENT=production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDevelopmentDotEnv(nil); err == nil {
		t.Fatal("expected production-via-dotenv error")
	} else if !strings.Contains(err.Error(), "REDGRES_ENVIRONMENT") {
		t.Fatalf("error %q", err)
	}
}

func TestDotEnvCannotSelectProduction(t *testing.T) {
	isolateConfig(t)
	abs := filepath.Join(t.TempDir(), "redgres.db")
	if err := os.WriteFile(".env", []byte("REDGRES_ENVIRONMENT=production\nREDGRES_BASE_URL=https://console.example.com\nREDGRES_SQLITE_PATH="+abs+"\nREDGRES_COOKIE_SECURE=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected production-via-dotenv error")
	}
	if !strings.Contains(err.Error(), "REDGRES_ENVIRONMENT") {
		t.Fatalf("error %q", err)
	}
}

func TestDotEnvIgnoresNonRedgresKeys(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(".env", []byte("HTTP_PROXY=http://attacker.example\nREDGRES_LOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if os.Getenv("HTTP_PROXY") == "http://attacker.example" {
		t.Fatal("HTTP_PROXY was applied from dotenv")
	}
}
