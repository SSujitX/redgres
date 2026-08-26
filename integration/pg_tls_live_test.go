package integration

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/SSujitX/redgres/internal/redisadmin"
)

// TestLivePostgresTLSVerifyFull verifies the direct PostgreSQL admin path over
// TLS against a real server with a self-signed CA: sslmode=verify-full with
// the CA succeeds (Ping + catalog + HTTP /status), verify-full with a wrong or
// missing CA fails closed, and sslmode=require works without verification.
// Requires REDGRES_TEST_TLS_POSTGRES_PORT + REDGRES_TEST_TLS_POSTGRES_CA (a
// loopback TLS PostgreSQL); the disposable CI cells stay plaintext and skip.
func TestLivePostgresTLSVerifyFull(t *testing.T) {
	clearInheritedRedgresEnv(t)
	tlsPort := strings.TrimSpace(os.Getenv("REDGRES_TEST_TLS_POSTGRES_PORT"))
	caPath := strings.TrimSpace(os.Getenv("REDGRES_TEST_TLS_POSTGRES_CA"))
	if tlsPort == "" || caPath == "" {
		t.Skip("REDGRES_TEST_TLS_POSTGRES_PORT/CA not set")
	}
	passwordFile := strings.TrimSpace(os.Getenv("REDGRES_TEST_POSTGRES_PASSWORD_FILE"))
	if passwordFile == "" {
		t.Skip("REDGRES_TEST_POSTGRES_PASSWORD_FILE not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	base := config.Config{
		Environment:           config.EnvironmentDevelopment,
		PostgresHost:          "127.0.0.1",
		PostgresPort:          tlsPort,
		PostgresDatabase:      "postgres",
		PostgresUser:          "postgres",
		PostgresPasswordFile:  passwordFile,
		PostgresSSLMode:       "verify-full",
		PostgresSSLRootCert:   caPath,
		PostgresExpectedMajor: livePostgresExpectedMajor(t),
	}

	svc, closer, err := postgresadmin.Open(ctx, base)
	if err != nil {
		t.Fatalf("Open verify-full with CA: %v", err)
	}
	defer closer()
	if err := svc.Ping(ctx); err != nil {
		t.Fatalf("Ping over TLS: %v", err)
	}
	if _, err := svc.List(ctx); err != nil {
		t.Fatalf("List over TLS: %v", err)
	}

	// HTTP /status reports the TLS-backed postgres component ok.
	_, redisOK := liveRedisEnv(t)
	if redisOK {
		h, cookie, csrf, _, _, _ := buildLiveHTTPServer(t, func(c *config.Config) {
			c.PostgresHost = "127.0.0.1"
			c.PostgresPort = tlsPort
			c.PostgresSSLMode = "verify-full"
			c.PostgresSSLRootCert = caPath
		})
		states := statusStates(t, liveAuthed(t, h, http.MethodGet, "/api/v1/status", cookie, csrf, ""))
		if states["postgres_direct"] != "ok" {
			t.Fatalf("postgres_direct = %q want ok (states %v)", states["postgres_direct"], states)
		}
	}

	// Fail closed: verify-full with an untrusted CA.
	wrongCA := filepath.Join(t.TempDir(), "wrong-ca.crt")
	if err := os.WriteFile(wrongCA, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.PostgresSSLRootCert = wrongCA
	if _, _, err := postgresadmin.Open(ctx, bad); err == nil {
		t.Fatal("verify-full with wrong CA must fail closed")
	}

	// Fail closed: verify-full with no CA (system roots only).
	noCA := base
	noCA.PostgresSSLRootCert = ""
	if _, _, err := postgresadmin.Open(ctx, noCA); err == nil {
		t.Fatal("verify-full without CA must fail closed")
	}

	// sslmode=require (no verification) still works.
	req := base
	req.PostgresSSLMode = "require"
	req.PostgresSSLRootCert = ""
	svcReq, closerReq, err := postgresadmin.Open(ctx, req)
	if err != nil {
		t.Fatalf("Open require: %v", err)
	}
	defer closerReq()
	if err := svcReq.Ping(ctx); err != nil {
		t.Fatalf("Ping require: %v", err)
	}
}

// TestLiveRedisTLSFailClosed verifies that a self-signed TLS Redis endpoint
// fails closed: go-redis rediss:// does not set InsecureSkipVerify, so the
// handshake against the untrusted certificate fails and redisadmin.Open
// returns ErrUnavailable. Requires REDGRES_TEST_TLS_REDIS_URL_FILE (a rediss
// URL to a loopback TLS Redis); the disposable CI cells stay plaintext.
func TestLiveRedisTLSFailClosed(t *testing.T) {
	clearInheritedRedgresEnv(t)
	urlFile := strings.TrimSpace(os.Getenv("REDGRES_TEST_TLS_REDIS_URL_FILE"))
	if urlFile == "" {
		t.Skip("REDGRES_TEST_TLS_REDIS_URL_FILE not set")
	}
	cfg := config.Config{
		Environment:         config.EnvironmentDevelopment,
		RedisAdminURLFile:   urlFile,
		RedisExpectedSeries: liveRedisExpectedSeries(t),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, err := redisadmin.Open(ctx, cfg)
	if !errors.Is(err, redisadmin.ErrUnavailable) {
		t.Fatalf("Open self-signed rediss = %v want ErrUnavailable (fail closed)", err)
	}
}
