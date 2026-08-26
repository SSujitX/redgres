package integration

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
)

// TestLivePgBouncerConsole verifies the pooled admin path against a real
// PgBouncer: postgresadmin.PingPooled executes SHOW VERSION on the pgbouncer
// console database, and the HTTP /status pgbouncer component reports ok.
// Requires REDGRES_TEST_PGBOUNCER_PORT (a loopback PgBouncer port proxying to
// the same PostgreSQL); the disposable CI cells do not run PgBouncer, so this
// test skips when it is unset.
func TestLivePgBouncerConsole(t *testing.T) {
	clearInheritedRedgresEnv(t)
	host, port, passwordFile, ok := livePostgresEnv(t)
	if !ok {
		t.Skip(skipLiveEnv)
	}
	pooledPort := strings.TrimSpace(os.Getenv("REDGRES_TEST_PGBOUNCER_PORT"))
	if pooledPort == "" {
		t.Skip("REDGRES_TEST_PGBOUNCER_PORT not set")
	}
	if !allowedLoopbackHost(host) {
		t.Fatalf("REDGRES_TEST_POSTGRES_HOST %q is not loopback", host)
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
		PostgresPooledPort:    pooledPort,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closer, err := postgresadmin.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closer()
	if err := svc.PingPooled(ctx); err != nil {
		t.Fatalf("PingPooled (SHOW VERSION on pgbouncer console): %v", err)
	}

	// HTTP /status must report the pgbouncer component ok.
	h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, func(c *config.Config) {
		c.PostgresPooledPort = pooledPort
	})
	states := statusStates(t, liveAuthed(t, h, http.MethodGet, "/api/v1/status", cookie, csrf, ""))
	if states["pgbouncer"] != "ok" {
		t.Fatalf("pgbouncer status = %q want ok (states %v)", states["pgbouncer"], states)
	}
}
