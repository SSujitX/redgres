package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const redisCanaryURL = "rediss://:canary-secret@10.0.0.1:6379/0"

func writeRedisURLFile(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertNoRedisCanary(t *testing.T, msg string) {
	t.Helper()
	for _, leak := range []string{"canary-secret", "10.0.0.1", redisCanaryURL, "@10.0.0.1"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("error %q leaked %q", msg, leak)
		}
	}
}

func TestLoadNoRedisKeysDevelopment(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisConfigured() {
		t.Fatal("RedisConfigured should be false when Redis keys are empty")
	}
	if cfg.RedisAdminURLFile != "" || cfg.RedisAllowPlaintext || cfg.RedisExpectedSeries != "" {
		t.Fatalf("redis fields = %#v", cfg)
	}
	if cfg.RedisPublicHost != "" || cfg.RedisPublicPort != "" {
		t.Fatalf("public redis fields = %#v", cfg)
	}
}

func TestLoadIncompleteRedisFileEnv(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", filepath.Join(t.TempDir(), "missing-redis-url"))

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected missing Redis URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadRedisAllowPlaintextWithoutFile(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_ALLOW_PLAINTEXT", "true")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected Redis plaintext override without URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
}

func TestLoadRejectsRawRedisURLAsFileEnv(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", redisCanaryURL)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected raw URL env value to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadRejectsPlaintextNonLoopbackWithoutAllow(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, "redis://:canary-secret@10.0.0.1:6379/0\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected non-loopback redis:// without allow-plaintext to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	if !strings.Contains(msg, "REDGRES_REDIS_ALLOW_PLAINTEXT") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ALLOW_PLAINTEXT", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadAcceptsLoopbackRedisURL(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, "redis://127.0.0.1:6379/0\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.RedisConfigured() {
		t.Fatal("expected RedisConfigured")
	}
	if cfg.RedisAdminURLFile != path {
		t.Fatalf("RedisAdminURLFile = %q", cfg.RedisAdminURLFile)
	}
	if cfg.RedisAllowPlaintext {
		t.Fatal("RedisAllowPlaintext should stay false")
	}
}

func TestLoadAcceptsRedissURL(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, "rediss://redis.example.com:6380/0\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.RedisConfigured() {
		t.Fatal("expected RedisConfigured")
	}
}

func TestLoadAcceptsNonLoopbackRedisWithAllowPlaintext(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, "redis://10.0.0.1:6379/0\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)
	t.Setenv("REDGRES_REDIS_ALLOW_PLAINTEXT", "true")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.RedisAllowPlaintext || !cfg.RedisConfigured() {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestLoadProductionWithoutRedisURLFileFailClosed(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", filepath.Join(t.TempDir(), "missing-production-redis"))

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected production without a usable Redis URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadProductionWorldReadableRedisFileFailClosed(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, redisCanaryURL+"\n", 0o644)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected production world-readable Redis URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadRedisCanaryURLAbsentFromErrors(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, redisCanaryURL+"\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)
	t.Setenv("REDGRES_REDIS_ALLOW_PLAINTEXT", "not-a-bool")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid plaintext flag to fail")
	}
	assertNoRedisCanary(t, err.Error())
}

func TestLoadRejectsSkipVerifyRediss(t *testing.T) {
	isolateConfig(t)
	path := writeRedisURLFile(t, redisCanaryURL+"?skip_verify=true\n", 0o600)
	t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected skip_verify rediss URL to fail")
	}
	msg := err.Error()
	if msg != "REDGRES_REDIS_ADMIN_URL_FILE: invalid value" {
		t.Fatalf("error = %q", msg)
	}
	assertNoRedisCanary(t, msg)
	if strings.Contains(msg, "skip_verify") {
		t.Fatalf("error echoed skip_verify: %q", msg)
	}
}

func TestLoadOptionalRedisPublicHostPort(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_PUBLIC_HOST", "redis.example.com")
	t.Setenv("REDGRES_REDIS_PUBLIC_PORT", "6380")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisPublicHost != "redis.example.com" || cfg.RedisPublicPort != "6380" {
		t.Fatalf("public = %q %q", cfg.RedisPublicHost, cfg.RedisPublicPort)
	}
	if cfg.RedisConfigured() {
		t.Fatal("public host/port must not mark Redis configured")
	}
}

func TestLoadProductionDoesNotRequireRedisPublicHostPort(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisPublicHost != "" || cfg.RedisPublicPort != "" {
		t.Fatalf("public = %q %q", cfg.RedisPublicHost, cfg.RedisPublicPort)
	}
}

func TestLoadAcceptsEmptyRedisExpectedSeries(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisExpectedSeries != "" {
		t.Fatalf("RedisExpectedSeries = %q", cfg.RedisExpectedSeries)
	}
}

func TestLoadAcceptsRedisExpectedSeriesWithURL(t *testing.T) {
	for _, series := range []string{"8.2", "8.8"} {
		t.Run(series, func(t *testing.T) {
			isolateConfig(t)
			path := writeRedisURLFile(t, "redis://127.0.0.1:6379/0\n", 0o600)
			t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)
			t.Setenv("REDGRES_REDIS_EXPECTED_SERIES", series)

			cfg, err := Load(nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.RedisExpectedSeries != series {
				t.Fatalf("RedisExpectedSeries = %q", cfg.RedisExpectedSeries)
			}
		})
	}
}

func TestLoadRejectsInvalidRedisExpectedSeries(t *testing.T) {
	denied := []string{"latest", "latest-tested", "8.8.2", "8.2.9", "8.10", "8.10.1", "8.0", "7.2"}
	for _, series := range denied {
		t.Run(series, func(t *testing.T) {
			isolateConfig(t)
			path := writeRedisURLFile(t, "redis://127.0.0.1:6379/0\n", 0o600)
			t.Setenv("REDGRES_REDIS_ADMIN_URL_FILE", path)
			t.Setenv("REDGRES_REDIS_EXPECTED_SERIES", series)

			_, err := Load(nil)
			if err == nil {
				t.Fatal("expected invalid series to fail")
			}
			msg := err.Error()
			if !strings.Contains(msg, "REDGRES_REDIS_EXPECTED_SERIES") {
				t.Fatalf("error %q does not name REDGRES_REDIS_EXPECTED_SERIES", msg)
			}
			assertNoRedisCanary(t, msg)
		})
	}
}

func TestLoadRedisExpectedSeriesWithoutURLFailsClosed(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_EXPECTED_SERIES", "8.8")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected series without URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestLoadRejectsInvalidRedisPublicPort(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDIS_PUBLIC_PORT", "not-a-port")

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid public port to fail")
	}
	if err.Error() != "REDGRES_REDIS_PUBLIC_PORT: invalid value" {
		t.Fatalf("error = %q", err.Error())
	}
}
