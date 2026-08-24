package redisadmin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/config"
)

func TestServicePingNilIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if err := svc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if err := nilSvc.Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service err = %v", err)
	}
}

func TestServicePingMapsMemoryPingErr(t *testing.T) {
	svc := NewService(&MemoryClient{PingErr: errors.New("password=canary-secret host=10.0.0.1")})
	err := svc.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), "10.0.0.1") {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestOpenNotConfiguredDevelopmentReturnsNilService(t *testing.T) {
	svc, closeFn, err := Open(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if svc != nil {
		t.Fatal("expected nil Service when Redis is not configured")
	}
}

func TestOpenProductionWithoutURLFileFailClosed(t *testing.T) {
	_, _, err := Open(context.Background(), config.Config{Environment: config.EnvironmentProduction})
	if err == nil {
		t.Fatal("expected production Open without Redis URL file to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	if strings.Contains(msg, "canary-secret") || strings.Contains(msg, "10.0.0.1") {
		t.Fatalf("leaked canary: %q", msg)
	}
}

func TestOpenValidURLUnusedHighPortSucceedsWithoutPing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("redis://127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if svc == nil {
		t.Fatal("expected Service")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	pingErr := svc.Ping(ctx)
	if !errors.Is(pingErr, ErrUnavailable) {
		t.Fatalf("Ping err = %v", pingErr)
	}
	if strings.Contains(pingErr.Error(), "61999") || strings.Contains(pingErr.Error(), "127.0.0.1") {
		t.Fatalf("leaked address: %v", pingErr)
	}
}

func TestOpenCapturesAdminUsernameWithoutStoringURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("redis://ops_admin@127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if svc == nil {
		t.Fatal("expected Service")
	}
	if svc.adminUser != "ops_admin" {
		t.Fatalf("adminUser = %q", svc.adminUser)
	}
}

func TestOpenCanaryURLAbsentFromErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("rediss://:canary-secret@10.0.0.1:6379/0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Open(context.Background(), config.Config{
		Environment:       config.EnvironmentProduction,
		RedisAdminURLFile: path,
	})
	if err == nil {
		t.Fatal("expected production world-readable Open to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}

const skipVerifyCanaryURL = "rediss://:canary-secret@10.0.0.1:6379/0?skip_verify=true"

func assertNoRedisCanary(t *testing.T, msg string) {
	t.Helper()
	for _, leak := range []string{"canary-secret", "10.0.0.1", "rediss://:canary-secret", skipVerifyCanaryURL, "NOAUTH", "WRONGPASS", "NOPERM"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("leaked %q in %q", leak, msg)
		}
	}
}

const sampleRedisInfo = `# Server
redis_version:8.2.1
uptime_in_seconds:123
# Clients
connected_clients:4
# Memory
used_memory:1048576
maxmemory:0
# Stats
instantaneous_ops_per_sec:12
`

func TestServiceStatusNilIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	m, err := svc.Status(context.Background())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	if m != (Metrics{}) {
		t.Fatalf("metrics = %#v", m)
	}
	var nilSvc *Service
	if _, err := nilSvc.Status(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service err = %v", err)
	}
}

func TestServiceStatusClassifiesPingAuthNOPERMAndDial(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "noauth", err: errors.New("NOAUTH Authentication required. password=canary-secret host=10.0.0.1"), want: ErrAuthFailed},
		{name: "wrongpass", err: errors.New("WRONGPASS invalid username-password pair. password=canary-secret"), want: ErrAuthFailed},
		{name: "noperm", err: errors.New("NOPERM this user has no permissions to run the 'ping' command host=10.0.0.1"), want: ErrPermissionDenied},
		{name: "dial", err: errors.New("dial tcp 10.0.0.1:6379: connect: connection refused password=canary-secret"), want: ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&MemoryClient{PingErr: tc.err})
			m, err := svc.Status(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			if m != (Metrics{}) {
				t.Fatalf("metrics = %#v", m)
			}
			assertNoRedisCanary(t, err.Error())
		})
	}
}

func TestServicePingStillMapsAuthToUnavailable(t *testing.T) {
	svc := NewService(&MemoryClient{PingErr: errors.New("NOAUTH Authentication required. password=canary-secret")})
	err := svc.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	assertNoRedisCanary(t, err.Error())
}

func TestServiceStatusPingOKInfoNOPERM(t *testing.T) {
	svc := NewService(&MemoryClient{
		InfoErr:  errors.New("NOPERM this user has no permissions to run the 'info' command password=canary-secret host=10.0.0.1"),
		InfoText: sampleRedisInfo,
		Size:     50,
	})
	m, err := svc.Status(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v", err)
	}
	if m != (Metrics{}) {
		t.Fatalf("metrics = %#v", m)
	}
	assertNoRedisCanary(t, err.Error())
}

func TestServiceStatusPingOKDBSizeNOPERM(t *testing.T) {
	svc := NewService(&MemoryClient{
		InfoText:  sampleRedisInfo,
		DBSizeErr: errors.New("NOPERM this user has no permissions to run the 'dbsize' command password=canary-secret host=10.0.0.1"),
	})
	m, err := svc.Status(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v", err)
	}
	if m != (Metrics{}) {
		t.Fatalf("metrics = %#v", m)
	}
	assertNoRedisCanary(t, err.Error())
}

func TestServiceStatusFullInfoAndDBSize(t *testing.T) {
	svc := NewService(&MemoryClient{InfoText: sampleRedisInfo, Size: 50})
	m, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if m.Version != "8.2.1" || m.UptimeSeconds != 123 || m.ConnectedClients != 4 {
		t.Fatalf("metrics = %#v", m)
	}
	if m.UsedMemoryBytes != 1048576 || m.MaxMemoryBytes != 0 || m.OpsPerSec != 12 || m.DBSize != 50 {
		t.Fatalf("metrics = %#v", m)
	}
	if m.LatencyMS < 0 {
		t.Fatalf("latency_ms = %v", m.LatencyMS)
	}
}

func TestServiceStatusMissingInfoKeyIsUnreachable(t *testing.T) {
	svc := NewService(&MemoryClient{
		InfoText: "# Server\nuptime_in_seconds:123\nconnected_clients:4\nused_memory:1\nmaxmemory:0\ninstantaneous_ops_per_sec:1\n",
		Size:     1,
	})
	m, err := svc.Status(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if m.Version != "" {
		t.Fatalf("zero-filled version: %#v", m)
	}
}

func TestServiceStatusUnparseableInfoKeyIsUnreachable(t *testing.T) {
	svc := NewService(&MemoryClient{
		InfoText: strings.ReplaceAll(sampleRedisInfo, "uptime_in_seconds:123", "uptime_in_seconds:not-a-number"),
		Size:     1,
	})
	m, err := svc.Status(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if m.Version != "" {
		t.Fatalf("zero-filled version: %#v", m)
	}
}

func TestOpenSkipVerifyRedissRejectedAllEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte(skipVerifyCanaryURL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if closeFn != nil {
		defer closeFn()
	}
	if err == nil || svc != nil {
		t.Fatal("expected skip_verify rediss Open to fail without a client")
	}
	msg := err.Error()
	if msg != "REDGRES_REDIS_ADMIN_URL_FILE: invalid value" {
		t.Fatalf("error = %q", msg)
	}
	assertNoRedisCanary(t, msg)
}

func TestOpenProductionWorldReadableStillFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte(skipVerifyCanaryURL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err := Open(context.Background(), config.Config{
		Environment:       config.EnvironmentProduction,
		RedisAdminURLFile: path,
	})
	if closeFn != nil {
		defer closeFn()
	}
	if err == nil || svc != nil {
		t.Fatal("expected production world-readable Open to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "REDGRES_REDIS_ADMIN_URL_FILE") {
		t.Fatalf("error %q does not name REDGRES_REDIS_ADMIN_URL_FILE", msg)
	}
	assertNoRedisCanary(t, msg)
}
