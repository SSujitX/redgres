package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/redisadmin"
	"github.com/SSujitX/redgres/internal/release"
)

const skipLiveEnv = "live integration env not set"

func TestLivePostgresCatalog(t *testing.T) {
	clearInheritedRedgresEnv(t)
	host, port, passwordFile, ok := livePostgresEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	cfg := config.Config{
		Environment:           config.EnvironmentDevelopment,
		PostgresHost:          host,
		PostgresPort:          port,
		PostgresDatabase:      "postgres",
		PostgresUser:          "postgres",
		PostgresPasswordFile:  passwordFile,
		PostgresSSLMode:       "prefer",
		PostgresExpectedMajor: livePostgresExpectedMajor(t),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := postgresadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closer()
	if err := svc.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	listed, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("postgres catalog items=%d (empty manageable list is success on a stock image)", len(listed.Databases))
}

func TestLiveRedisPingStatusACL(t *testing.T) {
	clearInheritedRedgresEnv(t)
	urlFile, ok := liveRedisEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	expected := liveRedisExpectedSeries(t)
	cfg := config.Config{
		Environment:         config.EnvironmentDevelopment,
		RedisAdminURLFile:   urlFile,
		RedisExpectedSeries: expected,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := redisadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if closer != nil {
		defer closer()
	}
	if svc == nil {
		t.Fatal("Open returned nil service")
	}
	if err := svc.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	metrics, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if metrics.Version == "" {
		t.Fatal("Status version is empty")
	}
	series, err := release.RedisSeriesFromVersion(metrics.Version)
	if err != nil || !release.SupportedRedisSeries(series) {
		t.Fatalf("redis_version %q is not a supported series", metrics.Version)
	}
	if expected != "" && series != expected {
		t.Fatalf("redis_version %q series %q want %q", metrics.Version, series, expected)
	}
	t.Logf("redis_version=%s series=%s", metrics.Version, series)
	if _, err := svc.ListUsers(ctx); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
}

func livePostgresEnv(t *testing.T) (host, port, passwordFile string, ok bool) {
	t.Helper()
	host = strings.TrimSpace(os.Getenv("REDGRES_TEST_POSTGRES_HOST"))
	port = strings.TrimSpace(os.Getenv("REDGRES_TEST_POSTGRES_PORT"))
	passwordFile = strings.TrimSpace(os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"))
	if host == "" || port == "" || passwordFile == "" {
		return "", "", "", false
	}
	if !allowedLoopbackHost(host) {
		t.Fatalf("REDGRES_TEST_POSTGRES_HOST %q is not loopback", host)
	}
	return host, port, passwordFile, true
}

func livePostgresExpectedMajor(t *testing.T) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("REDGRES_TEST_POSTGRES_EXPECTED_MAJOR"))
	if raw == "" {
		return 0
	}
	major, err := release.ParseExpectedPostgreSQLMajor(raw)
	if err != nil {
		t.Fatalf("REDGRES_TEST_POSTGRES_EXPECTED_MAJOR %q is not 17 or 18", raw)
	}
	return major
}

func liveRedisEnv(t *testing.T) (urlFile string, ok bool) {
	t.Helper()
	urlFile = strings.TrimSpace(os.Getenv("REDGRES_TEST_REDIS_URL_FILE"))
	if urlFile == "" {
		return "", false
	}
	raw, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatalf("read redis url file: %v", err)
	}
	line := strings.TrimSpace(string(raw))
	host := redisURLHost(t, line)
	if !allowedLoopbackHost(host) {
		t.Fatalf("redis URL host %q is not loopback", host)
	}
	return urlFile, true
}

func liveRedisExpectedSeries(t *testing.T) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("REDGRES_TEST_REDIS_EXPECTED_SERIES"))
	if raw == "" {
		return ""
	}
	series, err := release.ParseExpectedRedisSeries(raw)
	if err != nil {
		t.Fatalf("REDGRES_TEST_REDIS_EXPECTED_SERIES %q is not 8.2 or 8.8", raw)
	}
	return series
}

func redisURLHost(t *testing.T, raw string) string {
	t.Helper()
	withoutScheme, ok := strings.CutPrefix(raw, "redis://")
	if !ok {
		t.Fatalf("redis URL must be redis:// loopback, got %q", raw)
	}
	hostport := withoutScheme
	if i := strings.Index(hostport, "/"); i >= 0 {
		hostport = hostport[:i]
	}
	if strings.HasPrefix(hostport, "[") {
		end := strings.Index(hostport, "]")
		if end < 1 {
			t.Fatalf("invalid redis URL host in %q", raw)
		}
		return hostport[:end+1]
	}
	if i := strings.LastIndex(hostport, ":"); i >= 0 {
		return hostport[:i]
	}
	return hostport
}

func clearInheritedRedgresEnv(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "REDGRES_") && !strings.HasPrefix(key, "REDGRES_TEST_") {
			t.Setenv(key, "")
		}
	}
}
