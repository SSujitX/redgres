package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadBootstrapDefaults(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BootstrapAddress != "" {
		t.Fatalf("BootstrapAddress = %q, want empty (disabled)", cfg.BootstrapAddress)
	}
	if cfg.BootstrapTTL != DefaultBootstrapTTL {
		t.Fatalf("BootstrapTTL = %s, want %s", cfg.BootstrapTTL, DefaultBootstrapTTL)
	}
}

func TestLoadBootstrapAddressNonLoopbackAllowedInProduction(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_BOOTSTRAP_ADDRESS", "0.0.0.0:8989")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BootstrapAddress != "0.0.0.0:8989" {
		t.Fatalf("BootstrapAddress = %q", cfg.BootstrapAddress)
	}
}

func TestLoadBootstrapHTTPAllowsCookieSecureFalse(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "http://127.0.0.1:8989")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "false")
	t.Setenv("REDGRES_BOOTSTRAP_ADDRESS", "0.0.0.0:8989")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure should be false during bootstrap HTTP")
	}
	if cfg.BaseURL != "http://127.0.0.1:8989" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestLoadProductionHTTPRejectedWithoutBootstrap(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "http://127.0.0.1:8989")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	if _, err := Load(nil); err == nil || !strings.Contains(err.Error(), "REDGRES_BASE_URL") {
		t.Fatalf("expected REDGRES_BASE_URL error, got %v", err)
	}
}

func TestLoadBootstrapTTLFlag(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load([]string{"-bootstrap-ttl", "45m"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BootstrapTTL != 45*time.Minute {
		t.Fatalf("BootstrapTTL = %s, want 45m", cfg.BootstrapTTL)
	}
}

func TestLoadCloudflareTokenFileValidation(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_CLOUDFLARE_TOKEN_FILE", "/tmp/tok en?")
	if _, err := Load(nil); err == nil || !strings.Contains(err.Error(), "REDGRES_CLOUDFLARE_TOKEN_FILE") {
		t.Fatalf("expected REDGRES_CLOUDFLARE_TOKEN_FILE error, got %v", err)
	}

	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_CLOUDFLARE_TOKEN_FILE", "/etc/redgres/token")
	if _, err := Load(nil); err == nil || !strings.Contains(err.Error(), "REDGRES_CLOUDFLARE_TOKEN_FILE") {
		t.Fatalf("expected production path error, got %v", err)
	}
}

func TestLoadBootstrapValidation(t *testing.T) {
	cases := []struct {
		name    string
		address string
		ttl     string
		wantVar string
		secret  string
	}{
		{name: "not host port", address: "not-a-valid-addr", wantVar: "REDGRES_BOOTSTRAP_ADDRESS", secret: "not-a-valid-addr"},
		{name: "equals main address", address: "127.0.0.1:8790", wantVar: "REDGRES_BOOTSTRAP_ADDRESS", secret: "127.0.0.1:8790"},
		{name: "non-positive ttl", address: "0.0.0.0:8989", ttl: "0s", wantVar: "REDGRES_BOOTSTRAP_TTL", secret: "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfig(t)
			t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
			t.Setenv("REDGRES_BOOTSTRAP_ADDRESS", tc.address)
			if tc.ttl != "" {
				t.Setenv("REDGRES_BOOTSTRAP_TTL", tc.ttl)
			}
			_, err := Load(nil)
			if err == nil {
				t.Fatal("Load should fail")
			}
			if !strings.Contains(err.Error(), tc.wantVar) {
				t.Fatalf("error %q should name %q", err, tc.wantVar)
			}
			if tc.secret != "" && strings.Contains(err.Error(), tc.secret) {
				t.Fatalf("error %q must not echo secret %q", err, tc.secret)
			}
		})
	}
}
