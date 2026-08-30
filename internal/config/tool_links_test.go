package config

import (
	"strings"
	"testing"
)

func TestLoadToolLinksEmptyBoth(t *testing.T) {
	isolateConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "" || cfg.RedisInsightURL != "" {
		t.Fatalf("unset URLs = %q %q", cfg.PgAdminURL, cfg.RedisInsightURL)
	}
}

func TestLoadToolLinksWhitespaceIsUnset(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_PGADMIN_URL", "  ")
	t.Setenv("REDGRES_REDISINSIGHT_URL", "\t")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "" || cfg.RedisInsightURL != "" {
		t.Fatalf("whitespace URLs = %q %q", cfg.PgAdminURL, cfg.RedisInsightURL)
	}
}

func TestLoadToolLinksPgAdminAlone(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_PGADMIN_URL", "https://pgadmin.example.com")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "https://pgadmin.example.com" {
		t.Fatalf("PgAdminURL = %q", cfg.PgAdminURL)
	}
	if cfg.RedisInsightURL != "" {
		t.Fatalf("RedisInsightURL = %q", cfg.RedisInsightURL)
	}
}

func TestLoadToolLinksRedisInsightAlone(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_REDISINSIGHT_URL", "https://redis-insight.example.com")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisInsightURL != "https://redis-insight.example.com" {
		t.Fatalf("RedisInsightURL = %q", cfg.RedisInsightURL)
	}
	if cfg.PgAdminURL != "" {
		t.Fatalf("PgAdminURL = %q", cfg.PgAdminURL)
	}
}

func TestLoadToolLinksBothSetWithPathAndQuery(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_PGADMIN_URL", "https://pgadmin.example.com/browser?next=/x")
	t.Setenv("REDGRES_REDISINSIGHT_URL", "http://127.0.0.1:5540/redis")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "https://pgadmin.example.com/browser?next=/x" {
		t.Fatalf("PgAdminURL = %q", cfg.PgAdminURL)
	}
	if cfg.RedisInsightURL != "http://127.0.0.1:5540/redis" {
		t.Fatalf("RedisInsightURL = %q", cfg.RedisInsightURL)
	}
}

func TestLoadToolLinksProductionHTTPSOptional(t *testing.T) {
	isolateConfig(t)
	setProductionCore(t)
	t.Setenv("REDGRES_PGADMIN_URL", "https://pgadmin.example.com/browser")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "https://pgadmin.example.com/browser" {
		t.Fatalf("PgAdminURL = %q", cfg.PgAdminURL)
	}
	if cfg.RedisInsightURL != "" {
		t.Fatalf("RedisInsightURL = %q", cfg.RedisInsightURL)
	}
}

func TestLoadToolLinksProductionOmitsWhenUnset(t *testing.T) {
	isolateConfig(t)
	setProductionCore(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminURL != "" || cfg.RedisInsightURL != "" {
		t.Fatalf("production unset URLs = %q %q", cfg.PgAdminURL, cfg.RedisInsightURL)
	}
	if strings.Contains(cfg.PgAdminURL, "onelifeltd.xyz") || strings.Contains(cfg.RedisInsightURL, "onelifeltd.xyz") {
		t.Fatal("silent default hostname")
	}
}

func TestLoadRejectsToolLinkProductionHTTP(t *testing.T) {
	isolateConfig(t)
	setProductionCore(t)
	secret := "http://pgadmin.example.com"
	t.Setenv("REDGRES_PGADMIN_URL", secret)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected production http rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_PGADMIN_URL") {
		t.Fatalf("error %q does not name REDGRES_PGADMIN_URL", msg)
	}
	if strings.Contains(msg, secret) || strings.Contains(msg, "pgadmin.example.com") {
		t.Fatalf("error echoed URL: %q", msg)
	}
}

func TestLoadRejectsToolLinkUserinfo(t *testing.T) {
	isolateConfig(t)
	secret := "https://canary-user:canary-secret@redis-insight.example.com"
	t.Setenv("REDGRES_REDISINSIGHT_URL", secret)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected userinfo rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDISINSIGHT_URL") {
		t.Fatalf("error %q does not name REDGRES_REDISINSIGHT_URL", msg)
	}
	if strings.Contains(msg, "canary-secret") || strings.Contains(msg, "canary-user") || strings.Contains(msg, secret) {
		t.Fatalf("error echoed URL: %q", msg)
	}
}

func TestLoadRejectsToolLinkSchemesAndRelative(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "javascript pgadmin", key: "REDGRES_PGADMIN_URL", value: "javascript:alert(1)"},
		{name: "javascript with host", key: "REDGRES_REDISINSIGHT_URL", value: "javascript://evil.example.com/%0aalert(1)"},
		{name: "data", key: "REDGRES_PGADMIN_URL", value: "data:text/html,hello"},
		{name: "file", key: "REDGRES_REDISINSIGHT_URL", value: "file:///etc/passwd"},
		{name: "relative path", key: "REDGRES_PGADMIN_URL", value: "/browser"},
		{name: "scheme-relative", key: "REDGRES_REDISINSIGHT_URL", value: "//pgadmin.example.com"},
		{name: "host without scheme", key: "REDGRES_PGADMIN_URL", value: "pgadmin.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfig(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load(nil)
			if err == nil {
				t.Fatalf("expected rejection of %s", tc.value)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.key) {
				t.Fatalf("error %q does not name %s", msg, tc.key)
			}
			if strings.Contains(msg, tc.value) {
				t.Fatalf("error echoed URL: %q", msg)
			}
		})
	}
}

func TestLoadRejectsToolLinkFragment(t *testing.T) {
	isolateConfig(t)
	secret := "https://pgadmin.example.com/browser#session"
	t.Setenv("REDGRES_PGADMIN_URL", secret)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected fragment rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_PGADMIN_URL") {
		t.Fatalf("error %q does not name REDGRES_PGADMIN_URL", msg)
	}
	if strings.Contains(msg, secret) || strings.Contains(msg, "session") || strings.Contains(msg, "#") {
		t.Fatalf("error echoed URL: %q", msg)
	}
}

func TestLoadToolGateLoopback(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_PGADMIN_EMAIL", "Admin@Redgres.com")
	t.Setenv("REDGRES_PGADMIN_PASSWORD_FILE", "/var/lib/redgres/secrets/pgadmin.pass")
	t.Setenv("REDGRES_PGADMIN_MASTER_PASSWORD_FILE", "/var/lib/redgres/secrets/pgadmin.master")
	t.Setenv("REDGRES_TOOL_GATE_PGADMIN_LISTEN", "127.0.0.1:5050")
	t.Setenv("REDGRES_TOOL_GATE_PGADMIN_UPSTREAM", "http://127.0.0.1:5052")
	t.Setenv("REDGRES_TOOL_GATE_REDISINSIGHT_LISTEN", "127.0.0.1:5540")
	t.Setenv("REDGRES_TOOL_GATE_REDISINSIGHT_UPSTREAM", "127.0.0.1:5542")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PgAdminEmail != "admin@redgres.com" {
		t.Fatalf("email = %q", cfg.PgAdminEmail)
	}
	if cfg.PgAdminPasswordFile != "/var/lib/redgres/secrets/pgadmin.pass" {
		t.Fatalf("password file = %q", cfg.PgAdminPasswordFile)
	}
	if cfg.PgAdminMasterPasswordFile != "/var/lib/redgres/secrets/pgadmin.master" {
		t.Fatalf("master password file = %q", cfg.PgAdminMasterPasswordFile)
	}
	if cfg.ToolGatePgAdminListen != "127.0.0.1:5050" || cfg.ToolGatePgAdminUpstream != "http://127.0.0.1:5052" {
		t.Fatalf("pgadmin gate = %q %q", cfg.ToolGatePgAdminListen, cfg.ToolGatePgAdminUpstream)
	}
	if cfg.ToolGateRedisListen != "127.0.0.1:5540" || cfg.ToolGateRedisUpstream != "http://127.0.0.1:5542" {
		t.Fatalf("redis gate = %q %q", cfg.ToolGateRedisListen, cfg.ToolGateRedisUpstream)
	}
}

func TestLoadRejectsToolGateNonLoopback(t *testing.T) {
	isolateConfig(t)
	t.Setenv("REDGRES_TOOL_GATE_PGADMIN_LISTEN", "0.0.0.0:5050")
	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected non-loopback listen rejection")
	}
	if !strings.Contains(err.Error(), "REDGRES_TOOL_GATE_PGADMIN_LISTEN") {
		t.Fatalf("error = %q", err)
	}
}

func setProductionCore(t *testing.T) {
	t.Helper()
	t.Setenv("REDGRES_ENVIRONMENT", "production")
	t.Setenv("REDGRES_ADDRESS", "127.0.0.1:8790")
	t.Setenv("REDGRES_BASE_URL", "https://console.example.com")
	t.Setenv("REDGRES_SQLITE_PATH", productionSQLitePath)
	t.Setenv("REDGRES_COOKIE_SECURE", "true")
	t.Setenv("REDGRES_SESSION_TTL", "12h")
	t.Setenv("REDGRES_ABSOLUTE_SESSION_TTL", "24h")
}
