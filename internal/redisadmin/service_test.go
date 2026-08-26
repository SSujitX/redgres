package redisadmin

import (
	"context"
	"errors"
	"fmt"
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

func TestOpenValidURLUnusedHighPortFailsClosedOnINFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("redis://127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open err = %v", err)
	}
	assertNoRedisCanary(t, err.Error())
	if strings.Contains(err.Error(), "61999") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("leaked address: %v", err)
	}
}

func TestOpenUnreachableURLDoesNotEchoAdminUsername(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("redis://ops_admin@127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, _, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if svc != nil {
		t.Fatal("expected nil Service on unreachable Redis")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open err = %v", err)
	}
	assertNoRedisCanary(t, err.Error())
	if strings.Contains(err.Error(), "ops_admin") || strings.Contains(err.Error(), "61999") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("leaked identity: %v", err)
	}
}

func TestOpenCanceledContextFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.WriteFile(path, []byte("redis://127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, _, err := Open(ctx, config.Config{RedisAdminURLFile: path})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open err = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("Open did not honor canceled caller context")
	}
	assertNoRedisCanary(t, err.Error())
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

func TestOpenRejectsSymlinkedURLFileWithoutReadingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "redis-target")
	const canary = "redis-symlink-canary"
	if err := os.WriteFile(target, []byte("redis://:"+canary+"@127.0.0.1:61999/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "redis-admin-url")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, _, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if err == nil {
		t.Fatal("expected symlink to fail")
	}
	if err.Error() != "REDGRES_REDIS_ADMIN_URL_FILE: must be a regular file" {
		t.Fatalf("err = %q", err.Error())
	}
	if strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), path) {
		t.Fatalf("error exposed canary or path: %q", err.Error())
	}
}

func TestOpenRejectsNonRegularURLFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redis-admin-url")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	_, _, err := Open(context.Background(), config.Config{RedisAdminURLFile: path})
	if err == nil {
		t.Fatal("expected non-regular URL file to fail")
	}
	if err.Error() != "REDGRES_REDIS_ADMIN_URL_FILE: must be a regular file" {
		t.Fatalf("err = %q", err.Error())
	}
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

func TestServiceSearchOmitsProtectedUsernames(t *testing.T) {
	svc := &Service{
		client: &MemoryClient{ACLLines: []string{
			"user default on nopass ~* &* +@all",
			"user admin on ~* -@all +ping",
			"user redact_admin on ~* -@all +ping",
			"user ops_admin on ~* -@all +ping",
			"user project_a on ~project_a:* -@all +ping",
		}},
		adminUser: "ops_admin",
	}
	got, err := svc.Search(context.Background(), "project", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" || got.Truncated {
		t.Fatalf("project = %#v", got)
	}
	for _, q := range []string{"default", "admin", "redact_admin", "ops_admin"} {
		empty, err := svc.Search(context.Background(), q, 20)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if len(empty.Hits) != 0 || empty.Truncated {
			t.Fatalf("%s = %#v", q, empty)
		}
	}
}

func TestServiceSearchIsCaseInsensitiveOnUsername(t *testing.T) {
	svc := NewService(&MemoryClient{ACLLines: []string{
		"user project_a on ~project_a:* -@all +ping",
	}})
	got, err := svc.Search(context.Background(), "PROJECT", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" {
		t.Fatalf("search = %#v", got)
	}
}

func TestServiceSearchRespectsLimitAndTruncation(t *testing.T) {
	svc := NewService(&MemoryClient{ACLLines: []string{
		"user project_a on ~project_a:* -@all +ping",
		"user project_b on ~project_b:* -@all +ping",
	}})
	got, err := svc.Search(context.Background(), "project", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Name != "project_a" || !got.Truncated {
		t.Fatalf("limit = %#v", got)
	}

	lines := make([]string, 0, 501)
	for i := 0; i < 501; i++ {
		lines = append(lines, fmt.Sprintf("user u%03d on ~u%03d:* -@all +ping", i, i))
	}
	truncatedList := NewService(&MemoryClient{ACLLines: lines})
	listed, err := truncatedList.Search(context.Background(), "u000", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Hits) != 1 || listed.Hits[0].Name != "u000" || !listed.Truncated {
		t.Fatalf("list truncated = %#v", listed)
	}
}

func TestServiceSearchNilIsNotConfigured(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.Search(context.Background(), "project", 20); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	var nilSvc *Service
	if _, err := nilSvc.Search(context.Background(), "project", 20); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service: %v", err)
	}
}

func TestServiceSearchClassifiesACLErrorsWithoutCanary(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "noauth", err: errors.New("NOAUTH Authentication required. password=canary-secret host=10.0.0.1"), want: ErrAuthFailed},
		{name: "noperm", err: errors.New("NOPERM this user has no permissions to run the 'acl|list' command host=10.0.0.1"), want: ErrPermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&MemoryClient{ACLListErr: tc.err})
			got, err := svc.Search(context.Background(), "project", 20)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
			assertNoRedisCanary(t, err.Error())
			blob := fmt.Sprintf("%#v", got)
			if strings.Contains(blob, "canary-secret") {
				t.Fatalf("leaked canary into SearchResult: %s", blob)
			}
			if len(got.Hits) != 0 {
				t.Fatalf("hits = %#v", got)
			}
		})
	}
}
