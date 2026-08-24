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
	if strings.Contains(msg, "canary-secret") || strings.Contains(msg, "10.0.0.1") || strings.Contains(msg, "rediss://:canary-secret") {
		t.Fatalf("leaked canary: %q", msg)
	}
}
